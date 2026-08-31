package query

import (
	"testing"

	"github.com/xwb1989/sqlparser"

	"dbtool/server/internal/store"
)

func mustParseSelect(t *testing.T, stmt string) *sqlparser.Select {
	t.Helper()
	parsed, err := sqlparser.Parse(stmt)
	if err != nil {
		t.Fatalf("parse %q: %v", stmt, err)
	}
	sel, ok := parsed.(*sqlparser.Select)
	if !ok {
		t.Fatalf("parse %q: not a SELECT", stmt)
	}
	return sel
}

func TestCollectJoinTablesMySQL_MatchesRealTables(t *testing.T) {
	tests := []struct {
		name string
		stmt string
		want map[string]string
	}{
		{
			"aliased join",
			"SELECT * FROM users u JOIN orders o ON u.id = o.user_id",
			map[string]string{"u": "users", "o": "orders"},
		},
		{
			"unaliased join uses table name",
			"SELECT * FROM users JOIN orders ON users.id = orders.user_id",
			map[string]string{"users": "users", "orders": "orders"},
		},
		{
			"self join distinguishes aliases",
			"SELECT * FROM users a JOIN users b ON a.manager_id = b.id",
			map[string]string{"a": "users", "b": "users"},
		},
		{
			"comma join",
			"SELECT * FROM users u, orders o WHERE u.id = o.user_id",
			map[string]string{"u": "users", "o": "orders"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel := mustParseSelect(t, tt.stmt)
			got, _, ok := collectJoinTablesMySQL(sel.From)
			if !ok {
				t.Fatalf("collectJoinTablesMySQL(%q) failed, want match", tt.stmt)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("collectJoinTablesMySQL(%q) = %v, want %v", tt.stmt, got, tt.want)
			}
			for alias, table := range tt.want {
				ref := got[alias]
				if ref.Derived != nil || ref.Table != table {
					t.Errorf("alias %q -> %+v, want table %q", alias, ref, table)
				}
			}
		})
	}
}

// TestCollectJoinTablesMySQL_MatchesDerivedTable documents the one-level
// derived-table support (ADR 0011): a subquery in FROM resolves to a
// mysqlTableRef with Derived set instead of being rejected outright, so
// resolveDerivedColumnMySQL can look inside it.
func TestCollectJoinTablesMySQL_MatchesDerivedTable(t *testing.T) {
	sel := mustParseSelect(t, "SELECT * FROM (SELECT id, name FROM users) u JOIN orders o ON u.id = o.user_id")
	got, _, ok := collectJoinTablesMySQL(sel.From)
	if !ok {
		t.Fatal("collectJoinTablesMySQL failed, want match")
	}
	if got["u"].Derived == nil {
		t.Errorf(`alias "u" = %+v, want a Derived select`, got["u"])
	}
	if got["o"].Table != "orders" {
		t.Errorf(`alias "o" = %+v, want table "orders"`, got["o"])
	}
}

func TestCollectJoinTablesMySQL_RejectsUnionInDerivedTable(t *testing.T) {
	sel := mustParseSelect(t, "SELECT * FROM (SELECT id FROM users UNION SELECT id FROM orders) u JOIN orders o ON u.id = o.user_id")
	if _, _, ok := collectJoinTablesMySQL(sel.From); ok {
		t.Error("collectJoinTablesMySQL matched a UNION inside a derived table, want reject")
	}
}

func TestPrepareJoinOriginsMySQL_NonMySQLBailsOut(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsMySQL(db, store.DBKindPostgres, "SELECT u.id FROM users u JOIN orders o ON u.id = o.user_id")
	if ok || origins != nil {
		t.Errorf("prepareJoinOriginsMySQL on non-MySQL kind = (ok=%v, origins=%v), want (false, nil)", ok, origins)
	}
}

// TestPrepareJoinOriginsMySQL_GroupByBailsOutEntirely guards against a real
// regression found while verifying this against Docker MySQL: appending a
// hidden PK carrier column to a GROUP BY query can turn a query MySQL would
// otherwise run into an only_full_group_by error, and doing the same to a
// DISTINCT query would silently change which rows count as distinct. Both
// must leave the statement completely untouched.
func TestPrepareJoinOriginsMySQL_GroupByBailsOutEntirely(t *testing.T) {
	db := newTestDB(t)
	stmt := "SELECT u.name, COUNT(*) FROM users u JOIN orders o ON u.id = o.user_id GROUP BY u.name"
	rewritten, origins, ok := prepareJoinOriginsMySQL(db, testKind, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOriginsMySQL(GROUP BY) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

func TestPrepareJoinOriginsMySQL_DistinctBailsOutEntirely(t *testing.T) {
	db := newTestDB(t)
	stmt := "SELECT DISTINCT u.name FROM users u JOIN orders o ON u.id = o.user_id"
	rewritten, origins, ok := prepareJoinOriginsMySQL(db, testKind, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOriginsMySQL(DISTINCT) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

// TestPrepareJoinOriginsMySQL_SingleTableSupported documents the ADR 0011
// fix for a real bug: a single real table with an explicit column list
// used to fall between ADR 0009's `SELECT *`-only path and this pass's
// original 2-table-minimum gate, so no origins were ever produced and the
// download button never appeared for a query as ordinary as `SELECT id,
// name FROM users`. A single table is now accepted just like a JOIN — PK
// lookup still fails open against this test's SQLite backend (see the
// PKLookupFailsOpen-style tests below), so origins stay nil here; Docker
// verification confirms the end-to-end success case.
func TestPrepareJoinOriginsMySQL_SingleTableSupported(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsMySQL(db, testKind, "SELECT id, name FROM users")
	if !ok {
		t.Fatal("prepareJoinOriginsMySQL rejected a single-table explicit-column-list statement, want accept (ADR 0011 gap fix)")
	}
	if len(origins) != 2 {
		t.Fatalf("origins = %v, want 2 entries (one per selected column)", origins)
	}
}

// TestPrepareJoinOriginsMySQL_SingleTableUnqualifiedColumnSupported
// documents that, unlike in a JOIN, an unqualified column in a single-table
// query is unambiguous and must not be treated as an unrecognized shape.
func TestPrepareJoinOriginsMySQL_SingleTableUnqualifiedColumnSupported(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsMySQL(db, testKind, "SELECT name FROM users")
	if !ok {
		t.Fatal("prepareJoinOriginsMySQL rejected a single-table statement with an unqualified column, want accept")
	}
	if len(origins) != 1 {
		t.Fatalf("origins = %v, want 1 entry", origins)
	}
}

// TestPrepareJoinOriginsMySQL_DerivedTableColumnUnresolvable documents that
// a derived table using `SELECT *` can't be traced by name (ADR 0011's
// one-level pass doesn't have the inner table's schema to know what `*`
// expands to) — the statement shape is still recognized (ok=true), the
// column just gets no origin.
func TestPrepareJoinOriginsMySQL_DerivedTableColumnUnresolvable(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsMySQL(db, testKind, "SELECT u.id FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id")
	if !ok {
		t.Fatal("prepareJoinOriginsMySQL rejected a recognized derived-table JOIN shape")
	}
	if len(origins) != 1 || origins[0] != nil {
		t.Errorf("origins = %v, want a single nil entry (SELECT * can't be traced by name)", origins)
	}
}

// TestPrepareJoinOriginsMySQL_UnqualifiedColumnStaysUnknown documents the
// column-level fail-closed rule (ADR 0011): a column not qualified by a
// table alias is ambiguous in a JOIN and must not get an origin, even
// though the statement shape as a whole is recognized.
func TestPrepareJoinOriginsMySQL_UnqualifiedColumnStaysUnknown(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsMySQL(db, testKind, "SELECT count FROM users u JOIN orders o ON u.id = o.user_id")
	if !ok {
		t.Fatal("prepareJoinOriginsMySQL rejected a recognized JOIN shape")
	}
	if len(origins) != 1 || origins[0] != nil {
		t.Errorf("origins = %v, want a single nil entry", origins)
	}
}

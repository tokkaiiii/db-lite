package query

import (
	"testing"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	pgquery "github.com/wasilibs/go-pgquery"

	"dbtool/server/internal/store"
)

func mustParseSelectPostgres(t *testing.T, stmt string) *pg_query.SelectStmt {
	t.Helper()
	tree, err := pgquery.Parse(stmt)
	if err != nil {
		t.Fatalf("parse %q: %v", stmt, err)
	}
	if len(tree.Stmts) != 1 {
		t.Fatalf("parse %q: expected 1 statement, got %d", stmt, len(tree.Stmts))
	}
	sel := tree.Stmts[0].Stmt.GetSelectStmt()
	if sel == nil {
		t.Fatalf("parse %q: not a SELECT", stmt)
	}
	return sel
}

func TestCollectJoinTablesPostgres_MatchesRealTables(t *testing.T) {
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
			sel := mustParseSelectPostgres(t, tt.stmt)
			got, _, ok := collectJoinTablesPostgres(sel.FromClause)
			if !ok {
				t.Fatalf("collectJoinTablesPostgres(%q) failed, want match", tt.stmt)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("collectJoinTablesPostgres(%q) = %v, want %v", tt.stmt, got, tt.want)
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

// TestCollectJoinTablesPostgres_MatchesDerivedTable documents the
// one-level derived-table support (ADR 0011): a subquery in FROM resolves
// to a postgresTableRef with Derived set instead of being rejected
// outright, so resolveDerivedColumnPostgres can look inside it.
func TestCollectJoinTablesPostgres_MatchesDerivedTable(t *testing.T) {
	sel := mustParseSelectPostgres(t, "SELECT * FROM (SELECT id, name FROM users) u JOIN orders o ON u.id = o.user_id")
	got, _, ok := collectJoinTablesPostgres(sel.FromClause)
	if !ok {
		t.Fatal("collectJoinTablesPostgres failed, want match")
	}
	if got["u"].Derived == nil {
		t.Errorf(`alias "u" = %+v, want a Derived select`, got["u"])
	}
	if got["o"].Table != "orders" {
		t.Errorf(`alias "o" = %+v, want table "orders"`, got["o"])
	}
}

func TestCollectJoinTablesPostgres_RejectsUnionInDerivedTable(t *testing.T) {
	sel := mustParseSelectPostgres(t, "SELECT * FROM (SELECT id FROM users UNION SELECT id FROM orders) u JOIN orders o ON u.id = o.user_id")
	if _, _, ok := collectJoinTablesPostgres(sel.FromClause); ok {
		t.Error("collectJoinTablesPostgres matched a UNION inside a derived table, want reject")
	}
}

// TestPrepareJoinOriginsPostgres_SingleTableSupported documents the ADR
// 0011 fix for a real bug — see the MySQL test of the same shape for the
// full rationale. PK lookup still fails open against this test's SQLite
// backend, so origins stay nil here; Docker verification confirms the
// end-to-end success case.
func TestPrepareJoinOriginsPostgres_SingleTableSupported(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsPostgres(db, store.DBKindPostgres, "SELECT id, name FROM users")
	if !ok {
		t.Fatal("prepareJoinOriginsPostgres rejected a single-table explicit-column-list statement, want accept (ADR 0011 gap fix)")
	}
	if len(origins) != 2 {
		t.Fatalf("origins = %v, want 2 entries (one per selected column)", origins)
	}
}

// TestPrepareJoinOriginsPostgres_SingleTableUnqualifiedColumnSupported
// documents that, unlike in a JOIN, an unqualified column in a single-table
// query is unambiguous and must not be treated as an unrecognized shape.
func TestPrepareJoinOriginsPostgres_SingleTableUnqualifiedColumnSupported(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsPostgres(db, store.DBKindPostgres, "SELECT name FROM users")
	if !ok {
		t.Fatal("prepareJoinOriginsPostgres rejected a single-table statement with an unqualified column, want accept")
	}
	if len(origins) != 1 {
		t.Fatalf("origins = %v, want 1 entry", origins)
	}
}

// TestPrepareJoinOriginsPostgres_DerivedTableColumnUnresolvable documents
// that a derived table using `SELECT *` can't be traced by name (ADR
// 0011's one-level pass doesn't have the inner table's schema to know
// what `*` expands to) — the statement shape is still recognized
// (ok=true), the column just gets no origin.
func TestPrepareJoinOriginsPostgres_DerivedTableColumnUnresolvable(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsPostgres(db, store.DBKindPostgres, "SELECT u.id FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id")
	if !ok {
		t.Fatal("prepareJoinOriginsPostgres rejected a recognized derived-table JOIN shape")
	}
	if len(origins) != 1 || origins[0] != nil {
		t.Errorf("origins = %v, want a single nil entry (SELECT * can't be traced by name)", origins)
	}
}

func TestPrepareJoinOriginsPostgres_GroupByBailsOutEntirely(t *testing.T) {
	db := newTestDB(t)
	stmt := "SELECT u.name, COUNT(*) FROM users u JOIN orders o ON u.id = o.user_id GROUP BY u.name"
	rewritten, origins, ok := prepareJoinOriginsPostgres(db, store.DBKindPostgres, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOriginsPostgres(GROUP BY) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

func TestPrepareJoinOriginsPostgres_DistinctBailsOutEntirely(t *testing.T) {
	db := newTestDB(t)
	stmt := "SELECT DISTINCT u.name FROM users u JOIN orders o ON u.id = o.user_id"
	rewritten, origins, ok := prepareJoinOriginsPostgres(db, store.DBKindPostgres, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOriginsPostgres(DISTINCT) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

// TestPrepareJoinOriginsPostgres_UnqualifiedColumnStaysUnknown documents the
// column-level fail-closed rule (ADR 0011): a column not qualified by a
// table alias is ambiguous in a JOIN and must not get an origin, even
// though the statement shape as a whole is recognized.
func TestPrepareJoinOriginsPostgres_UnqualifiedColumnStaysUnknown(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsPostgres(db, store.DBKindPostgres, "SELECT count FROM users u JOIN orders o ON u.id = o.user_id")
	if !ok {
		t.Fatal("prepareJoinOriginsPostgres rejected a recognized JOIN shape")
	}
	if len(origins) != 1 || origins[0] != nil {
		t.Errorf("origins = %v, want a single nil entry", origins)
	}
}

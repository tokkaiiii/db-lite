package query

import (
	"testing"

	plsqlparser "github.com/bytebase/plsql-parser"

	"dbtool/server/internal/store"
)

// mustParseQueryBlockOracle parses stmt down to its Query_block, the same
// path prepareJoinOriginsOracle takes for a plain (non-UNION) SELECT.
func mustParseQueryBlockOracle(t *testing.T, stmt string) *plsqlparser.Query_blockContext {
	t.Helper()
	qb, ok := parseOracleSelect(stmt)
	if !ok {
		t.Fatalf("parse %q: not a plain query block", stmt)
	}
	return qb
}

func TestCollectJoinTablesOracle_MatchesRealTables(t *testing.T) {
	tests := []struct {
		name string
		stmt string
		want map[string]string
	}{
		{
			"aliased join",
			`SELECT * FROM users u JOIN orders o ON u.id = o.user_id`,
			map[string]string{"u": "users", "o": "orders"},
		},
		{
			"unaliased join uses table name",
			`SELECT * FROM users JOIN orders ON users.id = orders.user_id`,
			map[string]string{"users": "users", "orders": "orders"},
		},
		{
			"self join distinguishes aliases",
			`SELECT * FROM users a JOIN users b ON a.manager_id = b.id`,
			map[string]string{"a": "users", "b": "users"},
		},
		{
			"comma join",
			`SELECT * FROM users u, orders o WHERE u.id = o.user_id`,
			map[string]string{"u": "users", "o": "orders"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := mustParseQueryBlockOracle(t, tt.stmt)
			got, _, ok := collectJoinTablesOracle(qb.From_clause().Table_ref_list())
			if !ok {
				t.Fatalf("collectJoinTablesOracle(%q) failed, want match", tt.stmt)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("collectJoinTablesOracle(%q) = %v, want %v", tt.stmt, got, tt.want)
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

// TestCollectJoinTablesOracle_MatchesDerivedTable documents the one-level
// derived-table support (ADR 0011): a subquery in FROM resolves to an
// oracleTableRef with Derived set instead of being rejected outright, so
// resolveDerivedColumnOracle can look inside it.
func TestCollectJoinTablesOracle_MatchesDerivedTable(t *testing.T) {
	qb := mustParseQueryBlockOracle(t, `SELECT * FROM (SELECT id, name FROM users) u JOIN orders o ON u.id = o.user_id`)
	got, _, ok := collectJoinTablesOracle(qb.From_clause().Table_ref_list())
	if !ok {
		t.Fatal("collectJoinTablesOracle failed, want match")
	}
	if got["u"].Derived == nil {
		t.Errorf(`alias "u" = %+v, want a Derived select`, got["u"])
	}
	if got["o"].Table != "orders" {
		t.Errorf(`alias "o" = %+v, want table "orders"`, got["o"])
	}
}

func TestCollectJoinTablesOracle_RejectsUnionInDerivedTable(t *testing.T) {
	qb := mustParseQueryBlockOracle(t, `SELECT * FROM (SELECT id FROM users UNION SELECT id FROM orders) u JOIN orders o ON u.id = o.user_id`)
	if _, _, ok := collectJoinTablesOracle(qb.From_clause().Table_ref_list()); ok {
		t.Error("collectJoinTablesOracle matched a UNION inside a derived table, want reject")
	}
}

// TestPrepareJoinOriginsOracle_SingleTableSupported documents the ADR 0011
// fix for a real bug — see the MySQL test of the same shape for the full
// rationale. Needs a real (in-memory) DB, unlike the other Oracle tests
// above, because a single table now reaches PK lookup; it still fails open
// against this test's SQLite backend, so origins stay nil here — Docker
// verification confirms the end-to-end success case.
func TestPrepareJoinOriginsOracle_SingleTableSupported(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsOracle(db, store.DBKindOracle, `SELECT id, name FROM users`)
	if !ok {
		t.Fatal("prepareJoinOriginsOracle rejected a single-table explicit-column-list statement, want accept (ADR 0011 gap fix)")
	}
	if len(origins) != 2 {
		t.Fatalf("origins = %v, want 2 entries (one per selected column)", origins)
	}
}

// TestPrepareJoinOriginsOracle_SingleTableUnqualifiedColumnSupported
// documents that, unlike in a JOIN, an unqualified column in a single-table
// query is unambiguous and must not be treated as an unrecognized shape.
func TestPrepareJoinOriginsOracle_SingleTableUnqualifiedColumnSupported(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsOracle(db, store.DBKindOracle, `SELECT name FROM users`)
	if !ok {
		t.Fatal("prepareJoinOriginsOracle rejected a single-table statement with an unqualified column, want accept")
	}
	if len(origins) != 1 {
		t.Fatalf("origins = %v, want 1 entry", origins)
	}
}

// TestPrepareJoinOriginsOracle_DerivedTableColumnUnresolvable documents
// that a derived table using `SELECT *` can't be traced by name (ADR
// 0011's one-level pass doesn't have the inner table's schema to know
// what `*` expands to) — the statement shape is still recognized
// (ok=true), the column just gets no origin.
func TestPrepareJoinOriginsOracle_DerivedTableColumnUnresolvable(t *testing.T) {
	_, origins, ok := prepareJoinOriginsOracle(nil, store.DBKindOracle, `SELECT u.id FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id`)
	if !ok {
		t.Fatal("prepareJoinOriginsOracle rejected a recognized derived-table JOIN shape")
	}
	if len(origins) != 1 || origins[0] != nil {
		t.Errorf("origins = %v, want a single nil entry (SELECT * can't be traced by name)", origins)
	}
}

func TestPrepareJoinOriginsOracle_GroupByBailsOutEntirely(t *testing.T) {
	stmt := `SELECT u.name, COUNT(*) FROM users u JOIN orders o ON u.id = o.user_id GROUP BY u.name`
	rewritten, origins, ok := prepareJoinOriginsOracle(nil, store.DBKindOracle, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOriginsOracle(GROUP BY) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

func TestPrepareJoinOriginsOracle_DistinctBailsOutEntirely(t *testing.T) {
	stmt := `SELECT DISTINCT u.name FROM users u JOIN orders o ON u.id = o.user_id`
	rewritten, origins, ok := prepareJoinOriginsOracle(nil, store.DBKindOracle, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOriginsOracle(DISTINCT) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

// TestPrepareJoinOriginsOracle_UnqualifiedColumnStaysUnknown documents the
// column-level fail-closed rule (ADR 0011): a column not qualified by a
// table alias is ambiguous in a JOIN and must not get an origin, even
// though the statement shape as a whole is recognized.
func TestPrepareJoinOriginsOracle_UnqualifiedColumnStaysUnknown(t *testing.T) {
	_, origins, ok := prepareJoinOriginsOracle(nil, store.DBKindOracle, `SELECT cnt FROM users u JOIN orders o ON u.id = o.user_id`)
	if !ok {
		t.Fatal("prepareJoinOriginsOracle rejected a recognized JOIN shape")
	}
	if len(origins) != 1 || origins[0] != nil {
		t.Errorf("origins = %v, want a single nil entry", origins)
	}
}

// TestPrepareJoinOriginsOracle_PKLookupFailsOpen exercises the pass with a
// real (in-memory) DB so dbconn.PrimaryKeyColumns actually runs — testKind
// here is Oracle, which uses `:1`-style bind placeholders SQLite can't
// execute, so this documents the same fail-open behavior the other
// dialects' tests cover: a lookup error must not abort the statement, it
// just leaves that column's origin nil.
func TestPrepareJoinOriginsOracle_PKLookupFailsOpen(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsOracle(db, store.DBKindOracle, `SELECT u.id, u.name FROM users u JOIN orders o ON u.id = o.user_id`)
	if !ok {
		t.Fatal("prepareJoinOriginsOracle rejected a recognized JOIN shape")
	}
	for i, o := range origins {
		if o != nil {
			t.Errorf("origins[%d] = %v, want nil (PK lookup can't succeed against this test's SQLite backend)", i, o)
		}
	}
}

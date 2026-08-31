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

func TestCollectJoinTablesPostgres_Matches(t *testing.T) {
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
			got, ok := collectJoinTablesPostgres(sel.FromClause)
			if !ok {
				t.Fatalf("collectJoinTablesPostgres(%q) failed, want match", tt.stmt)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("collectJoinTablesPostgres(%q) = %v, want %v", tt.stmt, got, tt.want)
			}
			for alias, table := range tt.want {
				if got[alias] != table {
					t.Errorf("alias %q -> %q, want %q", alias, got[alias], table)
				}
			}
		})
	}
}

func TestCollectJoinTablesPostgres_RejectsDerivedTable(t *testing.T) {
	sel := mustParseSelectPostgres(t, "SELECT * FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id")
	if _, ok := collectJoinTablesPostgres(sel.FromClause); ok {
		t.Error("collectJoinTablesPostgres matched a derived table, want reject")
	}
}

func TestPrepareJoinOriginsPostgres_NoJoinBailsOut(t *testing.T) {
	db := newTestDB(t)
	_, _, ok := prepareJoinOriginsPostgres(db, store.DBKindPostgres, "SELECT id, name FROM users")
	if ok {
		t.Error("prepareJoinOriginsPostgres matched a single-table statement, want reject")
	}
}

func TestPrepareJoinOriginsPostgres_DerivedTableBailsOut(t *testing.T) {
	db := newTestDB(t)
	_, _, ok := prepareJoinOriginsPostgres(db, store.DBKindPostgres, "SELECT u.id FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id")
	if ok {
		t.Error("prepareJoinOriginsPostgres matched a derived table, want reject")
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

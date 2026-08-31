package query

import (
	"testing"

	"github.com/ha1tch/tsqlparser"
	"github.com/ha1tch/tsqlparser/ast"

	"dbtool/server/internal/store"
)

func mustParseSelectMSSQL(t *testing.T, stmt string) *ast.SelectStatement {
	t.Helper()
	program, errs := tsqlparser.Parse(stmt)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", stmt, errs)
	}
	if len(program.Statements) != 1 {
		t.Fatalf("parse %q: expected 1 statement, got %d", stmt, len(program.Statements))
	}
	sel, ok := program.Statements[0].(*ast.SelectStatement)
	if !ok {
		t.Fatalf("parse %q: not a SELECT", stmt)
	}
	return sel
}

func TestCollectJoinTablesMSSQL_Matches(t *testing.T) {
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
			sel := mustParseSelectMSSQL(t, tt.stmt)
			got, ok := collectJoinTablesMSSQL(sel.From.Tables)
			if !ok {
				t.Fatalf("collectJoinTablesMSSQL(%q) failed, want match", tt.stmt)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("collectJoinTablesMSSQL(%q) = %v, want %v", tt.stmt, got, tt.want)
			}
			for alias, table := range tt.want {
				if got[alias] != table {
					t.Errorf("alias %q -> %q, want %q", alias, got[alias], table)
				}
			}
		})
	}
}

func TestCollectJoinTablesMSSQL_RejectsDerivedTable(t *testing.T) {
	sel := mustParseSelectMSSQL(t, "SELECT * FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id")
	if _, ok := collectJoinTablesMSSQL(sel.From.Tables); ok {
		t.Error("collectJoinTablesMSSQL matched a derived table, want reject")
	}
}

func TestPrepareJoinOriginsMSSQL_NoJoinBailsOut(t *testing.T) {
	db := newTestDB(t)
	_, _, ok := prepareJoinOriginsMSSQL(db, store.DBKindMSSQL, "SELECT id, name FROM users")
	if ok {
		t.Error("prepareJoinOriginsMSSQL matched a single-table statement, want reject")
	}
}

func TestPrepareJoinOriginsMSSQL_DerivedTableBailsOut(t *testing.T) {
	db := newTestDB(t)
	_, _, ok := prepareJoinOriginsMSSQL(db, store.DBKindMSSQL, "SELECT u.id FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id")
	if ok {
		t.Error("prepareJoinOriginsMSSQL matched a derived table, want reject")
	}
}

func TestPrepareJoinOriginsMSSQL_GroupByBailsOutEntirely(t *testing.T) {
	db := newTestDB(t)
	stmt := "SELECT u.name, COUNT(*) FROM users u JOIN orders o ON u.id = o.user_id GROUP BY u.name"
	rewritten, origins, ok := prepareJoinOriginsMSSQL(db, store.DBKindMSSQL, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOriginsMSSQL(GROUP BY) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

func TestPrepareJoinOriginsMSSQL_DistinctBailsOutEntirely(t *testing.T) {
	db := newTestDB(t)
	stmt := "SELECT DISTINCT u.name FROM users u JOIN orders o ON u.id = o.user_id"
	rewritten, origins, ok := prepareJoinOriginsMSSQL(db, store.DBKindMSSQL, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOriginsMSSQL(DISTINCT) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

// TestPrepareJoinOriginsMSSQL_UnqualifiedColumnStaysUnknown documents the
// column-level fail-closed rule (ADR 0011): a column not qualified by a
// table alias is ambiguous in a JOIN and must not get an origin, even
// though the statement shape as a whole is recognized.
func TestPrepareJoinOriginsMSSQL_UnqualifiedColumnStaysUnknown(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsMSSQL(db, store.DBKindMSSQL, "SELECT count FROM users u JOIN orders o ON u.id = o.user_id")
	if !ok {
		t.Fatal("prepareJoinOriginsMSSQL rejected a recognized JOIN shape")
	}
	if len(origins) != 1 || origins[0] != nil {
		t.Errorf("origins = %v, want a single nil entry", origins)
	}
}

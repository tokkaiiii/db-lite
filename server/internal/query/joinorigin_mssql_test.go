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

func TestCollectJoinTablesMSSQL_MatchesRealTables(t *testing.T) {
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
			got, _, ok := collectJoinTablesMSSQL(sel.From.Tables)
			if !ok {
				t.Fatalf("collectJoinTablesMSSQL(%q) failed, want match", tt.stmt)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("collectJoinTablesMSSQL(%q) = %v, want %v", tt.stmt, got, tt.want)
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

// TestCollectJoinTablesMSSQL_MatchesDerivedTable documents the one-level
// derived-table support (ADR 0011): a subquery in FROM resolves to a
// mssqlTableRef with Derived set instead of being rejected outright, so
// resolveDerivedColumnMSSQL can look inside it.
func TestCollectJoinTablesMSSQL_MatchesDerivedTable(t *testing.T) {
	sel := mustParseSelectMSSQL(t, "SELECT * FROM (SELECT id, name FROM users) u JOIN orders o ON u.id = o.user_id")
	got, _, ok := collectJoinTablesMSSQL(sel.From.Tables)
	if !ok {
		t.Fatal("collectJoinTablesMSSQL failed, want match")
	}
	if got["u"].Derived == nil {
		t.Errorf(`alias "u" = %+v, want a Derived select`, got["u"])
	}
	if got["o"].Table != "orders" {
		t.Errorf(`alias "o" = %+v, want table "orders"`, got["o"])
	}
}

func TestCollectJoinTablesMSSQL_RejectsUnionInDerivedTable(t *testing.T) {
	sel := mustParseSelectMSSQL(t, "SELECT * FROM (SELECT id FROM users UNION SELECT id FROM orders) u JOIN orders o ON u.id = o.user_id")
	if _, _, ok := collectJoinTablesMSSQL(sel.From.Tables); ok {
		t.Error("collectJoinTablesMSSQL matched a UNION inside a derived table, want reject")
	}
}

// TestPrepareJoinOriginsMSSQL_SingleTableSupported documents the ADR 0011
// fix for a real bug — see the MySQL test of the same shape for the full
// rationale. PK lookup still fails open against this test's SQLite
// backend, so origins stay nil here; Docker verification confirms the
// end-to-end success case.
func TestPrepareJoinOriginsMSSQL_SingleTableSupported(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsMSSQL(db, store.DBKindMSSQL, "SELECT id, name FROM users")
	if !ok {
		t.Fatal("prepareJoinOriginsMSSQL rejected a single-table explicit-column-list statement, want accept (ADR 0011 gap fix)")
	}
	if len(origins) != 2 {
		t.Fatalf("origins = %v, want 2 entries (one per selected column)", origins)
	}
}

// TestPrepareJoinOriginsMSSQL_SingleTableUnqualifiedColumnSupported
// documents that, unlike in a JOIN, an unqualified column in a single-table
// query is unambiguous and must not be treated as an unrecognized shape.
func TestPrepareJoinOriginsMSSQL_SingleTableUnqualifiedColumnSupported(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsMSSQL(db, store.DBKindMSSQL, "SELECT name FROM users")
	if !ok {
		t.Fatal("prepareJoinOriginsMSSQL rejected a single-table statement with an unqualified column, want accept")
	}
	if len(origins) != 1 {
		t.Fatalf("origins = %v, want 1 entry", origins)
	}
}

// TestPrepareJoinOriginsMSSQL_DerivedTableColumnUnresolvable documents
// that a derived table using `SELECT *` can't be traced by name (ADR
// 0011's one-level pass doesn't have the inner table's schema to know
// what `*` expands to) — the statement shape is still recognized
// (ok=true), the column just gets no origin.
func TestPrepareJoinOriginsMSSQL_DerivedTableColumnUnresolvable(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsMSSQL(db, store.DBKindMSSQL, "SELECT u.id FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id")
	if !ok {
		t.Fatal("prepareJoinOriginsMSSQL rejected a recognized derived-table JOIN shape")
	}
	if len(origins) != 1 || origins[0] != nil {
		t.Errorf("origins = %v, want a single nil entry (SELECT * can't be traced by name)", origins)
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

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

func TestCollectJoinTables_Matches(t *testing.T) {
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
			got, ok := collectJoinTables(sel.From)
			if !ok {
				t.Fatalf("collectJoinTables(%q) failed, want match", tt.stmt)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("collectJoinTables(%q) = %v, want %v", tt.stmt, got, tt.want)
			}
			for alias, table := range tt.want {
				if got[alias] != table {
					t.Errorf("alias %q -> %q, want %q", alias, got[alias], table)
				}
			}
		})
	}
}

func TestCollectJoinTables_RejectsDerivedTable(t *testing.T) {
	sel := mustParseSelect(t, "SELECT * FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id")
	if _, ok := collectJoinTables(sel.From); ok {
		t.Error("collectJoinTables matched a derived table, want reject")
	}
}

func TestPrepareJoinOrigins_NonMySQLBailsOut(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOrigins(db, store.DBKindPostgres, "SELECT u.id FROM users u JOIN orders o ON u.id = o.user_id")
	if ok || origins != nil {
		t.Errorf("prepareJoinOrigins on non-MySQL kind = (ok=%v, origins=%v), want (false, nil)", ok, origins)
	}
}

// TestPrepareJoinOrigins_GroupByBailsOutEntirely guards against a real
// regression found while verifying this against Docker MySQL: appending a
// hidden PK carrier column to a GROUP BY query can turn a query MySQL would
// otherwise run into an only_full_group_by error, and doing the same to a
// DISTINCT query would silently change which rows count as distinct. Both
// must leave the statement completely untouched.
func TestPrepareJoinOrigins_GroupByBailsOutEntirely(t *testing.T) {
	db := newTestDB(t)
	stmt := "SELECT u.name, COUNT(*) FROM users u JOIN orders o ON u.id = o.user_id GROUP BY u.name"
	rewritten, origins, ok := prepareJoinOrigins(db, testKind, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOrigins(GROUP BY) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

func TestPrepareJoinOrigins_DistinctBailsOutEntirely(t *testing.T) {
	db := newTestDB(t)
	stmt := "SELECT DISTINCT u.name FROM users u JOIN orders o ON u.id = o.user_id"
	rewritten, origins, ok := prepareJoinOrigins(db, testKind, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOrigins(DISTINCT) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

func TestPrepareJoinOrigins_NoJoinBailsOut(t *testing.T) {
	db := newTestDB(t)
	_, _, ok := prepareJoinOrigins(db, testKind, "SELECT id, name FROM users")
	if ok {
		t.Error("prepareJoinOrigins matched a single-table statement, want reject (that's ADR 0008/0009's job)")
	}
}

func TestPrepareJoinOrigins_DerivedTableBailsOut(t *testing.T) {
	db := newTestDB(t)
	_, _, ok := prepareJoinOrigins(db, testKind, "SELECT u.id FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id")
	if ok {
		t.Error("prepareJoinOrigins matched a derived table, want reject")
	}
}

// TestPrepareJoinOrigins_UnqualifiedColumnStaysUnknown documents the
// column-level fail-closed rule (ADR 0011): a column not qualified by a
// table alias is ambiguous in a JOIN and must not get an origin, even
// though the statement shape as a whole is recognized.
func TestPrepareJoinOrigins_UnqualifiedColumnStaysUnknown(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOrigins(db, testKind, "SELECT count FROM users u JOIN orders o ON u.id = o.user_id")
	if !ok {
		t.Fatal("prepareJoinOrigins rejected a recognized JOIN shape")
	}
	if len(origins) != 1 || origins[0] != nil {
		t.Errorf("origins = %v, want a single nil entry", origins)
	}
}

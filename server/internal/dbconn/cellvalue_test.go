package dbconn

import (
	"testing"

	"dbtool/server/internal/store"
)

func TestPlaceholder(t *testing.T) {
	tests := []struct {
		kind store.DBKind
		n    int
		want string
	}{
		{store.DBKindMySQL, 1, "?"},
		{store.DBKindMySQL, 2, "?"},
		{store.DBKindMSSQL, 1, "@p1"},
		{store.DBKindMSSQL, 2, "@p2"},
		{store.DBKindPostgres, 1, "$1"},
		{store.DBKindOracle, 1, ":1"},
	}
	for _, tt := range tests {
		if got := placeholder(tt.kind, tt.n); got != tt.want {
			t.Errorf("placeholder(%v, %d) = %q, want %q", tt.kind, tt.n, got, tt.want)
		}
	}
}

func TestBuildFetchCellQuery_SingleKey(t *testing.T) {
	query, args, err := buildFetchCellQuery(store.DBKindMySQL, "docs", "payload", map[string]any{"id": 42})
	if err != nil {
		t.Fatalf("buildFetchCellQuery: %v", err)
	}
	wantQuery := "SELECT `payload` FROM `docs` WHERE `id` = ?"
	if query != wantQuery {
		t.Errorf("query = %q, want %q", query, wantQuery)
	}
	if len(args) != 1 || args[0] != 42 {
		t.Errorf("args = %v, want [42]", args)
	}
}

func TestBuildFetchCellQuery_CompositeKeySortedForDeterminism(t *testing.T) {
	query, args, err := buildFetchCellQuery(store.DBKindPostgres, "docs", "payload", map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatalf("buildFetchCellQuery: %v", err)
	}
	wantQuery := `SELECT "payload" FROM "docs" WHERE "a" = $1 AND "b" = $2`
	if query != wantQuery {
		t.Errorf("query = %q, want %q", query, wantQuery)
	}
	if len(args) != 2 || args[0] != 1 || args[1] != 2 {
		t.Errorf("args = %v, want [1 2]", args)
	}
}

func TestBuildFetchCellQuery_RejectsEmptyPrimaryKey(t *testing.T) {
	if _, _, err := buildFetchCellQuery(store.DBKindMySQL, "docs", "payload", map[string]any{}); err == nil {
		t.Error("expected an error for an empty primary key, got nil")
	}
}

func TestBuildFetchCellQuery_MSSQLPlaceholders(t *testing.T) {
	query, _, err := buildFetchCellQuery(store.DBKindMSSQL, "docs", "payload", map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("buildFetchCellQuery: %v", err)
	}
	want := "SELECT [payload] FROM [docs] WHERE [id] = @p1"
	if query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
}

func TestBuildFetchCellQuery_OraclePlaceholders(t *testing.T) {
	query, _, err := buildFetchCellQuery(store.DBKindOracle, "docs", "payload", map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("buildFetchCellQuery: %v", err)
	}
	want := `SELECT "payload" FROM "docs" WHERE "id" = :1`
	if query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
}

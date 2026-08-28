package dbconn

import (
	"database/sql"
	"testing"

	"dbtool/server/internal/store"
)

func TestIsLOBType(t *testing.T) {
	notMax := sql.NullInt64{Valid: true, Int64: 4000}
	isMax := sql.NullInt64{Valid: true, Int64: -1}
	none := sql.NullInt64{}

	tests := []struct {
		kind     store.DBKind
		dataType string
		maxLen   sql.NullInt64
		want     bool
	}{
		{store.DBKindMSSQL, "nvarchar", notMax, false},
		{store.DBKindMSSQL, "nvarchar", isMax, true},
		{store.DBKindMSSQL, "varchar", isMax, true},
		{store.DBKindMSSQL, "varbinary", isMax, true},
		{store.DBKindMSSQL, "varbinary", notMax, false},
		{store.DBKindMSSQL, "xml", none, true},
		{store.DBKindMSSQL, "text", none, true},
		{store.DBKindMSSQL, "ntext", none, true},
		{store.DBKindMSSQL, "image", none, true},
		{store.DBKindMSSQL, "int", none, false},
		{store.DBKindMySQL, "text", none, true},
		{store.DBKindMySQL, "longtext", none, true},
		{store.DBKindMySQL, "blob", none, true},
		{store.DBKindMySQL, "varchar", notMax, false},
		{store.DBKindPostgres, "text", none, true},
		{store.DBKindPostgres, "xml", none, true},
		{store.DBKindPostgres, "bytea", none, true},
		{store.DBKindPostgres, "varchar", notMax, false},
		{store.DBKindOracle, "clob", none, true},
		{store.DBKindOracle, "nclob", none, true},
		{store.DBKindOracle, "blob", none, true},
		{store.DBKindOracle, "long", none, true},
		{store.DBKindOracle, "varchar2", notMax, false},
	}
	for _, tt := range tests {
		got := isLOBType(tt.kind, tt.dataType, tt.maxLen)
		if got != tt.want {
			t.Errorf("isLOBType(%v, %q, %v) = %v, want %v", tt.kind, tt.dataType, tt.maxLen, got, tt.want)
		}
	}
}

func TestColumnSelectExpr_NonLOB(t *testing.T) {
	got := columnSelectExpr(store.DBKindMSSQL, "id", "int", false)
	want := "[id]"
	if got != want {
		t.Errorf("columnSelectExpr() = %q, want %q", got, want)
	}
}

func TestColumnSelectExpr_LOB(t *testing.T) {
	tests := []struct {
		kind     store.DBKind
		dataType string
		want     string
	}{
		{store.DBKindMSSQL, "xml", `SUBSTRING(CAST([payload] AS nvarchar(max)), 1, 2000) AS [payload]`},
		{store.DBKindMSSQL, "nvarchar", `SUBSTRING([payload], 1, 2000) AS [payload]`},
		{store.DBKindMySQL, "longtext", "SUBSTRING(`payload`, 1, 2000) AS `payload`"},
		{store.DBKindPostgres, "xml", `substring("payload"::text, 1, 2000) AS "payload"`},
		{store.DBKindPostgres, "text", `substring("payload", 1, 2000) AS "payload"`},
		{store.DBKindOracle, "clob", `DBMS_LOB.SUBSTR("payload", 2000, 1) AS "payload"`},
		{store.DBKindOracle, "long", `SUBSTR("payload", 1, 2000) AS "payload"`},
	}
	for _, tt := range tests {
		got := columnSelectExpr(tt.kind, "payload", tt.dataType, true)
		if got != tt.want {
			t.Errorf("columnSelectExpr(%v, %q) = %q, want %q", tt.kind, tt.dataType, got, tt.want)
		}
	}
}

func TestQuoteIdent(t *testing.T) {
	tests := []struct {
		kind store.DBKind
		name string
		want string
	}{
		{store.DBKindMSSQL, "col", "[col]"},
		{store.DBKindMSSQL, "we]ird", "[we]]ird]"},
		{store.DBKindMySQL, "col", "`col`"},
		{store.DBKindPostgres, "col", `"col"`},
		{store.DBKindOracle, "col", `"col"`},
	}
	for _, tt := range tests {
		if got := quoteIdent(tt.kind, tt.name); got != tt.want {
			t.Errorf("quoteIdent(%v, %q) = %q, want %q", tt.kind, tt.name, got, tt.want)
		}
	}
}

package query

import (
	"database/sql"
	"fmt"
	"unicode/utf8"

	"dbtool/server/internal/store"
)

// DefaultRowLimit caps how many rows a read (SELECT-class) statement
// returns, per the "server-side 기본 LIMIT" decision — protecting against
// accidentally pulling millions of rows into memory.
const DefaultRowLimit = 1000

// maxCellBytes caps how many bytes of a single cell's string
// representation are sent to the client, per ADR 0007 — BLOB/XML columns
// otherwise blow up response size and grid rendering time.
const maxCellBytes = 2000

// truncateCell shortens s to maxCellBytes (on a valid UTF-8 boundary) and
// appends a marker noting the original size, per ADR 0007. Values at or
// under the limit are returned unchanged.
func truncateCell(s string) string {
	if len(s) <= maxCellBytes {
		return s
	}
	cut := s[:maxCellBytes]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return fmt.Sprintf("%s...(잘림, 원본 %d바이트)", cut, len(s))
}

// Result is what a single query execution produces, in a shape a REST
// handler can serialize directly.
type Result struct {
	Columns      []string `json:"columns,omitempty"`
	Rows         [][]any  `json:"rows,omitempty"`
	Truncated    bool     `json:"truncated,omitempty"`
	RowsAffected int64    `json:"rowsAffected,omitempty"`
	// Table and PrimaryKey are set only when stmt matched the same narrow
	// `SELECT * FROM <table>` shape ADR 0008 rewrites, and that table has
	// a primary key — the two facts ADR 0009's cell-value download needs
	// to safely re-fetch one row's untruncated value. Left empty
	// otherwise (e.g. JOINs, tables without a primary key): the client
	// treats that as "download isn't available for this result".
	Table      string   `json:"table,omitempty"`
	PrimaryKey []string `json:"primaryKey,omitempty"`
}

// Execute runs stmt against db (open for kind). Read statements are capped
// at DefaultRowLimit rows and, when they match the narrow `SELECT * FROM
// <table>` shape ADR 0008 describes, rewritten to pre-truncate any
// LOB-class column on the DB server itself. Write statements run via Exec
// and report the affected row count.
func Execute(db *sql.DB, kind store.DBKind, stmt string) (*Result, error) {
	if IsWrite(stmt) {
		return executeWrite(db, stmt)
	}
	return executeRead(db, kind, stmt)
}

func executeWrite(db *sql.DB, stmt string) (*Result, error) {
	res, err := db.Exec(stmt)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		// Not every driver reports this; that's fine, just omit it.
		return &Result{}, nil
	}
	return &Result{RowsAffected: affected}, nil
}

func executeRead(db *sql.DB, kind store.DBKind, stmt string) (*Result, error) {
	rewritten, table, primaryKey := prepareSelectStar(db, kind, stmt)

	rows, err := db.Query(rewritten)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := &Result{Columns: cols, Rows: [][]any{}, Table: table, PrimaryKey: primaryKey}
	for rows.Next() {
		if len(result.Rows) >= DefaultRowLimit {
			result.Truncated = true
			break
		}
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, v := range raw {
			switch val := v.(type) {
			case []byte:
				// Drivers (notably MySQL) return string-typed columns as
				// []byte; left as-is, encoding/json base64-encodes them
				// instead of emitting readable text.
				raw[i] = truncateCell(string(val))
			case string:
				// Other drivers (e.g. Postgres for TEXT/XML) hand back a
				// native string directly, so it needs the same cap.
				raw[i] = truncateCell(val)
			}
		}
		result.Rows = append(result.Rows, raw)
	}
	return result, rows.Err()
}

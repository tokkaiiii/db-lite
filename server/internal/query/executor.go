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

// TruncateCell shortens s to maxCellBytes (on a valid UTF-8 boundary) and
// appends a marker noting the original size, per ADR 0007. Values at or
// under the limit are returned unchanged. Exported so the cell-download
// handler can build the same text a grid cell shows, to cross-check against
// a re-fetched value (ADR 0011).
func TruncateCell(s string) string {
	if len(s) <= maxCellBytes {
		return s
	}
	cut := s[:maxCellBytes]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return fmt.Sprintf("%s...(잘림, 원본 %d바이트)", cut, len(s))
}

// ColumnOrigin says which table one result column's value came from and how
// to find the exact database row it belongs to, for a cell-value download
// (ADR 0009 / ADR 0011): PrimaryKeyColumns names that table's PK columns,
// and PrimaryKeyRowIndexes says which position in the *same* row's slice
// holds each one's value — which may point past len(Result.Columns), into a
// hidden carrier column a JOIN rewrite appended (see prepareJoinOrigins).
type ColumnOrigin struct {
	Table                string   `json:"table"`
	PrimaryKeyColumns    []string `json:"primaryKeyColumns"`
	PrimaryKeyRowIndexes []int    `json:"primaryKeyRowIndexes"`
}

// Result is what a single query execution produces, in a shape a REST
// handler can serialize directly.
type Result struct {
	Columns      []string `json:"columns,omitempty"`
	Rows         [][]any  `json:"rows,omitempty"`
	Truncated    bool     `json:"truncated,omitempty"`
	RowsAffected int64    `json:"rowsAffected,omitempty"`
	// ColumnOrigins has one entry per Columns entry (nil where the origin
	// couldn't be pinned down — an aggregate/expression result, an
	// unqualified column in a JOIN, or a table without a primary key).
	// Rows may carry extra trailing cells beyond len(Columns): hidden PK
	// carriers a JOIN rewrite injected purely so a cell download can
	// re-fetch that row later — never meant to be displayed. The client
	// treats a nil origin as "download isn't available for this cell".
	ColumnOrigins []*ColumnOrigin `json:"columnOrigins,omitempty"`
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
	plan := prepareRead(db, kind, stmt)

	rows, err := db.Query(plan.rewritten)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	visibleCount := len(cols)
	var origins []*ColumnOrigin
	switch {
	case plan.singleTable != "":
		// ADR 0008/0009: `SELECT *` already returns every real column
		// (including any PK), so nothing's hidden — just find where the PK
		// columns landed in the driver's actual column order.
		origins = uniformOrigins(cols, plan.singleTable, plan.singlePK)
	case plan.joinOrigins != nil:
		// ADR 0011: origins (and any hidden PK carrier positions) were
		// already fully resolved statically while building the rewrite.
		visibleCount = len(plan.joinOrigins)
		origins = plan.joinOrigins
	}
	if visibleCount > len(cols) {
		// Should never happen, but a hidden-column bookkeeping bug here
		// must not corrupt what the client sees — fail safe to "no
		// downloads for this result" rather than misdisplay/miscount rows.
		visibleCount, origins = len(cols), nil
	}

	result := &Result{Columns: cols[:visibleCount], Rows: [][]any{}, ColumnOrigins: origins}
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
				raw[i] = TruncateCell(string(val))
			case string:
				// Other drivers (e.g. Postgres for TEXT/XML) hand back a
				// native string directly, so it needs the same cap.
				raw[i] = TruncateCell(val)
			}
		}
		result.Rows = append(result.Rows, raw)
	}
	return result, rows.Err()
}

// readPlan is what prepareRead decides for one statement: the SQL to
// actually run, and (at most one of) the two download paths' bookkeeping —
// executeRead resolves this into Result.ColumnOrigins once it knows the
// driver's actual column list.
type readPlan struct {
	rewritten   string
	singleTable string          // ADR 0008/0009 path; "" if not this shape
	singlePK    []string        // only meaningful when singleTable != ""
	joinOrigins []*ColumnOrigin // ADR 0011 path; nil if not this shape
}

// prepareRead inspects stmt once and decides which download path (if any)
// applies. It tries the ADR 0008/0009 single-table `SELECT * FROM <table>`
// shape first (cheap, no parser involved), and only falls back to the
// ADR 0011 JOIN-parsing path when that narrower shape doesn't match.
func prepareRead(db *sql.DB, kind store.DBKind, stmt string) readPlan {
	rewritten, table, primaryKey := prepareSelectStar(db, kind, stmt)
	if table != "" {
		return readPlan{rewritten: rewritten, singleTable: table, singlePK: primaryKey}
	}

	if rw, joinOrigins, ok := prepareJoinOrigins(db, kind, stmt); ok {
		return readPlan{rewritten: rw, joinOrigins: joinOrigins}
	}

	return readPlan{rewritten: stmt}
}

// uniformOrigins builds the ADR 0008/0009 single-table case's
// ColumnOrigins: every column shares the same table+PK, so all that's
// needed is finding where each PK column landed in the driver's actual
// column order (cols).
func uniformOrigins(cols []string, table string, primaryKey []string) []*ColumnOrigin {
	if table == "" || len(primaryKey) == 0 {
		return nil
	}
	idxs := make([]int, len(primaryKey))
	for i, pkCol := range primaryKey {
		idxs[i] = indexOfString(cols, pkCol)
	}
	origins := make([]*ColumnOrigin, len(cols))
	for i := range cols {
		origins[i] = &ColumnOrigin{Table: table, PrimaryKeyColumns: primaryKey, PrimaryKeyRowIndexes: idxs}
	}
	return origins
}

func indexOfString(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

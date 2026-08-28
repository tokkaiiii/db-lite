package query

import "database/sql"

// DefaultRowLimit caps how many rows a read (SELECT-class) statement
// returns, per the "server-side 기본 LIMIT" decision — protecting against
// accidentally pulling millions of rows into memory.
const DefaultRowLimit = 1000

// Result is what a single query execution produces, in a shape a REST
// handler can serialize directly.
type Result struct {
	Columns      []string `json:"columns,omitempty"`
	Rows         [][]any  `json:"rows,omitempty"`
	Truncated    bool     `json:"truncated,omitempty"`
	RowsAffected int64    `json:"rowsAffected,omitempty"`
}

// Execute runs stmt against db. Read statements are capped at
// DefaultRowLimit rows; write statements run via Exec and report the
// affected row count.
func Execute(db *sql.DB, stmt string) (*Result, error) {
	if IsWrite(stmt) {
		return executeWrite(db, stmt)
	}
	return executeRead(db, stmt)
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

func executeRead(db *sql.DB, stmt string) (*Result, error) {
	rows, err := db.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := &Result{Columns: cols, Rows: [][]any{}}
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
			// Drivers (notably MySQL) return string-typed columns as
			// []byte; left as-is, encoding/json base64-encodes them
			// instead of emitting readable text.
			if b, ok := v.([]byte); ok {
				raw[i] = string(b)
			}
		}
		result.Rows = append(result.Rows, raw)
	}
	return result, rows.Err()
}

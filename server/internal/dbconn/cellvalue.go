package dbconn

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"dbtool/server/internal/store"
)

// placeholder returns the dialect's positional bind-parameter syntax for
// the n-th (1-based) parameter in a query.
func placeholder(kind store.DBKind, n int) string {
	switch kind {
	case store.DBKindMSSQL:
		return fmt.Sprintf("@p%d", n)
	case store.DBKindPostgres:
		return fmt.Sprintf("$%d", n)
	case store.DBKindOracle:
		return fmt.Sprintf(":%d", n)
	default: // MySQL
		return "?"
	}
}

// FetchCellValue re-runs a single-column, single-row SELECT identified by
// primaryKey against table/column (bare, unqualified names) and returns
// that column's untruncated value — see ADR 0009. table and column are
// quoted per dialect so a name colliding with a reserved word still works;
// primaryKey values are passed as bind parameters, never interpolated
// into the SQL text, so arbitrary primary key values can't inject SQL.
// Callers are responsible for only calling this against a table they
// already know has a primary key (see query.Result.Table/PrimaryKey).
func FetchCellValue(db *sql.DB, kind store.DBKind, table, column string, primaryKey map[string]any) (any, error) {
	query, args, err := buildFetchCellQuery(kind, table, column, primaryKey)
	if err != nil {
		return nil, err
	}

	var value any
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// buildFetchCellQuery builds the SELECT and bind arguments FetchCellValue
// runs. Split out so the SQL text itself can be unit tested without a
// live DB.
func buildFetchCellQuery(kind store.DBKind, table, column string, primaryKey map[string]any) (query string, args []any, err error) {
	if len(primaryKey) == 0 {
		return "", nil, fmt.Errorf("primary key value required")
	}

	keys := make([]string, 0, len(primaryKey))
	for k := range primaryKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	conditions := make([]string, len(keys))
	args = make([]any, len(keys))
	for i, k := range keys {
		conditions[i] = fmt.Sprintf("%s = %s", quoteIdent(kind, k), placeholder(kind, i+1))
		args[i] = primaryKey[k]
	}

	query = fmt.Sprintf("SELECT %s FROM %s WHERE %s",
		quoteIdent(kind, column), quoteIdent(kind, table), strings.Join(conditions, " AND "))
	return query, args, nil
}

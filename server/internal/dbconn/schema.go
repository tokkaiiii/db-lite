package dbconn

import (
	"database/sql"
	"fmt"

	"dbtool/server/internal/store"
)

// schemaQueries lists every (table, column) pair in the one schema each DB
// kind treats as "the" default within a Catalog — MSSQL's dbo, Postgres's
// public, and for Oracle (which has no Catalog) the connecting user's own
// tables. Ordered by table then column position, so groupColumns doesn't
// need to sort.
var schemaQueries = map[store.DBKind]string{
	store.DBKindMySQL: `
		SELECT TABLE_NAME, COLUMN_NAME FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		ORDER BY TABLE_NAME, ORDINAL_POSITION`,
	store.DBKindMSSQL: `
		SELECT TABLE_NAME, COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = 'dbo'
		ORDER BY TABLE_NAME, ORDINAL_POSITION`,
	store.DBKindPostgres: `
		SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = 'public'
		ORDER BY table_name, ordinal_position`,
	store.DBKindOracle: `
		SELECT TABLE_NAME, COLUMN_NAME FROM USER_TAB_COLUMNS
		ORDER BY TABLE_NAME, COLUMN_ID`,
}

// DescribeSchema returns every table's columns on db (already open for
// kind), keyed by table name, in column order — shaped for direct use as a
// CodeMirror SQL completion schema on the client.
func DescribeSchema(db *sql.DB, kind store.DBKind) (map[string][]string, error) {
	query, ok := schemaQueries[kind]
	if !ok {
		return map[string][]string{}, nil
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("describe schema: %w", err)
	}
	defer rows.Close()

	schema := map[string][]string{}
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return nil, err
		}
		schema[table] = append(schema[table], column)
	}
	return schema, rows.Err()
}

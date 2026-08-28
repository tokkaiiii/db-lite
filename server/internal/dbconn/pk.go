package dbconn

import (
	"database/sql"

	"dbtool/server/internal/store"
)

// primaryKeyQueries finds a table's primary key column names, in key
// ordinal order — used only to identify a row well enough to re-fetch one
// column's untruncated value (see ADR 0009). Scoped to the same single
// default schema as schemaQueries/lobColumnQueries.
var primaryKeyQueries = map[store.DBKind]string{
	store.DBKindMySQL: `
		SELECT k.COLUMN_NAME
		FROM information_schema.TABLE_CONSTRAINTS t
		JOIN information_schema.KEY_COLUMN_USAGE k
		  ON k.CONSTRAINT_NAME = t.CONSTRAINT_NAME AND k.TABLE_SCHEMA = t.TABLE_SCHEMA
		WHERE t.CONSTRAINT_TYPE = 'PRIMARY KEY'
		  AND t.TABLE_SCHEMA = DATABASE() AND t.TABLE_NAME = ?
		ORDER BY k.ORDINAL_POSITION`,
	store.DBKindMSSQL: `
		SELECT k.COLUMN_NAME
		FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS t
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE k
		  ON k.CONSTRAINT_NAME = t.CONSTRAINT_NAME AND k.TABLE_SCHEMA = t.TABLE_SCHEMA
		WHERE t.CONSTRAINT_TYPE = 'PRIMARY KEY'
		  AND t.TABLE_SCHEMA = 'dbo' AND t.TABLE_NAME = @p1
		ORDER BY k.ORDINAL_POSITION`,
	store.DBKindPostgres: `
		SELECT k.column_name
		FROM information_schema.table_constraints t
		JOIN information_schema.key_column_usage k
		  ON k.constraint_name = t.constraint_name AND k.table_schema = t.table_schema
		WHERE t.constraint_type = 'PRIMARY KEY'
		  AND t.table_schema = 'public' AND t.table_name = $1
		ORDER BY k.ordinal_position`,
	store.DBKindOracle: `
		SELECT cc.COLUMN_NAME
		FROM USER_CONSTRAINTS c
		JOIN USER_CONS_COLUMNS cc ON cc.CONSTRAINT_NAME = c.CONSTRAINT_NAME
		WHERE c.CONSTRAINT_TYPE = 'P' AND c.TABLE_NAME = :1
		ORDER BY cc.POSITION`,
}

// PrimaryKeyColumns returns the primary key column names of table (bare,
// unqualified, unquoted — see query.bareTableName) on db, already open for
// kind, in key order. A nil result with a nil error means the table has no
// primary key (or wasn't found): callers should treat that as "this
// feature isn't available for this table", not as an error.
func PrimaryKeyColumns(db *sql.DB, kind store.DBKind, table string) ([]string, error) {
	q, ok := primaryKeyQueries[kind]
	if !ok {
		return nil, nil
	}

	rows, err := db.Query(q, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

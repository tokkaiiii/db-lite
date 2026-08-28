package dbconn

import (
	"database/sql"
	"fmt"

	"dbtool/server/internal/store"
)

// catalogQueries returns the names of every user Catalog on the server db
// is connected to — excluding each DB kind's own built-in system/maintenance
// databases, which a User querying their own data has no reason to pick.
var catalogQueries = map[store.DBKind]string{
	store.DBKindMySQL: `
		SELECT SCHEMA_NAME FROM information_schema.SCHEMATA
		WHERE SCHEMA_NAME NOT IN ('information_schema', 'mysql', 'performance_schema', 'sys')
		ORDER BY SCHEMA_NAME`,
	store.DBKindMSSQL: `
		SELECT name FROM sys.databases
		WHERE name NOT IN ('master', 'model', 'msdb', 'tempdb')
		ORDER BY name`,
	store.DBKindPostgres: `
		SELECT datname FROM pg_database
		WHERE NOT datistemplate
		ORDER BY datname`,
}

// ListCatalogs returns the Catalogs available on db, which must already be
// open (via Open) for kind. Oracle has no Catalog concept — its service
// name fixes the target PDB at connect time — so it always returns an
// empty list rather than an error.
func ListCatalogs(db *sql.DB, kind store.DBKind) ([]string, error) {
	query, ok := catalogQueries[kind]
	if !ok {
		return []string{}, nil
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list catalogs: %w", err)
	}
	defer rows.Close()

	catalogs := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		catalogs = append(catalogs, name)
	}
	return catalogs, rows.Err()
}

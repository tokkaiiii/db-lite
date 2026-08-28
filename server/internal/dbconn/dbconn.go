// Package dbconn opens a *sql.DB for a store.Connection, hiding the
// per-DBKind driver name and DSN format behind one function.
package dbconn

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	_ "github.com/sijms/go-ora/v2"

	"dbtool/server/internal/store"
)

// postgresDefaultCatalog is the maintenance database PostgreSQL always
// provisions — used when no Catalog has been selected yet (e.g. while
// listing Catalogs, or for a caller that predates Catalog selection).
// Unlike MySQL/MSSQL, Postgres has no "no database" connection mode: every
// connection targets exactly one database from the start.
const postgresDefaultCatalog = "postgres"

// Open returns a *sql.DB for the given Connection, connected to catalog —
// the individual database/schema within that Connection's server instance
// the caller wants to run statements against (see the Catalog glossary
// entry in CONTEXT.md). Pass "" to connect without picking one, which only
// MySQL and MSSQL support; Oracle ignores catalog entirely, since its
// service name already fixes the target PDB. Callers are expected to
// pool/reuse or close the result — this does not cache connections itself.
func Open(c store.Connection, catalog string) (*sql.DB, error) {
	driver, dsn, err := driverAndDSN(c, catalog)
	if err != nil {
		return nil, err
	}
	return sql.Open(driver, dsn)
}

func driverAndDSN(c store.Connection, catalog string) (driver, dsn string, err error) {
	switch c.Kind {
	case store.DBKindMSSQL:
		dsn = fmt.Sprintf("sqlserver://%s:%s@%s:%d", c.Username, c.Password, c.Host, c.Port)
		if catalog != "" {
			dsn += "?database=" + catalog
		}
		return "sqlserver", dsn, nil
	case store.DBKindMySQL:
		return "mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
			c.Username, c.Password, c.Host, c.Port, catalog), nil
	case store.DBKindPostgres:
		if catalog == "" {
			catalog = postgresDefaultCatalog
		}
		return "pgx", fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
			c.Username, c.Password, c.Host, c.Port, catalog), nil
	case store.DBKindOracle:
		if c.ServiceName == "" {
			return "", "", fmt.Errorf("oracle connection %q has no service name/SID configured", c.Name)
		}
		return "oracle", fmt.Sprintf("oracle://%s:%s@%s:%d/%s",
			c.Username, c.Password, c.Host, c.Port, c.ServiceName), nil
	default:
		return "", "", fmt.Errorf("unsupported db kind: %q", c.Kind)
	}
}

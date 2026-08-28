// Package dbconn opens a *sql.DB for a store.Connection, hiding the
// per-DBKind driver name and DSN format behind one function.
package dbconn

import (
	"database/sql"
	"fmt"
	"net/url"

	mysqldriver "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	goora "github.com/sijms/go-ora/v2"

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

// driverAndDSN builds each DSN through its driver's own structured
// builder (mysqldriver.Config, goora.BuildUrl) or net/url.URL, rather than
// interpolating username/password into a string directly — a Connection's
// password is arbitrary user input and may contain '@', '/', '?', '#', or
// other characters that are only safe once properly escaped.
func driverAndDSN(c store.Connection, catalog string) (driver, dsn string, err error) {
	switch c.Kind {
	case store.DBKindMSSQL:
		u := url.URL{
			Scheme: "sqlserver",
			User:   url.UserPassword(c.Username, c.Password),
			Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
		}
		if catalog != "" {
			u.RawQuery = url.Values{"database": {catalog}}.Encode()
		}
		return "sqlserver", u.String(), nil
	case store.DBKindMySQL:
		cfg := mysqldriver.NewConfig()
		cfg.User = c.Username
		cfg.Passwd = c.Password
		cfg.Net = "tcp"
		cfg.Addr = fmt.Sprintf("%s:%d", c.Host, c.Port)
		cfg.DBName = catalog
		return "mysql", cfg.FormatDSN(), nil
	case store.DBKindPostgres:
		if catalog == "" {
			catalog = postgresDefaultCatalog
		}
		u := url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(c.Username, c.Password),
			Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
			Path:   "/" + catalog,
		}
		return "pgx", u.String(), nil
	case store.DBKindOracle:
		if c.ServiceName == "" {
			return "", "", fmt.Errorf("oracle connection %q has no service name/SID configured", c.Name)
		}
		return "oracle", goora.BuildUrl(c.Host, c.Port, c.ServiceName, c.Username, c.Password, nil), nil
	default:
		return "", "", fmt.Errorf("unsupported db kind: %q", c.Kind)
	}
}

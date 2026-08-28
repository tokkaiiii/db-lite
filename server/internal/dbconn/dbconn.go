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

// Open returns a *sql.DB for the given Connection. Callers are expected to
// pool/reuse or close it — this does not cache connections itself.
func Open(c store.Connection) (*sql.DB, error) {
	driver, dsn, err := driverAndDSN(c)
	if err != nil {
		return nil, err
	}
	return sql.Open(driver, dsn)
}

func driverAndDSN(c store.Connection) (driver, dsn string, err error) {
	switch c.Kind {
	case store.DBKindMSSQL:
		return "sqlserver", fmt.Sprintf("sqlserver://%s:%s@%s:%d",
			c.Username, c.Password, c.Host, c.Port), nil
	case store.DBKindMySQL:
		return "mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/",
			c.Username, c.Password, c.Host, c.Port), nil
	case store.DBKindPostgres:
		return "pgx", fmt.Sprintf("postgres://%s:%s@%s:%d/postgres",
			c.Username, c.Password, c.Host, c.Port), nil
	case store.DBKindOracle:
		return "oracle", fmt.Sprintf("oracle://%s:%s@%s:%d/",
			c.Username, c.Password, c.Host, c.Port), nil
	default:
		return "", "", fmt.Errorf("unsupported db kind: %q", c.Kind)
	}
}

package query

import (
	"database/sql"

	"dbtool/server/internal/store"
)

// prepareJoinOrigins is the ADR 0011 "JOIN" download path's entry point.
// ADR 0009 already covers PK-having single-table `SELECT *` via a narrow
// regex (rewrite.go); this handles the next narrowest shape a real parser
// can analyze safely: a SELECT over a plain multi-table JOIN (no derived
// tables/subqueries in FROM), where each output column is a simple
// `alias.column` reference. Each DB kind needs its own parser (dialects
// differ too much for one to cover all of them — see ADR 0011), so this
// just dispatches to the one for kind.
//
// ok is false whenever stmt isn't a shape the matching dialect's pass
// understands (parse failure, no JOIN, a derived table, `SELECT *`, no
// parser for kind yet): callers should treat the statement as having no
// origins, same as before ADR 0011.
func prepareJoinOrigins(db *sql.DB, kind store.DBKind, stmt string) (rewritten string, origins []*ColumnOrigin, ok bool) {
	switch kind {
	case store.DBKindMySQL:
		return prepareJoinOriginsMySQL(db, kind, stmt)
	case store.DBKindPostgres:
		return prepareJoinOriginsPostgres(db, kind, stmt)
	case store.DBKindMSSQL:
		return prepareJoinOriginsMSSQL(db, kind, stmt)
	case store.DBKindOracle:
		return prepareJoinOriginsOracle(db, kind, stmt)
	default:
		return stmt, nil, false
	}
}

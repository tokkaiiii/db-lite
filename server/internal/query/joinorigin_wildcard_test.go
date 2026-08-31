package query

import (
	"testing"

	"dbtool/server/internal/store"
)

// TestPrepareJoinOrigins_BareStarAcrossJoinFailsOpenWithoutSchema_AllDialects
// exercises a bare `SELECT *` JOIN against this test's SQLite-backed DB,
// which can't run any dialect's schema-lookup query (MySQL/Postgres/MSSQL
// information_schema syntax, Oracle's USER_TAB_COLUMNS) — so
// expandWildcardOrigins's column-count lookup fails for every dialect and
// origin tracking is abandoned (ok=false, statement left untouched). This
// is the *fail-open* path, not a deliberate "bare `*` isn't supported"
// rejection — see docs/adr/0011 for real-schema verification (Docker) of
// the success path, and the regression this guards: a wrong column count
// here must never corrupt the query itself, only leave downloads
// unavailable.
func TestPrepareJoinOrigins_BareStarAcrossJoinFailsOpenWithoutSchema_AllDialects(t *testing.T) {
	db := newTestDB(t)
	stmt := `SELECT * FROM users u JOIN orders o ON u.id = o.user_id`
	for _, kind := range []store.DBKind{store.DBKindMySQL, store.DBKindPostgres, store.DBKindMSSQL, store.DBKindOracle} {
		t.Run(string(kind), func(t *testing.T) {
			rewritten, origins, ok := prepareJoinOrigins(db, kind, stmt)
			if ok || origins != nil || rewritten != stmt {
				t.Errorf("prepareJoinOrigins(%v, bare SELECT *) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", kind, rewritten, origins, ok)
			}
		})
	}
}

// TestPrepareJoinOrigins_QualifiedStarAcrossJoinBailsOut_AllDialects
// documents that `alias.*` (as opposed to a bare whole-list `*`) stays
// unsupported — see prepareJoinOriginsMySQL's wildcard guard.
func TestPrepareJoinOrigins_QualifiedStarAcrossJoinBailsOut_AllDialects(t *testing.T) {
	db := newTestDB(t)
	stmt := `SELECT u.*, o.total FROM users u JOIN orders o ON u.id = o.user_id`
	for _, kind := range []store.DBKind{store.DBKindMySQL, store.DBKindPostgres, store.DBKindMSSQL, store.DBKindOracle} {
		t.Run(string(kind), func(t *testing.T) {
			rewritten, origins, ok := prepareJoinOrigins(db, kind, stmt)
			if ok || origins != nil || rewritten != stmt {
				t.Errorf("prepareJoinOrigins(%v, alias.*) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", kind, rewritten, origins, ok)
			}
		})
	}
}

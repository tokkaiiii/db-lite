package query

import (
	"testing"

	"dbtool/server/internal/store"
)

func TestPrepareJoinOrigins_BareStarAcrossJoinBailsOut_AllDialects(t *testing.T) {
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

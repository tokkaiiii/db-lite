package query

import (
	"testing"

	"github.com/antlr4-go/antlr/v4"
	plsqlparser "github.com/bytebase/plsql-parser"

	"dbtool/server/internal/store"
)

// mustParseQueryBlockOracle parses stmt down to its Query_block, the same
// path prepareJoinOriginsOracle takes for a plain (non-UNION) SELECT — see
// that function for why each step is checked.
func mustParseQueryBlockOracle(t *testing.T, stmt string) plsqlparser.IQuery_blockContext {
	t.Helper()
	input := antlr.NewInputStream(stmt)
	lexer := plsqlparser.NewPlSqlLexer(input)
	errs := &countingErrorListener{}
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errs)
	tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := plsqlparser.NewPlSqlParser(tokens)
	p.SetVersion12(true)
	p.RemoveErrorListeners()
	p.AddErrorListener(errs)
	p.BuildParseTrees = true

	selStmt := p.Select_statement()
	if errs.errors > 0 {
		t.Fatalf("parse %q: %d syntax error(s)", stmt, errs.errors)
	}
	qb := selStmt.Select_only_statement().Subquery().Subquery_basic_elements().Query_block()
	if qb == nil {
		t.Fatalf("parse %q: not a plain query block", stmt)
	}
	return qb
}

func TestCollectJoinTablesOracle_Matches(t *testing.T) {
	tests := []struct {
		name string
		stmt string
		want map[string]string
	}{
		{
			"aliased join",
			`SELECT * FROM users u JOIN orders o ON u.id = o.user_id`,
			map[string]string{"u": "users", "o": "orders"},
		},
		{
			"unaliased join uses table name",
			`SELECT * FROM users JOIN orders ON users.id = orders.user_id`,
			map[string]string{"users": "users", "orders": "orders"},
		},
		{
			"self join distinguishes aliases",
			`SELECT * FROM users a JOIN users b ON a.manager_id = b.id`,
			map[string]string{"a": "users", "b": "users"},
		},
		{
			"comma join",
			`SELECT * FROM users u, orders o WHERE u.id = o.user_id`,
			map[string]string{"u": "users", "o": "orders"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := mustParseQueryBlockOracle(t, tt.stmt)
			got, ok := collectJoinTablesOracle(qb.From_clause().Table_ref_list())
			if !ok {
				t.Fatalf("collectJoinTablesOracle(%q) failed, want match", tt.stmt)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("collectJoinTablesOracle(%q) = %v, want %v", tt.stmt, got, tt.want)
			}
			for alias, table := range tt.want {
				if got[alias] != table {
					t.Errorf("alias %q -> %q, want %q", alias, got[alias], table)
				}
			}
		})
	}
}

func TestCollectJoinTablesOracle_RejectsDerivedTable(t *testing.T) {
	qb := mustParseQueryBlockOracle(t, `SELECT * FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id`)
	if _, ok := collectJoinTablesOracle(qb.From_clause().Table_ref_list()); ok {
		t.Error("collectJoinTablesOracle matched a derived table, want reject")
	}
}

func TestPrepareJoinOriginsOracle_NoJoinBailsOut(t *testing.T) {
	_, _, ok := prepareJoinOriginsOracle(nil, store.DBKindOracle, `SELECT id, name FROM users`)
	if ok {
		t.Error("prepareJoinOriginsOracle matched a single-table statement, want reject")
	}
}

func TestPrepareJoinOriginsOracle_DerivedTableBailsOut(t *testing.T) {
	_, _, ok := prepareJoinOriginsOracle(nil, store.DBKindOracle, `SELECT u.id FROM (SELECT * FROM users) u JOIN orders o ON u.id = o.user_id`)
	if ok {
		t.Error("prepareJoinOriginsOracle matched a derived table, want reject")
	}
}

func TestPrepareJoinOriginsOracle_GroupByBailsOutEntirely(t *testing.T) {
	stmt := `SELECT u.name, COUNT(*) FROM users u JOIN orders o ON u.id = o.user_id GROUP BY u.name`
	rewritten, origins, ok := prepareJoinOriginsOracle(nil, store.DBKindOracle, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOriginsOracle(GROUP BY) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

func TestPrepareJoinOriginsOracle_DistinctBailsOutEntirely(t *testing.T) {
	stmt := `SELECT DISTINCT u.name FROM users u JOIN orders o ON u.id = o.user_id`
	rewritten, origins, ok := prepareJoinOriginsOracle(nil, store.DBKindOracle, stmt)
	if ok || origins != nil || rewritten != stmt {
		t.Errorf("prepareJoinOriginsOracle(DISTINCT) = (rewritten=%q, origins=%v, ok=%v), want (stmt unchanged, nil, false)", rewritten, origins, ok)
	}
}

// TestPrepareJoinOriginsOracle_UnqualifiedColumnStaysUnknown documents the
// column-level fail-closed rule (ADR 0011): a column not qualified by a
// table alias is ambiguous in a JOIN and must not get an origin, even
// though the statement shape as a whole is recognized.
func TestPrepareJoinOriginsOracle_UnqualifiedColumnStaysUnknown(t *testing.T) {
	_, origins, ok := prepareJoinOriginsOracle(nil, store.DBKindOracle, `SELECT cnt FROM users u JOIN orders o ON u.id = o.user_id`)
	if !ok {
		t.Fatal("prepareJoinOriginsOracle rejected a recognized JOIN shape")
	}
	if len(origins) != 1 || origins[0] != nil {
		t.Errorf("origins = %v, want a single nil entry", origins)
	}
}

// TestPrepareJoinOriginsOracle_NoPKStillParses exercises the pass with a
// real (in-memory) DB so dbconn.PrimaryKeyColumns actually runs — testKind
// here is Oracle, which uses `:1`-style bind placeholders SQLite can't
// execute, so this documents the same fail-open behavior the other
// dialects' tests cover: a lookup error must not abort the statement, it
// just leaves that column's origin nil.
func TestPrepareJoinOriginsOracle_PKLookupFailsOpen(t *testing.T) {
	db := newTestDB(t)
	_, origins, ok := prepareJoinOriginsOracle(db, store.DBKindOracle, `SELECT u.id, u.name FROM users u JOIN orders o ON u.id = o.user_id`)
	if !ok {
		t.Fatal("prepareJoinOriginsOracle rejected a recognized JOIN shape")
	}
	for i, o := range origins {
		if o != nil {
			t.Errorf("origins[%d] = %v, want nil (PK lookup can't succeed against this test's SQLite backend)", i, o)
		}
	}
}

package query

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/antlr4-go/antlr/v4"
	plsqlparser "github.com/bytebase/plsql-parser"

	"dbtool/server/internal/dbconn"
	"dbtool/server/internal/store"
)

// countingErrorListener counts syntax errors instead of printing them to
// stderr (antlr's default) — prepareJoinOriginsOracle just needs to know
// whether parsing was clean, not the messages.
type countingErrorListener struct {
	antlr.DefaultErrorListener
	errors int
}

func (l *countingErrorListener) SyntaxError(_ antlr.Recognizer, _ interface{}, _, _ int, _ string, _ antlr.RecognitionException) {
	l.errors++
}

// prepareJoinOriginsOracle is the Oracle dialect's ADR 0011 JOIN pass — see
// prepareJoinOriginsMySQL for the shared rationale and prepareJoinOrigins
// for how a kind picks its dialect.
//
// Unlike the other three dialects' parsers, plsql-parser is a generated
// ANTLR grammar with no AST-to-SQL deparser, so the rewrite works
// differently: instead of rebuilding the statement from a modified tree,
// it splices the hidden PK carrier columns into the original text right
// after the last visible column (found via the parsed selected_list's stop
// token position) — the same text-splice idea ADR 0008's regex rewrite
// already relies on, just anchored by a real parse instead of a regex.
func prepareJoinOriginsOracle(db *sql.DB, kind store.DBKind, stmt string) (rewritten string, origins []*ColumnOrigin, ok bool) {
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
	if errs.errors > 0 || tokens.LT(1).GetTokenType() != antlr.TokenEOF {
		// Either a real syntax error, or trailing input Select_statement()
		// didn't consume (e.g. a second statement after `;`) — both mean
		// this isn't safely a single plain SELECT.
		return stmt, nil, false
	}

	onlyStmt := selStmt.Select_only_statement()
	if onlyStmt == nil {
		return stmt, nil, false
	}
	subq := onlyStmt.Subquery()
	if subq == nil || len(subq.AllSubquery_operation_part()) > 0 {
		return stmt, nil, false // UNION/INTERSECT/MINUS — follow-up work (ADR 0011)
	}
	basic := subq.Subquery_basic_elements()
	if basic == nil {
		return stmt, nil, false
	}
	qb := basic.Query_block()
	if qb == nil {
		return stmt, nil, false // parenthesized subquery, not a plain query block
	}
	if qb.DISTINCT() != nil || qb.UNIQUE() != nil || qb.Group_by_clause() != nil {
		// See the MySQL pass: appending a hidden PK carrier column isn't
		// safe when GROUP BY/DISTINCT is in play — it can either error the
		// query out or silently change which rows count as distinct.
		return stmt, nil, false
	}
	fromClause := qb.From_clause()
	selectedList := qb.Selected_list()
	if fromClause == nil || selectedList == nil {
		return stmt, nil, false
	}

	aliasToTable, tablesOK := collectJoinTablesOracle(fromClause.Table_ref_list())
	if !tablesOK || len(aliasToTable) < 2 {
		return stmt, nil, false
	}

	elements := selectedList.AllSelect_list_elements()
	visibleCount := len(elements)
	if selectedList.ASTERISK() != nil || visibleCount == 0 {
		return stmt, nil, false // bare `SELECT *` across a JOIN: not this pass's job
	}
	origins = make([]*ColumnOrigin, visibleCount)

	type hiddenNeed struct {
		alias, table string
		pkCols       []string
	}
	tablePK := map[string][]string{}
	tableNeeded := map[string]bool{}
	var needed []hiddenNeed
	var err error

	for i, elem := range elements {
		if elem.Table_wild() != nil {
			continue // `alias.*`: not a single traceable column
		}
		ge := unwrapGeneralElementOracle(elem.Expression())
		if ge == nil || len(ge.AllGeneral_element_part()) != 2 {
			continue // unqualified or computed column in a JOIN: ambiguous
		}
		parts := ge.AllGeneral_element_part()
		if parts[0].Function_argument() != nil || parts[1].Function_argument() != nil {
			continue // e.g. `pkg.func()`, not a plain column reference
		}
		alias := oracleUnquote(parts[0].Id_expression().GetText())
		table, known := aliasToTable[alias]
		if !known {
			continue
		}

		pk, cached := tablePK[table]
		if !cached {
			pk, err = dbconn.PrimaryKeyColumns(db, kind, table)
			if err != nil {
				pk = nil
			}
			tablePK[table] = pk
		}
		if len(pk) == 0 {
			continue // no PK: can't re-fetch a row for this table (mirrors ADR 0009)
		}

		if !tableNeeded[alias] {
			tableNeeded[alias] = true
			needed = append(needed, hiddenNeed{alias: alias, table: table, pkCols: pk})
		}
		origins[i] = &ColumnOrigin{Table: table, PrimaryKeyColumns: pk}
	}

	if len(needed) == 0 {
		return stmt, origins, true
	}

	var hiddenCols strings.Builder
	hiddenIndex := map[string][]int{}
	nextIndex := visibleCount
	for _, need := range needed {
		idxs := make([]int, len(need.pkCols))
		for j, pkCol := range need.pkCols {
			hiddenName := fmt.Sprintf("__pk_%s_%s", need.alias, pkCol)
			fmt.Fprintf(&hiddenCols, `, "%s"."%s" AS "%s"`, need.alias, pkCol, hiddenName)
			idxs[j] = nextIndex
			nextIndex++
		}
		hiddenIndex[need.alias] = idxs
	}

	for i, elem := range elements {
		if origins[i] == nil {
			continue
		}
		alias := oracleUnquote(unwrapGeneralElementOracle(elem.Expression()).AllGeneral_element_part()[0].Id_expression().GetText())
		origins[i].PrimaryKeyRowIndexes = hiddenIndex[alias]
	}

	// Splice the hidden columns in right after the last visible column's
	// last character. antlr's Go runtime indexes by rune, not byte, so the
	// splice must too (this app is Korean-language and table/column names
	// can be non-ASCII).
	runes := []rune(stmt)
	insertAt := selectedList.GetStop().GetStop() + 1
	rewritten = string(runes[:insertAt]) + hiddenCols.String() + string(runes[insertAt:])
	return rewritten, origins, true
}

// unwrapGeneralElementOracle descends through the expression grammar's
// pass-through layers (each just wraps a single child when no operator is
// actually present) to find whether expr is, in its entirety, nothing more
// than a general_element (a bare `alias.column`-shaped reference) — as
// opposed to something built from one, like `alias.column + 1`.
func unwrapGeneralElementOracle(expr plsqlparser.IExpressionContext) *plsqlparser.General_elementContext {
	var n antlr.Tree = expr
	for {
		if ge, ok := n.(*plsqlparser.General_elementContext); ok {
			return ge
		}
		prc, ok := n.(antlr.ParserRuleContext)
		if !ok || prc.GetChildCount() != 1 {
			return nil
		}
		n = prc.GetChild(0)
	}
}

// oracleUnquote strips the double quotes from a delimited identifier
// (Oracle's `"Mixed Case"` form); a plain identifier passes through
// unchanged.
func oracleUnquote(s string) string {
	return strings.Trim(s, `"`)
}

// collectJoinTablesOracle walks a FROM clause made only of plain table
// references (optionally JOINed), mapping each alias (or bare table name
// when unaliased) to its bare table name. ok is false the moment it finds
// anything this narrow pass doesn't understand — a derived table/subquery,
// most commonly.
func collectJoinTablesOracle(list plsqlparser.ITable_ref_listContext) (map[string]string, bool) {
	if list == nil {
		return nil, false
	}
	result := map[string]string{}
	addAux := func(aux plsqlparser.ITable_ref_auxContext) bool {
		if aux == nil {
			return false
		}
		one, isPlain := aux.Table_ref_aux_internal().(*plsqlparser.Table_ref_aux_internal_oneContext)
		if !isPlain {
			return false // derived table/`ONLY(...)`/JSON table — not this pass's job
		}
		dml := one.Dml_table_expression_clause()
		if dml == nil || dml.Tableview_name() == nil {
			return false
		}
		tv := dml.Tableview_name()
		bare := oracleUnquote(tv.Identifier().GetText())
		if tv.Id_expression() != nil {
			bare = oracleUnquote(tv.Id_expression().GetText()) // schema.table -> table
		}
		alias := bare
		if aux.Table_alias() != nil {
			alias = oracleUnquote(aux.Table_alias().GetText())
		}
		result[alias] = bare
		return true
	}
	for _, ref := range list.AllTable_ref() {
		if !addAux(ref.Table_ref_aux()) {
			return nil, false
		}
		for _, join := range ref.AllJoin_clause() {
			if !addAux(join.Table_ref_aux()) {
				return nil, false
			}
		}
	}
	return result, true
}

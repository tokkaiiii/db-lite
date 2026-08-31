package query

import (
	"database/sql"
	"fmt"
	"sort"
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

// oracleTableRef is what an alias in a FROM clause resolves to: exactly
// one of Table (a real table) or Derived (a subquery in FROM, one level
// deep — see resolveDerivedColumnOracle) is set.
type oracleTableRef struct {
	Table   string
	Derived *plsqlparser.Query_blockContext
}

// parseOracleSelect parses stmt as a single Oracle SELECT and, if it's a
// plain (non-UNION) query block, returns it. ok is false for anything
// else: a syntax error, trailing input after the statement, a top-level
// UNION/INTERSECT/MINUS, or a parenthesized subquery instead of a query
// block.
func parseOracleSelect(stmt string) (qb *plsqlparser.Query_blockContext, ok bool) {
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
		return nil, false
	}
	return asPlainQueryBlock(selStmt.Select_only_statement())
}

// asPlainQueryBlock unwraps a select_only_statement down to its
// query_block, the same descent prepareJoinOriginsOracle and
// resolveDerivedColumnOracle both need (a subquery used as a derived
// table parses through this same rule, just nested one level deeper).
// ok is false for a UNION/INTERSECT/MINUS or a parenthesized subquery
// instead of a plain query block.
func asPlainQueryBlock(onlyStmt plsqlparser.ISelect_only_statementContext) (*plsqlparser.Query_blockContext, bool) {
	if onlyStmt == nil {
		return nil, false
	}
	subq := onlyStmt.Subquery()
	if subq == nil || len(subq.AllSubquery_operation_part()) > 0 {
		return nil, false // UNION/INTERSECT/MINUS — follow-up work (ADR 0011)
	}
	basic := subq.Subquery_basic_elements()
	if basic == nil {
		return nil, false
	}
	qbIface := basic.Query_block()
	if qbIface == nil {
		return nil, false // parenthesized subquery, not a plain query block
	}
	qb, ok := qbIface.(*plsqlparser.Query_blockContext)
	if !ok {
		return nil, false
	}
	return qb, true
}

// prepareJoinOriginsOracle is the Oracle dialect's ADR 0011
// JOIN/derived-table download pass — see prepareJoinOriginsMySQL for the
// shared rationale and design (this mirrors it; only the AST shapes and
// rewrite mechanics differ). CTEs (WITH ...) aren't attempted yet — see
// the wayfinder issue.
//
// Unlike the other three dialects' parsers, plsql-parser is a generated
// ANTLR grammar with no AST-to-SQL deparser, so the rewrite works
// differently: instead of rebuilding the statement from a modified tree,
// it splices the hidden PK carrier columns into the original text right
// after each involved SELECT list's last visible column (found via that
// selected_list's parsed stop-token position) — the same text-splice idea
// ADR 0008's regex rewrite already relies on, just anchored by a real
// parse instead of a regex. A need routed through a derived table needs
// two splice points (the derived SELECT's own list, and the outer one),
// so all of a statement's splice points are collected up front and
// applied in one pass, left to right, to keep their positions valid
// against each other.
func prepareJoinOriginsOracle(db *sql.DB, kind store.DBKind, stmt string) (rewritten string, origins []*ColumnOrigin, ok bool) {
	qb, ok := parseOracleSelect(stmt)
	if !ok {
		return stmt, nil, false
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

	aliasToRef, order, tablesOK := collectJoinTablesOracle(fromClause.Table_ref_list())
	if !tablesOK || len(aliasToRef) < 2 {
		return stmt, nil, false
	}

	if selectedList.ASTERISK() != nil {
		// A bare (unqualified) `SELECT *` across the whole FROM: no
		// rewrite needed at all, since SQL's `*` expansion order is
		// well-defined — see expandWildcardOrigins.
		tables := make([]string, len(order))
		for i, alias := range order {
			ref := aliasToRef[alias]
			if ref.Derived != nil {
				return stmt, nil, false // wildcard expansion through a derived table isn't supported yet
			}
			tables[i] = ref.Table
		}
		wildcardOrigins, wildcardOK := expandWildcardOrigins(db, kind, tables)
		if !wildcardOK {
			return stmt, nil, false
		}
		return stmt, wildcardOrigins, true
	}

	elements := selectedList.AllSelect_list_elements()
	visibleCount := len(elements)
	if visibleCount == 0 {
		return stmt, nil, false
	}
	for _, elem := range elements {
		if elem.Table_wild() != nil {
			// `alias.*` expands to however many real columns that table
			// has, which this pass has no schema access to count — so
			// visibleCount can't be trusted as the number of physical
			// result columns at all if any element is a wildcard.
			// Bailing out entirely (not just leaving this one column's
			// origin nil) is required — see the MySQL pass's version of
			// this same guard.
			return stmt, nil, false
		}
	}
	origins = make([]*ColumnOrigin, visibleCount)
	hiddenKeys := make([]string, visibleCount)

	type hiddenNeed struct {
		outerAlias string
		table      string
		pkCols     []string
		derived    *plsqlparser.Query_blockContext // non-nil: also inject into this inner SELECT
		innerAlias string
	}
	tablePK := map[string][]string{}
	tableNeeded := map[string]bool{}
	var needed []hiddenNeed
	var err error

	lookupPK := func(table string) []string {
		pk, cached := tablePK[table]
		if !cached {
			pk, err = dbconn.PrimaryKeyColumns(db, kind, table)
			if err != nil {
				pk = nil
			}
			tablePK[table] = pk
		}
		return pk
	}

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
		outerAlias := oracleUnquote(parts[0].Id_expression().GetText())
		ref, known := aliasToRef[outerAlias]
		if !known {
			continue
		}

		if ref.Derived == nil {
			pk := lookupPK(ref.Table)
			if len(pk) == 0 {
				continue // no PK: can't re-fetch a row for this table (mirrors ADR 0009)
			}
			key := outerAlias
			if !tableNeeded[key] {
				tableNeeded[key] = true
				needed = append(needed, hiddenNeed{outerAlias: outerAlias, table: ref.Table, pkCols: pk})
			}
			origins[i] = &ColumnOrigin{Table: ref.Table, PrimaryKeyColumns: pk}
			hiddenKeys[i] = key
			continue
		}

		colName := oracleUnquote(parts[1].Id_expression().GetText())
		table, innerAlias, ok := resolveDerivedColumnOracle(ref.Derived, colName)
		if !ok {
			continue
		}
		pk := lookupPK(table)
		if len(pk) == 0 {
			continue
		}
		key := outerAlias + "." + innerAlias
		if !tableNeeded[key] {
			tableNeeded[key] = true
			needed = append(needed, hiddenNeed{outerAlias: outerAlias, table: table, pkCols: pk, derived: ref.Derived, innerAlias: innerAlias})
		}
		origins[i] = &ColumnOrigin{Table: table, PrimaryKeyColumns: pk}
		hiddenKeys[i] = key
	}

	if len(needed) == 0 {
		return stmt, origins, true
	}

	// Build one splice chunk per involved SELECT list: the outer one
	// always gets one (a pass-through per need), and each distinct
	// derived query block used gets its own (the real PK column,
	// exposed under the same hidden name the outer pass-through expects).
	type chunk struct {
		pos  int
		text *strings.Builder
	}
	chunks := map[*plsqlparser.Query_blockContext]*chunk{}
	chunkFor := func(owner *plsqlparser.Query_blockContext, list plsqlparser.ISelected_listContext) *chunk {
		c, ok := chunks[owner]
		if !ok {
			c = &chunk{pos: list.GetStop().GetStop() + 1, text: &strings.Builder{}}
			chunks[owner] = c
		}
		return c
	}
	outerChunk := chunkFor(qb, selectedList)

	hiddenIndex := map[string][]int{}
	nextIndex := visibleCount
	for _, need := range needed {
		key := need.outerAlias
		if need.derived != nil {
			key += "." + need.innerAlias
		}
		idxs := make([]int, len(need.pkCols))
		for j, pkCol := range need.pkCols {
			hiddenName := fmt.Sprintf("__pk_%s_%s", sanitizeIdentPart(key), pkCol)
			outerColRef := pkCol
			if need.derived != nil {
				innerChunk := chunkFor(need.derived, need.derived.Selected_list())
				fmt.Fprintf(innerChunk.text, `, "%s"."%s" AS "%s"`, need.innerAlias, pkCol, hiddenName)
				outerColRef = hiddenName
			}
			fmt.Fprintf(outerChunk.text, `, "%s"."%s" AS "%s"`, need.outerAlias, outerColRef, hiddenName)
			idxs[j] = nextIndex
			nextIndex++
		}
		hiddenIndex[key] = idxs
	}

	for i := range origins[:visibleCount] {
		if origins[i] == nil {
			continue
		}
		origins[i].PrimaryKeyRowIndexes = hiddenIndex[hiddenKeys[i]]
	}

	// Apply every chunk's splice in one left-to-right pass over the
	// original text so each chunk's position (computed against the
	// unmodified text) stays valid regardless of how many others sit
	// before or after it. antlr's Go runtime indexes by rune, not byte,
	// so the splice must too (this app is Korean-language and
	// table/column names can be non-ASCII).
	positions := make([]*chunk, 0, len(chunks))
	for _, c := range chunks {
		positions = append(positions, c)
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i].pos < positions[j].pos })

	runes := []rune(stmt)
	var out strings.Builder
	prev := 0
	for _, c := range positions {
		out.WriteString(string(runes[prev:c.pos]))
		out.WriteString(c.text.String())
		prev = c.pos
	}
	out.WriteString(string(runes[prev:]))
	return out.String(), origins, true
}

// resolveDerivedColumnOracle looks up outerColName in derived's own
// selected_list (by its column_alias, or its bare column name when
// unaliased — Oracle's own naming rule for a plain column reference) and,
// if that item is itself a plain `alias.column` (or, when derived's FROM
// has exactly one table, a bare unqualified column) reference into a real
// table (or JOIN of real tables) in derived's own FROM, returns that
// table and the alias it's known by inside derived. ok is false for
// anything this narrow one-level pass can't trace — see
// resolveDerivedColumnMySQL, the same rules apply.
func resolveDerivedColumnOracle(derived *plsqlparser.Query_blockContext, outerColName string) (table, innerAlias string, ok bool) {
	if derived.DISTINCT() != nil || derived.UNIQUE() != nil || derived.Group_by_clause() != nil {
		return "", "", false
	}
	fromClause := derived.From_clause()
	selectedList := derived.Selected_list()
	if fromClause == nil || selectedList == nil || selectedList.ASTERISK() != nil {
		return "", "", false
	}
	innerTables, _, tablesOK := collectJoinTablesOracle(fromClause.Table_ref_list())
	if !tablesOK {
		return "", "", false
	}

	for _, elem := range selectedList.AllSelect_list_elements() {
		if elem.Table_wild() != nil {
			continue
		}
		var exposedName string
		if alias := elem.Column_alias(); alias != nil {
			switch {
			case alias.Identifier() != nil:
				exposedName = oracleUnquote(alias.Identifier().GetText())
			case alias.Quoted_string() != nil:
				exposedName = oracleUnquote(alias.Quoted_string().GetText())
			}
		}
		ge := unwrapGeneralElementOracle(elem.Expression())
		if exposedName == "" {
			if ge == nil {
				continue // a computed expression with no alias: not nameable
			}
			parts := ge.AllGeneral_element_part()
			exposedName = oracleUnquote(parts[len(parts)-1].Id_expression().GetText())
		}
		if exposedName != outerColName {
			continue
		}
		if ge == nil {
			return "", "", false // a computed expression: not traceable
		}

		var alias string
		switch parts := ge.AllGeneral_element_part(); len(parts) {
		case 1:
			if parts[0].Function_argument() != nil || len(innerTables) != 1 {
				return "", "", false // a function call, or ambiguous without a schema to check against
			}
			for a := range innerTables {
				alias = a
			}
		case 2:
			if parts[0].Function_argument() != nil || parts[1].Function_argument() != nil {
				return "", "", false
			}
			alias = oracleUnquote(parts[0].Id_expression().GetText())
		default:
			return "", "", false
		}
		ref, known := innerTables[alias]
		if !known || ref.Derived != nil {
			return "", "", false // one level only — no further nesting
		}
		return ref.Table, alias, true
	}
	return "", "", false
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

// collectJoinTablesOracle walks a FROM clause made of table references
// (optionally JOINed), mapping each alias (or bare table name when
// unaliased) to what it refers to — a real table, or (one level deep) a
// derived table. ok is false the moment it finds anything this narrow
// pass doesn't understand, so the caller can bail out rather than guess.
func collectJoinTablesOracle(list plsqlparser.ITable_ref_listContext) (refs map[string]oracleTableRef, order []string, ok bool) {
	if list == nil {
		return nil, nil, false
	}
	result := map[string]oracleTableRef{}
	addAux := func(aux plsqlparser.ITable_ref_auxContext) bool {
		if aux == nil {
			return false
		}
		one, isPlain := aux.Table_ref_aux_internal().(*plsqlparser.Table_ref_aux_internal_oneContext)
		if !isPlain {
			return false // `ONLY(...)`/JSON table — not this pass's job
		}
		dml := one.Dml_table_expression_clause()
		if dml == nil {
			return false
		}

		if dml.Select_statement() != nil {
			derivedQB, ok := asPlainQueryBlock(dml.Select_statement().Select_only_statement())
			if !ok || aux.Table_alias() == nil {
				return false // a UNION in the subquery, or no alias — can't reference it
			}
			alias := oracleUnquote(aux.Table_alias().GetText())
			result[alias] = oracleTableRef{Derived: derivedQB}
			order = append(order, alias)
			return true
		}

		if dml.Tableview_name() == nil {
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
		result[alias] = oracleTableRef{Table: bare}
		order = append(order, alias)
		return true
	}
	for _, ref := range list.AllTable_ref() {
		if !addAux(ref.Table_ref_aux()) {
			return nil, nil, false
		}
		for _, join := range ref.AllJoin_clause() {
			if !addAux(join.Table_ref_aux()) {
				return nil, nil, false
			}
		}
	}
	return result, order, true
}

package query

import (
	"database/sql"
	"fmt"

	"github.com/ha1tch/tsqlparser"
	"github.com/ha1tch/tsqlparser/ast"

	"dbtool/server/internal/dbconn"
	"dbtool/server/internal/store"
)

// prepareJoinOriginsMSSQL is the MSSQL dialect's ADR 0011 JOIN pass — see
// prepareJoinOriginsMySQL for the shared rationale and prepareJoinOrigins
// for how a kind picks its dialect. Structurally identical to the other
// dialects' passes; only the AST shapes differ (ha1tch/tsqlparser's
// hand-written recursive-descent AST here).
func prepareJoinOriginsMSSQL(db *sql.DB, kind store.DBKind, stmt string) (rewritten string, origins []*ColumnOrigin, ok bool) {
	program, errs := tsqlparser.Parse(stmt)
	if len(errs) > 0 || len(program.Statements) != 1 {
		return stmt, nil, false
	}
	sel, isSelect := program.Statements[0].(*ast.SelectStatement)
	if !isSelect {
		return stmt, nil, false
	}
	if sel.Distinct || len(sel.GroupBy) > 0 || sel.Having != nil {
		// See the MySQL pass: appending a hidden PK carrier column isn't
		// safe when GROUP BY/DISTINCT is in play — it can either error the
		// query out or silently change which rows count as distinct.
		return stmt, nil, false
	}
	if sel.From == nil {
		return stmt, nil, false
	}

	aliasToTable, tablesOK := collectJoinTablesMSSQL(sel.From.Tables)
	if !tablesOK || len(aliasToTable) < 2 {
		return stmt, nil, false
	}

	visibleCount := len(sel.Columns)
	origins = make([]*ColumnOrigin, visibleCount)

	type hiddenNeed struct {
		alias, table string
		pkCols       []string
	}
	tablePK := map[string][]string{}
	tableNeeded := map[string]bool{}
	var needed []hiddenNeed
	var err error

	for i, col := range sel.Columns {
		if col.AllColumns || col.Variable != nil {
			continue // `*`/`alias.*` or `@var = expr`: not a single traceable column
		}
		qi, isQualified := col.Expression.(*ast.QualifiedIdentifier)
		if !isQualified || len(qi.Parts) != 2 {
			continue // unqualified or computed column in a JOIN: ambiguous
		}
		alias := qi.Parts[0].Value
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

	hiddenIndex := map[string][]int{}
	nextIndex := visibleCount
	for _, need := range needed {
		idxs := make([]int, len(need.pkCols))
		for j, pkCol := range need.pkCols {
			hiddenName := fmt.Sprintf("__pk_%s_%s", need.alias, pkCol)
			sel.Columns = append(sel.Columns, ast.SelectColumn{
				Expression: &ast.QualifiedIdentifier{Parts: []*ast.Identifier{
					{Value: need.alias},
					{Value: pkCol},
				}},
				Alias: &ast.Identifier{Value: hiddenName},
			})
			idxs[j] = nextIndex
			nextIndex++
		}
		hiddenIndex[need.alias] = idxs
	}

	for i, col := range sel.Columns[:visibleCount] {
		if origins[i] == nil {
			continue
		}
		alias := col.Expression.(*ast.QualifiedIdentifier).Parts[0].Value
		origins[i].PrimaryKeyRowIndexes = hiddenIndex[alias]
	}

	return sel.String(), origins, true
}

// collectJoinTablesMSSQL walks a FROM clause made only of plain table
// references (optionally JOINed), mapping each alias (or bare table name
// when unaliased) to its bare table name. ok is false the moment it finds
// anything this narrow pass doesn't understand — a derived table/subquery,
// most commonly.
func collectJoinTablesMSSQL(tables []ast.TableReference) (map[string]string, bool) {
	result := map[string]string{}
	var walk func(ast.TableReference) bool
	walk = func(ref ast.TableReference) bool {
		switch t := ref.(type) {
		case *ast.JoinClause:
			return walk(t.Left) && walk(t.Right)
		case *ast.TableName:
			if t.Name == nil || len(t.Name.Parts) == 0 {
				return false
			}
			bare := t.Name.Parts[len(t.Name.Parts)-1].Value
			alias := bare
			if t.Alias != nil {
				alias = t.Alias.Value
			}
			result[alias] = bare
			return true
		default:
			return false // derived table/subquery/table-valued function — not this pass's job
		}
	}
	for _, t := range tables {
		if !walk(t) {
			return nil, false
		}
	}
	return result, true
}

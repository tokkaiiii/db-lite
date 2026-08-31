package query

import (
	"database/sql"
	"fmt"

	"github.com/ha1tch/tsqlparser"
	"github.com/ha1tch/tsqlparser/ast"

	"dbtool/server/internal/dbconn"
	"dbtool/server/internal/store"
)

// mssqlTableRef is what an alias in a FROM clause resolves to: exactly one
// of Table (a real table) or Derived (a subquery in FROM, one level deep —
// see resolveDerivedColumnMSSQL) is set.
type mssqlTableRef struct {
	Table   string
	Derived *ast.SelectStatement
}

// prepareJoinOriginsMSSQL is the MSSQL dialect's ADR 0011
// JOIN/derived-table download pass — see prepareJoinOriginsMySQL for the
// shared rationale and design (this mirrors it; only the AST shapes
// differ — ha1tch/tsqlparser's hand-written recursive-descent tree here).
// CTEs (WITH ...) aren't attempted yet — see the wayfinder issue.
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

	aliasToRef, order, tablesOK := collectJoinTablesMSSQL(sel.From.Tables)
	if !tablesOK || len(aliasToRef) < 2 {
		return stmt, nil, false
	}

	if len(sel.Columns) == 1 && sel.Columns[0].AllColumns {
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
	for _, col := range sel.Columns {
		if mssqlColumnIsWildcard(col) {
			// `alias.*`, or `*` mixed with other columns, expands to
			// however many real columns that table has, which this pass
			// has no schema access to count in that shape — so
			// len(sel.Columns) can't be trusted as the number of physical
			// result columns at all. Bailing out entirely (not just
			// leaving this one column's origin nil) is required — see
			// the MySQL pass's version of this same guard.
			return stmt, nil, false
		}
	}

	visibleCount := len(sel.Columns)
	origins = make([]*ColumnOrigin, visibleCount)
	hiddenKeys := make([]string, visibleCount)

	type hiddenNeed struct {
		outerAlias string
		table      string
		pkCols     []string
		derived    *ast.SelectStatement // non-nil: also inject into this inner SELECT
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

	for i, col := range sel.Columns {
		if col.AllColumns || col.Variable != nil {
			continue // `*`/`alias.*` or `@var = expr`: not a single traceable column
		}
		qi, isQualified := col.Expression.(*ast.QualifiedIdentifier)
		if !isQualified || len(qi.Parts) != 2 {
			continue // unqualified or computed column in a JOIN: ambiguous
		}
		outerAlias := qi.Parts[0].Value
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

		table, innerAlias, ok := resolveDerivedColumnMSSQL(ref.Derived, qi.Parts[1].Value)
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

	// Append one hidden `__pk_<...>_<col>` carrier per PK column per table
	// that contributed a traceable column. A need routed through a derived
	// table gets injected twice: once inside the derived SELECT (to expose
	// the value at all) and once in the outer SELECT (a plain pass-through
	// referencing what the derived table now exposes).
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
				need.derived.Columns = append(need.derived.Columns, ast.SelectColumn{
					Expression: &ast.QualifiedIdentifier{Parts: []*ast.Identifier{
						{Value: need.innerAlias},
						{Value: pkCol},
					}},
					Alias: &ast.Identifier{Value: hiddenName},
				})
				outerColRef = hiddenName
			}
			sel.Columns = append(sel.Columns, ast.SelectColumn{
				Expression: &ast.QualifiedIdentifier{Parts: []*ast.Identifier{
					{Value: need.outerAlias},
					{Value: outerColRef},
				}},
				Alias: &ast.Identifier{Value: hiddenName},
			})
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

	return sel.String(), origins, true
}

// mssqlColumnIsWildcard reports whether col is a `*` or `alias.*` select
// item. tsqlparser only sets SelectColumn.AllColumns for a bare top-level
// `*`; `alias.*` parses as a qualified reference whose last part is
// literally the identifier "*", so both shapes need checking.
func mssqlColumnIsWildcard(col ast.SelectColumn) bool {
	if col.AllColumns {
		return true
	}
	switch e := col.Expression.(type) {
	case *ast.Identifier:
		return e.Value == "*"
	case *ast.QualifiedIdentifier:
		return len(e.Parts) > 0 && e.Parts[len(e.Parts)-1].Value == "*"
	default:
		return false
	}
}

// resolveDerivedColumnMSSQL looks up outerColName in derived's own SELECT
// list (by its alias, or its bare column name when unaliased) and, if
// that item is itself a plain `alias.column` (or, when derived's FROM has
// exactly one table, a bare unqualified column) reference into a real
// table (or JOIN of real tables) in derived's own FROM, returns that table
// and the alias it's known by inside derived. ok is false for anything
// this narrow one-level pass can't trace — see resolveDerivedColumnMySQL,
// the same rules apply.
func resolveDerivedColumnMSSQL(derived *ast.SelectStatement, outerColName string) (table, innerAlias string, ok bool) {
	if derived.Union != nil || derived.Distinct || len(derived.GroupBy) > 0 || derived.Having != nil {
		return "", "", false
	}
	if derived.From == nil {
		return "", "", false
	}
	innerTables, _, tablesOK := collectJoinTablesMSSQL(derived.From.Tables)
	if !tablesOK {
		return "", "", false
	}

	for _, col := range derived.Columns {
		if col.AllColumns || col.Variable != nil {
			continue
		}
		var exposedName string
		switch {
		case col.Alias != nil:
			exposedName = col.Alias.Value
		case col.Expression != nil:
			switch e := col.Expression.(type) {
			case *ast.QualifiedIdentifier:
				exposedName = e.Parts[len(e.Parts)-1].Value
			case *ast.Identifier:
				exposedName = e.Value
			}
		}
		if exposedName != outerColName {
			continue
		}

		var alias string
		switch e := col.Expression.(type) {
		case *ast.Identifier:
			if len(innerTables) != 1 {
				return "", "", false // ambiguous without a schema to check against
			}
			for a := range innerTables {
				alias = a
			}
		case *ast.QualifiedIdentifier:
			if len(e.Parts) != 2 {
				return "", "", false
			}
			alias = e.Parts[0].Value
		default:
			return "", "", false // a computed expression: not traceable
		}
		ref, known := innerTables[alias]
		if !known || ref.Derived != nil {
			return "", "", false // one level only — no further nesting
		}
		return ref.Table, alias, true
	}
	return "", "", false
}

// collectJoinTablesMSSQL walks a FROM clause made of table references
// (optionally JOINed), mapping each alias (or bare table name when
// unaliased) to what it refers to — a real table, or (one level deep) a
// derived table — and also returning those aliases in FROM order (needed
// to expand a bare `SELECT *`; see expandWildcardOrigins). ok is false the
// moment it finds anything this narrow pass doesn't understand, so the
// caller can bail out rather than guess.
func collectJoinTablesMSSQL(tables []ast.TableReference) (refs map[string]mssqlTableRef, order []string, ok bool) {
	result := map[string]mssqlTableRef{}
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
			result[alias] = mssqlTableRef{Table: bare}
			order = append(order, alias)
			return true
		case *ast.DerivedTable:
			if t.Subquery == nil || t.Subquery.Union != nil || t.Alias == nil {
				return false // a UNION in the subquery, or no alias — can't reference it
			}
			result[t.Alias.Value] = mssqlTableRef{Derived: t.Subquery}
			order = append(order, t.Alias.Value)
			return true
		default:
			return false // table-valued function or similar — not this pass's job
		}
	}
	for _, t := range tables {
		if !walk(t) {
			return nil, nil, false
		}
	}
	return result, order, true
}

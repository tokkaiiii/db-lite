package query

import (
	"database/sql"
	"fmt"

	"github.com/xwb1989/sqlparser"

	"dbtool/server/internal/dbconn"
	"dbtool/server/internal/store"
)

// mysqlTableRef is what an alias in a FROM clause resolves to: exactly one
// of Table (a real table) or Derived (a subquery in FROM, one level deep —
// see resolveDerivedColumnMySQL) is set.
type mysqlTableRef struct {
	Table   string
	Derived *sqlparser.Select
}

// prepareJoinOriginsMySQL is the ADR 0011 JOIN/derived-table download path.
// ADR 0009 already covers PK-having single-table `SELECT *` via a narrow
// regex (rewrite.go); this handles the next narrowest shapes a real parser
// can analyze safely: a SELECT over one or more real tables (optionally
// JOINed), optionally with one of the "tables" actually being a derived
// table (a subquery in FROM) that itself is just a plain table or JOIN —
// not recursively arbitrary depth. This also covers a single real table
// with an explicit column list (`SELECT id, name FROM users`), the gap
// ADR 0009's `SELECT *`-only shape and this pass's original JOIN-only gate
// both missed — see ADR 0011's "단일 테이블도 지원" section. Each output
// column must be a simple `column` or `alias.column` reference (the former
// only unambiguous when FROM has exactly one table).
//
// CTEs (WITH ...) aren't attempted: this MySQL parser (a 2018-era fork
// predating MySQL 8.0's WITH support) can't parse them at all, so a
// statement using one simply fails to parse and falls through to ok=false
// — same as any other unsupported shape.
func prepareJoinOriginsMySQL(db *sql.DB, kind store.DBKind, stmt string) (rewritten string, origins []*ColumnOrigin, ok bool) {
	if kind != store.DBKindMySQL {
		return stmt, nil, false
	}

	parsed, err := sqlparser.Parse(stmt)
	if err != nil {
		return stmt, nil, false
	}
	sel, isSelect := parsed.(*sqlparser.Select)
	if !isSelect {
		return stmt, nil, false
	}
	if len(sel.GroupBy) > 0 || sel.Having != nil || sel.Distinct != "" {
		// Appending hidden PK carrier columns isn't safe here: for
		// GROUP BY it can turn a query MySQL would otherwise run into a
		// only_full_group_by error, and for DISTINCT it would silently
		// change which rows count as distinct — a correctness bug, not
		// just a broken download. Bail out entirely rather than touch
		// the query at all.
		return stmt, nil, false
	}

	aliasToRef, order, tablesOK := collectJoinTablesMySQL(sel.From)
	if !tablesOK || len(aliasToRef) == 0 {
		return stmt, nil, false
	}

	// Phase 1: figure out each select-list item's physical width (1 for a
	// plain column or anything else; the referenced table's real column
	// count for a wildcard) and, for wildcard items, their fully-resolved
	// origin block up front — see expandWildcardOrigins. A wildcard on a
	// derived table, or one whose schema can't be looked up, bails the
	// whole statement: after that point nothing later in the list can be
	// positioned correctly either.
	starts := make([]int, len(sel.SelectExprs))
	blocks := make([][]*ColumnOrigin, len(sel.SelectExprs))
	pos := 0
	for i, se := range sel.SelectExprs {
		starts[i] = pos
		star, isStar := se.(*sqlparser.StarExpr)
		if !isStar {
			pos++
			continue
		}
		var tables []string
		if !star.TableName.IsEmpty() {
			ref, known := aliasToRef[star.TableName.Name.String()]
			if !known || ref.Derived != nil {
				return stmt, nil, false
			}
			tables = []string{ref.Table}
		} else {
			tables = make([]string, len(order))
			for j, alias := range order {
				ref := aliasToRef[alias]
				if ref.Derived != nil {
					return stmt, nil, false
				}
				tables[j] = ref.Table
			}
		}
		block, ok := expandWildcardOrigins(db, kind, tables)
		if !ok {
			return stmt, nil, false
		}
		blocks[i] = block
		pos += len(block)
	}
	visibleCount := pos
	origins = make([]*ColumnOrigin, visibleCount)
	hiddenKeys := make([]string, visibleCount) // parallel to origins; hiddenIndex lookup key once known

	// hiddenNeed describes one PK column of one real table that some
	// output column traced back to. innerAlias/innerPKExpr are only set
	// when the real table sits one level down inside a derived table —
	// see resolveDerivedColumnMySQL.
	type hiddenNeed struct {
		outerAlias string
		table      string
		pkCols     []string
		derived    *sqlparser.Select // non-nil: also inject into this inner SELECT
		innerAlias string            // alias of `table` inside `derived`'s own FROM
	}
	tablePK := map[string][]string{}
	tableNeeded := map[string]bool{}
	var needed []hiddenNeed
	var err2 error

	lookupPK := func(table string) []string {
		pk, cached := tablePK[table]
		if !cached {
			pk, err2 = dbconn.PrimaryKeyColumns(db, kind, table)
			if err2 != nil {
				pk = nil
			}
			tablePK[table] = pk
		}
		return pk
	}

	// Phase 2: fill in origins — wildcard items from their precomputed
	// block, everything else via the existing per-column resolution.
	for i, se := range sel.SelectExprs {
		if blocks[i] != nil {
			copy(origins[starts[i]:], blocks[i])
			continue
		}
		aliasedExpr, isAliased := se.(*sqlparser.AliasedExpr)
		if !isAliased {
			continue // anything else: origin stays unknown (nil)
		}
		colName, isCol := aliasedExpr.Expr.(*sqlparser.ColName)
		if !isCol {
			continue // a computed expression: not traceable
		}
		outerAlias := colName.Qualifier.Name.String()
		if outerAlias == "" {
			// Unqualified is only unambiguous when FROM has exactly one
			// table — same rule the derived-table pass already relies on
			// (resolveDerivedColumnMySQL).
			if len(order) != 1 {
				continue
			}
			outerAlias = order[0]
		}
		ref, known := aliasToRef[outerAlias]
		if !known {
			continue
		}
		colPos := starts[i]

		if ref.Derived == nil {
			pk := lookupPK(ref.Table)
			if len(pk) == 0 {
				continue // no PK: can't re-fetch a row for this table (mirrors ADR 0009)
			}
			key := outerAlias // direct table refs are keyed by their own alias
			if !tableNeeded[key] {
				tableNeeded[key] = true
				needed = append(needed, hiddenNeed{outerAlias: outerAlias, table: ref.Table, pkCols: pk})
			}
			origins[colPos] = &ColumnOrigin{Table: ref.Table, PrimaryKeyColumns: pk}
			hiddenKeys[colPos] = key
			continue
		}

		table, innerAlias, ok := resolveDerivedColumnMySQL(ref.Derived, colName.Name.String())
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
		origins[colPos] = &ColumnOrigin{Table: table, PrimaryKeyColumns: pk}
		hiddenKeys[colPos] = key
	}

	if len(needed) == 0 {
		// Parsed fine, but no column's origin could be pinned down (or none
		// of their tables have a PK) — still "ok" so the caller uses these
		// all-nil origins rather than retrying (there's nothing more to try).
		return stmt, origins, true
	}

	// Append one hidden `__pk_<...>_<col>` carrier per PK column per table
	// that contributed a traceable column, so a later cell download can
	// re-fetch that row. These sit past visibleCount and are never shown
	// to the user (see Result.ColumnOrigins). A need routed through a
	// derived table gets injected twice: once inside the derived SELECT
	// (to expose the value at all) and once in the outer SELECT (a plain
	// pass-through referencing what the derived table now exposes).
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
			// outerColRef is what the OUTER select list reaches through
			// need.outerAlias to get this PK value: the real column name
			// directly, for a plain table — or, for a derived table, the
			// hidden name just injected into its own select list (a plain
			// table can't be "reached into" for an unselected column, but
			// this project controls both ends of the derived case, so it
			// injects the same name at both).
			outerColRef := pkCol
			if need.derived != nil {
				need.derived.SelectExprs = append(need.derived.SelectExprs, &sqlparser.AliasedExpr{
					Expr: &sqlparser.ColName{
						Name:      sqlparser.NewColIdent(pkCol),
						Qualifier: sqlparser.TableName{Name: sqlparser.NewTableIdent(need.innerAlias)},
					},
					As: sqlparser.NewColIdent(hiddenName),
				})
				outerColRef = hiddenName
			}
			sel.SelectExprs = append(sel.SelectExprs, &sqlparser.AliasedExpr{
				Expr: &sqlparser.ColName{
					Name:      sqlparser.NewColIdent(outerColRef),
					Qualifier: sqlparser.TableName{Name: sqlparser.NewTableIdent(need.outerAlias)},
				},
				As: sqlparser.NewColIdent(hiddenName),
			})
			idxs[j] = nextIndex
			nextIndex++
		}
		hiddenIndex[key] = idxs
	}

	for i := range origins[:visibleCount] {
		if origins[i] == nil || hiddenKeys[i] == "" {
			// A wildcard-block position (blocks[i] != nil in phase 2)
			// never sets hiddenKeys — its PrimaryKeyRowIndexes was already
			// filled in correctly by expandWildcardOrigins, pointing at
			// the PK's own position inside that same block. Only
			// positions phase 2 resolved via a real hiddenNeed (always a
			// non-empty key) get their indexes from here.
			continue
		}
		origins[i].PrimaryKeyRowIndexes = hiddenIndex[hiddenKeys[i]]
	}

	return sqlparser.String(sel), origins, true
}

// sanitizeIdentPart makes s safe to splice into an unquoted MySQL
// identifier fragment (hidden column names are built from alias names the
// query itself already used, so this just swaps the one separator
// character — '.' — that can't appear in a bare identifier).
func sanitizeIdentPart(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			out[i] = '_'
		} else {
			out[i] = s[i]
		}
	}
	return string(out)
}

// resolveDerivedColumnMySQL looks up outerColName in derived's own SELECT
// list (by its `AS` alias, or its bare column name when unaliased) and, if
// that item is itself a plain `alias.column` reference into a real table
// (or JOIN of real tables) in derived's own FROM, returns that table and
// the alias it's known by inside derived. ok is false for anything this
// narrow one-level pass can't trace: a `*`, a computed expression, GROUP
// BY/DISTINCT/HAVING on the derived query (same reasoning as the outer
// query — see prepareJoinOriginsMySQL), or the column coming from yet
// another nested derived table (recursion stops at one level).
func resolveDerivedColumnMySQL(derived *sqlparser.Select, outerColName string) (table, innerAlias string, ok bool) {
	if len(derived.GroupBy) > 0 || derived.Having != nil || derived.Distinct != "" {
		return "", "", false
	}
	innerTables, _, tablesOK := collectJoinTablesMySQL(derived.From)
	if !tablesOK {
		return "", "", false
	}

	for _, se := range derived.SelectExprs {
		aliasedExpr, isAliased := se.(*sqlparser.AliasedExpr)
		if !isAliased {
			continue // `*`
		}
		var exposedName string
		if !aliasedExpr.As.IsEmpty() {
			exposedName = aliasedExpr.As.String()
		} else if colName, isCol := aliasedExpr.Expr.(*sqlparser.ColName); isCol {
			exposedName = colName.Name.String()
		}
		if exposedName != outerColName {
			continue
		}
		colName, isCol := aliasedExpr.Expr.(*sqlparser.ColName)
		if !isCol {
			return "", "", false // a computed expression: not traceable
		}
		alias := colName.Qualifier.Name.String()
		if colName.Qualifier.IsEmpty() {
			// Unqualified is only unambiguous when derived's own FROM has
			// exactly one table — same rule the single-table ADR 0008/0009
			// path already relies on.
			if len(innerTables) != 1 {
				return "", "", false
			}
			for a := range innerTables {
				alias = a
			}
		}
		ref, known := innerTables[alias]
		if !known || ref.Derived != nil {
			return "", "", false // one level only — no further nesting
		}
		return ref.Table, alias, true
	}
	return "", "", false
}

// collectJoinTablesMySQL walks a FROM clause made of table references
// (optionally JOINed), mapping each alias (or bare table name when
// unaliased) to what it refers to — a real table, or (one level deep) a
// derived table — and also returning those aliases in FROM order (needed
// to expand a bare `SELECT *`; see expandWildcardOrigins). ok is false the
// moment it finds anything this narrow pass doesn't understand, so the
// caller can bail out rather than guess.
func collectJoinTablesMySQL(from sqlparser.TableExprs) (refs map[string]mysqlTableRef, order []string, ok bool) {
	result := map[string]mysqlTableRef{}
	var walk func(sqlparser.TableExpr) bool
	walk = func(te sqlparser.TableExpr) bool {
		switch t := te.(type) {
		case *sqlparser.JoinTableExpr:
			return walk(t.LeftExpr) && walk(t.RightExpr)
		case *sqlparser.AliasedTableExpr:
			switch inner := t.Expr.(type) {
			case sqlparser.TableName:
				alias := t.As.String()
				if alias == "" {
					alias = inner.Name.String()
				}
				result[alias] = mysqlTableRef{Table: bareTableName(inner.Name.String())}
				order = append(order, alias)
				return true
			case *sqlparser.Subquery:
				derivedSel, isSelect := inner.Select.(*sqlparser.Select)
				if !isSelect || t.As.IsEmpty() {
					return false // a UNION in the subquery, or no alias — can't reference it
				}
				result[t.As.String()] = mysqlTableRef{Derived: derivedSel}
				order = append(order, t.As.String())
				return true
			default:
				return false
			}
		default:
			return false
		}
	}
	for _, te := range from {
		if !walk(te) {
			return nil, nil, false
		}
	}
	return result, order, true
}

package query

import (
	"database/sql"
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	pgquery "github.com/wasilibs/go-pgquery"

	"dbtool/server/internal/dbconn"
	"dbtool/server/internal/store"
)

// postgresTableRef is what an alias in a FROM clause resolves to: exactly
// one of Table (a real table) or Derived (a subquery in FROM, one level
// deep — see resolveDerivedColumnPostgres) is set.
type postgresTableRef struct {
	Table   string
	Derived *pg_query.SelectStmt
}

// prepareJoinOriginsPostgres is the Postgres dialect's ADR 0011
// JOIN/derived-table download pass — see prepareJoinOriginsMySQL for the
// shared rationale and design (this mirrors it; only the AST shapes
// differ). CTEs (WITH ...) aren't attempted yet — see the wayfinder issue.
//
// Parsing/deparsing goes through wasilibs/go-pgquery — a drop-in for
// pganalyze/pg_query_go that runs the real Postgres parser compiled to
// WASM instead of via cgo, so this stays cross-compilable without a C
// toolchain. Its ParseResult is the same protobuf type pg_query_go
// exports, so node construction still uses pg_query_go's Make* helpers.
func prepareJoinOriginsPostgres(db *sql.DB, kind store.DBKind, stmt string) (rewritten string, origins []*ColumnOrigin, ok bool) {
	tree, err := pgquery.Parse(stmt)
	if err != nil || len(tree.Stmts) != 1 {
		return stmt, nil, false
	}
	sel := tree.Stmts[0].Stmt.GetSelectStmt()
	if sel == nil || sel.Op != pg_query.SetOperation_SETOP_NONE {
		return stmt, nil, false // nil, or a UNION/INTERSECT/EXCEPT — follow-up work (ADR 0011)
	}
	if len(sel.DistinctClause) > 0 || len(sel.GroupClause) > 0 || sel.HavingClause != nil {
		// See the MySQL pass: appending a hidden PK carrier column isn't
		// safe when GROUP BY/DISTINCT is in play — it can either error the
		// query out or silently change which rows count as distinct.
		return stmt, nil, false
	}

	aliasToRef, tablesOK := collectJoinTablesPostgres(sel.FromClause)
	if !tablesOK || len(aliasToRef) < 2 {
		return stmt, nil, false
	}
	for _, item := range sel.TargetList {
		if colRef := item.GetResTarget().GetVal().GetColumnRef(); colRef != nil {
			for _, f := range colRef.Fields {
				if f.GetAStar() != nil {
					// `*` or `alias.*` expands to however many real
					// columns that table has, which this pass has no
					// schema access to count — so len(sel.TargetList)
					// can't be trusted as the number of physical result
					// columns at all. Bailing out entirely (not just
					// leaving this one column's origin nil) is required
					// — see the MySQL pass's version of this same guard.
					return stmt, nil, false
				}
			}
		}
	}

	visibleCount := len(sel.TargetList)
	origins = make([]*ColumnOrigin, visibleCount)
	hiddenKeys := make([]string, visibleCount)

	type hiddenNeed struct {
		outerAlias string
		table      string
		pkCols     []string
		derived    *pg_query.SelectStmt // non-nil: also inject into this inner SELECT
		innerAlias string
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

	for i, item := range sel.TargetList {
		resTarget := item.GetResTarget()
		if resTarget == nil {
			continue
		}
		colRef := resTarget.Val.GetColumnRef()
		if colRef == nil || len(colRef.Fields) != 2 {
			continue // unqualified/computed column, or `*`/`alias.*`: ambiguous or unsupported
		}
		qualifier := colRef.Fields[0].GetString_()
		column := colRef.Fields[1].GetString_()
		if qualifier == nil || column == nil {
			continue // second field is `*` (alias.*) — not a single traceable column
		}
		outerAlias := qualifier.Sval
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

		table, innerAlias, ok := resolveDerivedColumnPostgres(ref.Derived, column.Sval)
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
		// Parsed fine, but no column's origin could be pinned down (or none
		// of their tables have a PK) — still "ok" so the caller uses these
		// all-nil origins rather than retrying (there's nothing more to try).
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
				innerRef := pg_query.MakeColumnRefNode(
					[]*pg_query.Node{pg_query.MakeStrNode(need.innerAlias), pg_query.MakeStrNode(pkCol)}, -1)
				need.derived.TargetList = append(need.derived.TargetList,
					pg_query.MakeResTargetNodeWithNameAndVal(hiddenName, innerRef, -1))
				outerColRef = hiddenName
			}
			outerRef := pg_query.MakeColumnRefNode(
				[]*pg_query.Node{pg_query.MakeStrNode(need.outerAlias), pg_query.MakeStrNode(outerColRef)}, -1)
			sel.TargetList = append(sel.TargetList, pg_query.MakeResTargetNodeWithNameAndVal(hiddenName, outerRef, -1))
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

	out, err := pgquery.Deparse(tree)
	if err != nil {
		return stmt, nil, false
	}
	return out, origins, true
}

// resolveDerivedColumnPostgres looks up outerColName in derived's own
// SELECT list (by its output name — an explicit AS, or a bare column
// ref's own name when unaliased, matching Postgres's own naming rule) and,
// if that item is itself a plain `alias.column` (or, when derived's FROM
// has exactly one table, a bare unqualified column) reference into a real
// table (or JOIN of real tables) in derived's own FROM, returns that table
// and the alias it's known by inside derived. ok is false for anything
// this narrow one-level pass can't trace — see resolveDerivedColumnMySQL,
// the same rules apply.
func resolveDerivedColumnPostgres(derived *pg_query.SelectStmt, outerColName string) (table, innerAlias string, ok bool) {
	if derived.Op != pg_query.SetOperation_SETOP_NONE {
		return "", "", false // UNION/INTERSECT/EXCEPT inside the derived table
	}
	if len(derived.DistinctClause) > 0 || len(derived.GroupClause) > 0 || derived.HavingClause != nil {
		return "", "", false
	}
	innerTables, tablesOK := collectJoinTablesPostgres(derived.FromClause)
	if !tablesOK {
		return "", "", false
	}

	for _, item := range derived.TargetList {
		resTarget := item.GetResTarget()
		if resTarget == nil {
			continue
		}
		colRef := resTarget.Val.GetColumnRef()
		if colRef == nil {
			continue // a computed expression: only nameable via explicit AS, checked below
		}
		exposedName := resTarget.Name
		if exposedName == "" {
			// Postgres's own naming rule: an unaliased plain column ref's
			// output name is its own (last) name part.
			last := colRef.Fields[len(colRef.Fields)-1].GetString_()
			if last == nil {
				continue
			}
			exposedName = last.Sval
		}
		if exposedName != outerColName {
			continue
		}

		var alias string
		switch len(colRef.Fields) {
		case 1:
			if len(innerTables) != 1 {
				return "", "", false // ambiguous without a schema to check against
			}
			for a := range innerTables {
				alias = a
			}
		case 2:
			qualifier := colRef.Fields[0].GetString_()
			if qualifier == nil {
				return "", "", false
			}
			alias = qualifier.Sval
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

// collectJoinTablesPostgres walks a FROM clause made of table references
// (optionally JOINed), mapping each alias (or bare table name when
// unaliased) to what it refers to — a real table, or (one level deep) a
// derived table. ok is false the moment it finds anything this narrow pass
// doesn't understand, so the caller can bail out rather than guess.
func collectJoinTablesPostgres(from []*pg_query.Node) (map[string]postgresTableRef, bool) {
	result := map[string]postgresTableRef{}
	var walk func(*pg_query.Node) bool
	walk = func(n *pg_query.Node) bool {
		if join := n.GetJoinExpr(); join != nil {
			return walk(join.Larg) && walk(join.Rarg)
		}
		if rv := n.GetRangeVar(); rv != nil {
			alias := rv.Relname
			if rv.Alias != nil && rv.Alias.Aliasname != "" {
				alias = rv.Alias.Aliasname
			}
			result[alias] = postgresTableRef{Table: rv.Relname}
			return true
		}
		if rs := n.GetRangeSubselect(); rs != nil {
			derivedSel := rs.Subquery.GetSelectStmt()
			if derivedSel == nil || derivedSel.Op != pg_query.SetOperation_SETOP_NONE || rs.Alias == nil || rs.Alias.Aliasname == "" {
				return false // a UNION in the subquery, or no alias — can't reference it
			}
			result[rs.Alias.Aliasname] = postgresTableRef{Derived: derivedSel}
			return true
		}
		return false
	}
	for _, n := range from {
		if !walk(n) {
			return nil, false
		}
	}
	return result, true
}

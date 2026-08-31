package query

import (
	"database/sql"
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	pgquery "github.com/wasilibs/go-pgquery"

	"dbtool/server/internal/dbconn"
	"dbtool/server/internal/store"
)

// prepareJoinOriginsPostgres is the Postgres dialect's ADR 0011 JOIN pass —
// see prepareJoinOriginsMySQL for the shared rationale and prepareJoinOrigins
// for how a kind picks its dialect. Structurally identical to the MySQL
// pass; only the AST shapes differ (pg_query_go's protobuf tree instead of
// vitess/xwb1989's Go-native one).
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
	if sel == nil {
		return stmt, nil, false
	}
	if len(sel.DistinctClause) > 0 || len(sel.GroupClause) > 0 || sel.HavingClause != nil {
		// See the MySQL pass: appending a hidden PK carrier column isn't
		// safe when GROUP BY/DISTINCT is in play — it can either error the
		// query out or silently change which rows count as distinct.
		return stmt, nil, false
	}

	aliasToTable, tablesOK := collectJoinTablesPostgres(sel.FromClause)
	if !tablesOK || len(aliasToTable) < 2 {
		return stmt, nil, false
	}

	visibleCount := len(sel.TargetList)
	origins = make([]*ColumnOrigin, visibleCount)

	type hiddenNeed struct {
		alias, table string
		pkCols       []string
	}
	tablePK := map[string][]string{}
	tableNeeded := map[string]bool{}
	var needed []hiddenNeed

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
		alias := qualifier.Sval
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
			colRefNode := pg_query.MakeColumnRefNode(
				[]*pg_query.Node{pg_query.MakeStrNode(need.alias), pg_query.MakeStrNode(pkCol)},
				-1,
			)
			sel.TargetList = append(sel.TargetList, pg_query.MakeResTargetNodeWithNameAndVal(hiddenName, colRefNode, -1))
			idxs[j] = nextIndex
			nextIndex++
		}
		hiddenIndex[need.alias] = idxs
	}

	for i, item := range sel.TargetList[:visibleCount] {
		if origins[i] == nil {
			continue
		}
		alias := item.GetResTarget().Val.GetColumnRef().Fields[0].GetString_().Sval
		origins[i].PrimaryKeyRowIndexes = hiddenIndex[alias]
	}

	out, err := pgquery.Deparse(tree)
	if err != nil {
		return stmt, nil, false
	}
	return out, origins, true
}

// collectJoinTablesPostgres walks a FROM clause made only of plain table
// references (optionally JOINed), mapping each alias (or bare table name
// when unaliased) to its bare table name. ok is false the moment it finds
// anything this narrow pass doesn't understand — a derived table/subquery,
// most commonly.
func collectJoinTablesPostgres(from []*pg_query.Node) (map[string]string, bool) {
	result := map[string]string{}
	var walk func(*pg_query.Node) bool
	walk = func(n *pg_query.Node) bool {
		if join := n.GetJoinExpr(); join != nil {
			return walk(join.Larg) && walk(join.Rarg)
		}
		rv := n.GetRangeVar()
		if rv == nil {
			return false // derived table (subquery) — not this pass's job
		}
		alias := rv.Relname
		if rv.Alias != nil && rv.Alias.Aliasname != "" {
			alias = rv.Alias.Aliasname
		}
		result[alias] = rv.Relname
		return true
	}
	for _, n := range from {
		if !walk(n) {
			return nil, false
		}
	}
	return result, true
}

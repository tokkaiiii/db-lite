package query

import (
	"database/sql"
	"fmt"

	"github.com/xwb1989/sqlparser"

	"dbtool/server/internal/dbconn"
	"dbtool/server/internal/store"
)

// prepareJoinOrigins is the ADR 0011 "JOIN" download path. ADR 0009 already
// covers PK-having single-table `SELECT *` via a narrow regex (rewrite.go);
// this handles the next narrowest shape a real parser can analyze safely: a
// SELECT over a plain multi-table JOIN (no derived tables/subqueries in
// FROM), where each output column is a simple `alias.column` reference.
//
// It's deliberately MySQL-only and JOIN-only for now — Postgres/MSSQL/Oracle
// dialects, subqueries/CTEs, and UNION are follow-up work tracked against
// ADR 0011 (see the wayfinder issue), not attempted here.
//
// ok is false whenever stmt isn't a shape this pass understands (parse
// failure, non-MySQL, no JOIN, a derived table, `SELECT *`): callers should
// treat the statement as having no origins, same as before ADR 0011.
func prepareJoinOrigins(db *sql.DB, kind store.DBKind, stmt string) (rewritten string, origins []*ColumnOrigin, ok bool) {
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

	aliasToTable, tablesOK := collectJoinTables(sel.From)
	if !tablesOK || len(aliasToTable) < 2 {
		return stmt, nil, false
	}

	visibleCount := len(sel.SelectExprs)
	origins = make([]*ColumnOrigin, visibleCount)

	type hiddenNeed struct {
		alias, table string
		pkCols       []string
	}
	tablePK := map[string][]string{}
	tableNeeded := map[string]bool{}
	var needed []hiddenNeed

	for i, se := range sel.SelectExprs {
		aliasedExpr, isAliased := se.(*sqlparser.AliasedExpr)
		if !isAliased {
			continue // *StarExpr or anything else: origin stays unknown (nil)
		}
		colName, isCol := aliasedExpr.Expr.(*sqlparser.ColName)
		if !isCol || colName.Qualifier.IsEmpty() {
			continue // unqualified or computed column in a JOIN: ambiguous
		}
		alias := colName.Qualifier.Name.String()
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
		// Parsed fine, but no column's origin could be pinned down (or none
		// of their tables have a PK) — still "ok" so the caller uses these
		// all-nil origins rather than retrying (there's nothing more to try).
		return stmt, origins, true
	}

	// Append one hidden `__pk_<alias>_<col>` carrier per PK column per
	// table that contributed a traceable column, so a later cell download
	// can re-fetch that row. These sit past visibleCount and are never
	// shown to the user (see Result.ColumnOrigins).
	hiddenIndex := map[string][]int{}
	nextIndex := visibleCount
	for _, need := range needed {
		idxs := make([]int, len(need.pkCols))
		for j, pkCol := range need.pkCols {
			hiddenName := fmt.Sprintf("__pk_%s_%s", need.alias, pkCol)
			sel.SelectExprs = append(sel.SelectExprs, &sqlparser.AliasedExpr{
				Expr: &sqlparser.ColName{
					Name:      sqlparser.NewColIdent(pkCol),
					Qualifier: sqlparser.TableName{Name: sqlparser.NewTableIdent(need.alias)},
				},
				As: sqlparser.NewColIdent(hiddenName),
			})
			idxs[j] = nextIndex
			nextIndex++
		}
		hiddenIndex[need.alias] = idxs
	}

	for i, se := range sel.SelectExprs[:visibleCount] {
		if origins[i] == nil {
			continue
		}
		colName := se.(*sqlparser.AliasedExpr).Expr.(*sqlparser.ColName)
		origins[i].PrimaryKeyRowIndexes = hiddenIndex[colName.Qualifier.Name.String()]
	}

	return sqlparser.String(sel), origins, true
}

// collectJoinTables walks a FROM clause made only of plain table references
// (optionally JOINed), mapping each alias (or bare table name when
// unaliased) to its bare table name. ok is false the moment it finds
// anything this narrow pass doesn't understand — a derived table/subquery,
// most commonly — so the caller can bail out rather than guess.
func collectJoinTables(from sqlparser.TableExprs) (map[string]string, bool) {
	result := map[string]string{}
	var walk func(sqlparser.TableExpr) bool
	walk = func(te sqlparser.TableExpr) bool {
		switch t := te.(type) {
		case *sqlparser.JoinTableExpr:
			return walk(t.LeftExpr) && walk(t.RightExpr)
		case *sqlparser.AliasedTableExpr:
			tableName, isTable := t.Expr.(sqlparser.TableName)
			if !isTable {
				return false // derived table (subquery) — not this pass's job
			}
			alias := t.As.String()
			if alias == "" {
				alias = tableName.Name.String()
			}
			result[alias] = bareTableName(tableName.Name.String())
			return true
		default:
			return false
		}
	}
	for _, te := range from {
		if !walk(te) {
			return nil, false
		}
	}
	return result, true
}

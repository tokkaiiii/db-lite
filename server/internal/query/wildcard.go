package query

import (
	"database/sql"

	"dbtool/server/internal/dbconn"
	"dbtool/server/internal/store"
)

// expandWildcardOrigins computes ColumnOrigins for a bare `SELECT *` whose
// FROM lists only real tables, named in tables in FROM order — see
// prepareJoinOriginsMySQL and its sibling functions for how each dialect
// reaches this. SQL's expansion rule for a bare `*` is well-defined and
// consistent across MySQL/Postgres/MSSQL/Oracle: each FROM table's columns
// appear in that table's own schema order, tables themselves in FROM
// order — so no rewrite is needed at all, this just has to enumerate the
// same order the driver will.
//
// ok is false if any table's column list can't be determined. Unlike
// other origin-tracking paths, a partial per-column fail-closed isn't
// possible here: this function's entire premise is knowing exactly how
// many physical columns precede each subsequent table's block, so one
// missing table's column count breaks every position after it — the
// whole statement's origin tracking must be abandoned.
func expandWildcardOrigins(db *sql.DB, kind store.DBKind, tables []string) (origins []*ColumnOrigin, ok bool) {
	for _, table := range tables {
		cols, err := dbconn.LOBColumns(db, kind, table)
		if err != nil || len(cols) == 0 {
			return nil, false
		}

		pk, err := dbconn.PrimaryKeyColumns(db, kind, table)
		if err != nil {
			pk = nil
		}
		var pkIdx []int
		if len(pk) > 0 {
			base := len(origins)
			pkIdx = make([]int, len(pk))
			for i, pkCol := range pk {
				pkIdx[i] = -1
				for j, c := range cols {
					if c.Name == pkCol {
						pkIdx[i] = base + j
						break
					}
				}
			}
			for _, idx := range pkIdx {
				if idx == -1 {
					// A PK column name didn't match any column LOBColumns
					// returned — shouldn't happen for a real table, but if
					// it does, the indexes can't be trusted.
					pk = nil
					break
				}
			}
		}

		for range cols {
			if len(pk) > 0 {
				origins = append(origins, &ColumnOrigin{Table: table, PrimaryKeyColumns: pk, PrimaryKeyRowIndexes: pkIdx})
			} else {
				origins = append(origins, nil)
			}
		}
	}
	return origins, true
}

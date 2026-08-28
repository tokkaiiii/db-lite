package dbconn

import (
	"database/sql"
	"fmt"
	"strings"

	"dbtool/server/internal/store"
)

// lobColumnQueries looks up every column of one table — name, declared SQL
// type, and (for MSSQL only, where it distinguishes "varchar(4000)" from
// "varchar(max)") its declared character length. Scoped to the same single
// default schema as schemaQueries (dbo/public) for the same reason.
var lobColumnQueries = map[store.DBKind]string{
	store.DBKindMySQL: `
		SELECT COLUMN_NAME, DATA_TYPE, NULL
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION`,
	store.DBKindMSSQL: `
		SELECT COLUMN_NAME, DATA_TYPE, CHARACTER_MAXIMUM_LENGTH
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = 'dbo' AND TABLE_NAME = @p1
		ORDER BY ORDINAL_POSITION`,
	store.DBKindPostgres: `
		SELECT column_name, data_type, NULL
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position`,
	// Oracle stores unquoted identifiers upper-cased, and this comparison
	// is case-sensitive, so a query written as `FROM docs` (lower/mixed
	// case) won't match and simply finds no columns — which is fine, ADR
	// 0008's rewrite fails open in that case (verified against a real
	// Oracle container: `FROM docs` falls back to post-fetch truncation
	// only, `FROM DOCS` gets the SQL-level rewrite).
	store.DBKindOracle: `
		SELECT COLUMN_NAME, DATA_TYPE, NULL
		FROM USER_TAB_COLUMNS
		WHERE TABLE_NAME = :1
		ORDER BY COLUMN_ID`,
}

// LOBColumn is one column of a table considered for the ADR 0008 rewrite:
// its original name, and the fully quoted/aliased SQL fragment that should
// replace it in a rewritten SELECT list.
type LOBColumn struct {
	Name       string
	SelectExpr string
	IsLOB      bool
}

// LOBColumns returns every column of table (bare, unqualified, unquoted —
// see query.ParsedSelectStar) on db, already open for kind, in declaration
// order. A nil/empty result (with a nil error) means "nothing usable was
// found" — callers should treat that the same as an error and fall back to
// running the original statement unmodified, per ADR 0008's "fail open"
// rule: this is an optional optimization, never a correctness requirement.
func LOBColumns(db *sql.DB, kind store.DBKind, table string) ([]LOBColumn, error) {
	q, ok := lobColumnQueries[kind]
	if !ok {
		return nil, nil
	}

	rows, err := db.Query(q, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []LOBColumn
	for rows.Next() {
		var name, dataType string
		var maxLen sql.NullInt64
		if err := rows.Scan(&name, &dataType, &maxLen); err != nil {
			return nil, err
		}
		isLOB := isLOBType(kind, dataType, maxLen)
		columns = append(columns, LOBColumn{
			Name:       name,
			SelectExpr: columnSelectExpr(kind, name, dataType, isLOB),
			IsLOB:      isLOB,
		})
	}
	return columns, rows.Err()
}

// isLOBType reports whether a column of this declared type has no fixed
// upper bound on size — MSSQL's "(max)" types, MySQL's TEXT/BLOB family,
// Postgres's TEXT/XML/BYTEA, Oracle's CLOB/NCLOB/BLOB/LONG. A
// length-bounded type (varchar(4000), ...) is excluded even if large: its
// worst case is predictable, so ADR 0007's post-fetch truncation alone is
// enough and it doesn't need a DB-side rewrite.
func isLOBType(kind store.DBKind, dataType string, maxLen sql.NullInt64) bool {
	dt := strings.ToLower(dataType)
	switch kind {
	case store.DBKindMSSQL:
		switch dt {
		case "text", "ntext", "image", "xml":
			return true
		case "varchar", "nvarchar", "varbinary":
			// INFORMATION_SCHEMA.COLUMNS reports "(max)" as -1.
			return maxLen.Valid && maxLen.Int64 == -1
		}
	case store.DBKindMySQL:
		switch dt {
		case "tinytext", "text", "mediumtext", "longtext",
			"tinyblob", "blob", "mediumblob", "longblob":
			return true
		}
	case store.DBKindPostgres:
		switch dt {
		case "text", "xml", "bytea":
			return true
		}
	case store.DBKindOracle:
		switch dt {
		case "clob", "nclob", "blob", "long":
			return true
		}
	}
	return false
}

// columnSelectExpr returns the SQL fragment for one column in a rewritten
// SELECT list: the quoted column name alone if it isn't a LOB-class column,
// or a dialect-appropriate "give me only the first 2000 <units>" wrapper
// (aliased back to the original name) if it is. See ADR 0008 for why 2000
// doesn't need to line up exactly with ADR 0007's 2000-byte cap.
func columnSelectExpr(kind store.DBKind, name, dataType string, isLOB bool) string {
	quoted := quoteIdent(kind, name)
	if !isLOB {
		return quoted
	}
	return fmt.Sprintf(lobWrapTemplate(kind, strings.ToLower(dataType)), quoted) + " AS " + quoted
}

// lobWrapTemplate returns a fmt.Sprintf template (one %s, the quoted
// column reference) that truncates that column to ~2000 units server-side.
func lobWrapTemplate(kind store.DBKind, dataType string) string {
	switch kind {
	case store.DBKindMSSQL:
		if dataType == "xml" {
			// SUBSTRING doesn't accept xml directly; cast to text first.
			return "SUBSTRING(CAST(%s AS nvarchar(max)), 1, 2000)"
		}
		// SUBSTRING (unlike LEFT) also accepts varbinary/image directly.
		return "SUBSTRING(%s, 1, 2000)"
	case store.DBKindMySQL:
		return "SUBSTRING(%s, 1, 2000)"
	case store.DBKindPostgres:
		if dataType == "xml" {
			return "substring(%s::text, 1, 2000)"
		}
		return "substring(%s, 1, 2000)"
	case store.DBKindOracle:
		if dataType == "long" {
			// DBMS_LOB.SUBSTR doesn't support the legacy LONG type.
			return "SUBSTR(%s, 1, 2000)"
		}
		// 2000 stays under the default (non-extended) 4000/2000-byte
		// VARCHAR2/RAW limits DBMS_LOB.SUBSTR's return value is bound by.
		return "DBMS_LOB.SUBSTR(%s, 2000, 1)"
	}
	return "%s"
}

// quoteIdent wraps name in the dialect's identifier-quoting syntax, so a
// column name that collides with a reserved word or contains special
// characters still works in the rewritten SQL.
func quoteIdent(kind store.DBKind, name string) string {
	switch kind {
	case store.DBKindMSSQL:
		return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
	case store.DBKindMySQL:
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	case store.DBKindPostgres, store.DBKindOracle:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return name
}

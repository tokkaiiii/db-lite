package query

import (
	"database/sql"
	"regexp"
	"strings"

	"dbtool/server/internal/dbconn"
	"dbtool/server/internal/store"
)

var (
	selectStarRe = regexp.MustCompile(`(?i)^select\s+\*\s+from\s+`)
	identTokenRe = regexp.MustCompile(`^[A-Za-z0-9_."` + "`" + `\[\]]+`)
	asKeywordRe  = regexp.MustCompile(`(?i)^as\s+`)
	whereOrderRe = regexp.MustCompile(`(?i)^(where\b|order\s+by\b)`)
)

// ParsedSelectStar is the decomposition of a `SELECT * FROM <table> [[AS]
// alias] [WHERE ...] [ORDER BY ...]` statement — the only shape narrow
// enough to rewrite safely without a real SQL parser (see ADR 0008).
// Anything more (JOIN, subqueries, an explicit column list, CTEs, UNION,
// multiple statements, comments, GROUP BY/LIMIT/...) fails to parse.
type ParsedSelectStar struct {
	Table string // raw token as written: may be schema-qualified/quoted
	Alias string // "" if none was given
	Tail  string // "WHERE ..." / "ORDER BY ..." verbatim, or ""
}

// parseSelectStarSingleTable recognizes the narrow statement shape
// ParsedSelectStar documents. It fails closed: anything ambiguous or
// unrecognized returns ok=false rather than guessing, since a wrong
// rewrite would silently change query results.
func parseSelectStarSingleTable(stmt string) (p ParsedSelectStar, ok bool) {
	full := strings.TrimSpace(stmt)
	if strings.Contains(full, "--") || strings.Contains(full, "/*") {
		return p, false
	}
	if strings.HasSuffix(full, ";") {
		full = strings.TrimSpace(strings.TrimSuffix(full, ";"))
	}
	if strings.Contains(full, ";") {
		return p, false // more than one statement
	}

	loc := selectStarRe.FindStringIndex(full)
	if loc == nil {
		return p, false
	}
	rest := full[loc[1]:]

	table := identTokenRe.FindString(rest)
	if table == "" {
		return p, false
	}
	p.Table = table
	rest = strings.TrimSpace(rest[len(table):])

	if rest == "" {
		return p, true
	}
	if whereOrderRe.MatchString(rest) {
		p.Tail = rest
		return p, true
	}

	if loc := asKeywordRe.FindStringIndex(rest); loc != nil {
		rest = rest[loc[1]:]
	}
	alias := identTokenRe.FindString(rest)
	if alias == "" {
		return ParsedSelectStar{}, false
	}
	p.Alias = alias
	rest = strings.TrimSpace(rest[len(alias):])

	if rest == "" {
		return p, true
	}
	if !whereOrderRe.MatchString(rest) {
		return ParsedSelectStar{}, false
	}
	p.Tail = rest
	return p, true
}

// bareTableName strips a schema qualifier ("dbo.t" -> "t") and any
// quoting ([t], `t`, "t") so the name can be matched against an
// information_schema-style TABLE_NAME column.
func bareTableName(raw string) string {
	segments := strings.Split(raw, ".")
	last := segments[len(segments)-1]
	return strings.Trim(last, `[]"`+"`")
}

// prepareSelectStar inspects stmt once for the narrow `SELECT * FROM
// <table>` shape both ADR 0008 and ADR 0009 depend on, and returns:
//
//   - rewritten: stmt, pre-truncating any LOB-class column on the DB
//     server itself (ADR 0008) when the shape matched and the table has
//     one; stmt unchanged otherwise.
//   - table, primaryKey: the bare table name and its primary key columns
//     (ADR 0009), set only when the shape matched AND the table has a
//     primary key; zero values otherwise.
//
// Both lookups fail open: a parse failure, a lookup error, or nothing
// worth acting on all just fall back to "not available" rather than
// surfacing an error — these are optional capabilities layered on top of
// running the statement, never a correctness requirement.
func prepareSelectStar(db *sql.DB, kind store.DBKind, stmt string) (rewritten, table string, primaryKey []string) {
	rewritten = stmt

	parsed, ok := parseSelectStarSingleTable(stmt)
	if !ok {
		return rewritten, "", nil
	}
	bare := bareTableName(parsed.Table)
	if bare == "" {
		return rewritten, "", nil
	}

	if columns, err := dbconn.LOBColumns(db, kind, bare); err == nil {
		hasLOB := false
		for _, c := range columns {
			if c.IsLOB {
				hasLOB = true
				break
			}
		}
		if hasLOB {
			rewritten = buildRewrittenSelect(parsed, columns)
		}
	}

	if pk, err := dbconn.PrimaryKeyColumns(db, kind, bare); err == nil && len(pk) > 0 {
		table, primaryKey = bare, pk
	}

	return rewritten, table, primaryKey
}

func buildRewrittenSelect(p ParsedSelectStar, columns []dbconn.LOBColumn) string {
	exprs := make([]string, len(columns))
	for i, c := range columns {
		exprs[i] = c.SelectExpr
	}

	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(strings.Join(exprs, ", "))
	b.WriteString(" FROM ")
	b.WriteString(p.Table)
	if p.Alias != "" {
		b.WriteString(" ")
		b.WriteString(p.Alias)
	}
	if p.Tail != "" {
		b.WriteString(" ")
		b.WriteString(p.Tail)
	}
	return b.String()
}

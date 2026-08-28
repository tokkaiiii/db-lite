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

// rewriteSelectStarLOB rewrites stmt to pre-truncate LOB-class columns on
// the DB server itself (ADR 0008), when stmt is a plain `SELECT * FROM
// <table>` and that table has at least one such column. Any failure to
// parse, look up, or find anything worth rewriting returns stmt unchanged
// — this is an optional optimization, never a correctness requirement, so
// it always fails open rather than surfacing an error to the caller.
func rewriteSelectStarLOB(db *sql.DB, kind store.DBKind, stmt string) string {
	parsed, ok := parseSelectStarSingleTable(stmt)
	if !ok {
		return stmt
	}
	table := bareTableName(parsed.Table)
	if table == "" {
		return stmt
	}

	columns, err := dbconn.LOBColumns(db, kind, table)
	if err != nil || len(columns) == 0 {
		return stmt
	}

	hasLOB := false
	for _, c := range columns {
		if c.IsLOB {
			hasLOB = true
			break
		}
	}
	if !hasLOB {
		return stmt
	}

	return buildRewrittenSelect(parsed, columns)
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

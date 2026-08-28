package query

import "strings"

// readOnlyKeywords are the statement-leading keywords considered read-only.
// Everything else (INSERT/UPDATE/DELETE/DDL/stored procedure calls/...) is
// a Write Query per CONTEXT.md.
var readOnlyKeywords = map[string]bool{
	"SELECT":  true,
	"SHOW":    true,
	"EXPLAIN": true,
}

// IsWrite reports whether stmt is a Write Query, judged solely by its
// leading keyword.
func IsWrite(stmt string) bool {
	trimmed := strings.TrimSpace(stmt)
	if trimmed == "" {
		return false
	}
	firstWord := strings.ToUpper(strings.Fields(trimmed)[0])
	return !readOnlyKeywords[firstWord]
}

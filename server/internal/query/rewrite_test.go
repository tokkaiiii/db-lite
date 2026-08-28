package query

import (
	"testing"

	"dbtool/server/internal/dbconn"
)

func TestParseSelectStarSingleTable_Matches(t *testing.T) {
	tests := []struct {
		name      string
		stmt      string
		wantTable string
		wantAlias string
		wantTail  string
	}{
		{"bare", "SELECT * FROM users", "users", "", ""},
		{"lowercase keywords", "select * from users", "users", "", ""},
		{"trailing semicolon", "SELECT * FROM users;", "users", "", ""},
		{"extra whitespace", "SELECT   *   FROM\nusers", "users", "", ""},
		{"alias", "SELECT * FROM users u", "users", "u", ""},
		{"AS alias", "SELECT * FROM users AS u", "users", "u", ""},
		{"where", "SELECT * FROM users WHERE id = 1", "users", "", "WHERE id = 1"},
		{"order by", "SELECT * FROM users ORDER BY id", "users", "", "ORDER BY id"},
		{"alias + where", "SELECT * FROM users u WHERE u.id = 1", "users", "u", "WHERE u.id = 1"},
		{"alias + order by", "SELECT * FROM users u ORDER BY u.id", "users", "u", "ORDER BY u.id"},
		{"schema qualified", "SELECT * FROM dbo.users", "dbo.users", "", ""},
		{"bracket quoted", "SELECT * FROM [dbo].[users]", "[dbo].[users]", "", ""},
		{"backtick quoted", "SELECT * FROM `mydb`.`users`", "`mydb`.`users`", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := parseSelectStarSingleTable(tt.stmt)
			if !ok {
				t.Fatalf("parseSelectStarSingleTable(%q) failed to match, want table=%q", tt.stmt, tt.wantTable)
			}
			if p.Table != tt.wantTable {
				t.Errorf("Table = %q, want %q", p.Table, tt.wantTable)
			}
			if p.Alias != tt.wantAlias {
				t.Errorf("Alias = %q, want %q", p.Alias, tt.wantAlias)
			}
			if p.Tail != tt.wantTail {
				t.Errorf("Tail = %q, want %q", p.Tail, tt.wantTail)
			}
		})
	}
}

func TestParseSelectStarSingleTable_Rejects(t *testing.T) {
	tests := []string{
		"SELECT id, name FROM users",
		"SELECT * FROM users JOIN orders ON users.id = orders.user_id",
		"SELECT * FROM (SELECT * FROM users) t",
		"SELECT * FROM users GROUP BY id",
		"SELECT * FROM users LIMIT 10",
		"SELECT * FROM users; DROP TABLE users;",
		"SELECT * FROM users -- comment",
		"SELECT * FROM users /* comment */",
		"WITH cte AS (SELECT * FROM users) SELECT * FROM cte",
		"INSERT INTO users VALUES (1)",
		"",
	}
	for _, stmt := range tests {
		t.Run(stmt, func(t *testing.T) {
			if _, ok := parseSelectStarSingleTable(stmt); ok {
				t.Errorf("parseSelectStarSingleTable(%q) matched, want reject", stmt)
			}
		})
	}
}

func TestBareTableName(t *testing.T) {
	tests := map[string]string{
		"users":              "users",
		"dbo.users":          "users",
		"[dbo].[users]":      "users",
		"`mydb`.`users`":     "users",
		`"public"."users"`:   "users",
		"[Users With Space]": "Users With Space",
	}
	for raw, want := range tests {
		if got := bareTableName(raw); got != want {
			t.Errorf("bareTableName(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestBuildRewrittenSelect(t *testing.T) {
	p := ParsedSelectStar{Table: "users", Alias: "u", Tail: "WHERE u.id = 1"}
	columns := []dbconn.LOBColumn{
		{Name: "id", SelectExpr: `"id"`},
		{Name: "payload", SelectExpr: `SUBSTRING("payload", 1, 2000) AS "payload"`, IsLOB: true},
	}

	got := buildRewrittenSelect(p, columns)
	want := `SELECT "id", SUBSTRING("payload", 1, 2000) AS "payload" FROM users u WHERE u.id = 1`
	if got != want {
		t.Errorf("buildRewrittenSelect() = %q, want %q", got, want)
	}
}

func TestBuildRewrittenSelect_NoAliasNoTail(t *testing.T) {
	p := ParsedSelectStar{Table: "users"}
	columns := []dbconn.LOBColumn{{Name: "id", SelectExpr: `"id"`}}

	got := buildRewrittenSelect(p, columns)
	want := `SELECT "id" FROM users`
	if got != want {
		t.Errorf("buildRewrittenSelect() = %q, want %q", got, want)
	}
}

package query

import "testing"

func TestIsWrite(t *testing.T) {
	cases := []struct {
		name string
		stmt string
		want bool
	}{
		{"select", "SELECT * FROM users", false},
		{"select lowercase", "select * from users", false},
		{"show", "SHOW TABLES", false},
		{"explain", "EXPLAIN SELECT 1", false},
		{"leading whitespace", "  \n\tSELECT 1", false},
		{"insert", "INSERT INTO users (name) VALUES ('a')", true},
		{"update", "UPDATE users SET name = 'a'", true},
		{"delete", "DELETE FROM users", true},
		{"ddl", "CREATE TABLE t (id INT)", true},
		{"stored procedure call", "CALL do_something()", true},
		{"empty", "", false},
		{"whitespace only", "   ", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsWrite(c.stmt); got != c.want {
				t.Errorf("IsWrite(%q) = %v, want %v", c.stmt, got, c.want)
			}
		})
	}
}

package query

import (
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTruncateCell_UnderLimit(t *testing.T) {
	s := "짧은 문자열"
	if got := truncateCell(s); got != s {
		t.Errorf("truncateCell(%q) = %q, want unchanged", s, got)
	}
}

func TestTruncateCell_OverLimit(t *testing.T) {
	big := strings.Repeat("a", maxCellBytes+500)
	got := truncateCell(big)

	if len(got) == len(big) {
		t.Fatalf("expected truncation, got same length %d", len(got))
	}
	if !strings.HasPrefix(got, strings.Repeat("a", maxCellBytes)) {
		t.Errorf("truncated value should start with the first %d bytes unchanged", maxCellBytes)
	}
	if !strings.Contains(got, "잘림") {
		t.Errorf("truncated value should contain the truncation marker, got %q", got)
	}
	if !strings.Contains(got, "원본 2500바이트") {
		t.Errorf("truncated value should report the original byte count, got %q", got)
	}
}

func TestTruncateCell_DoesNotSplitMultiByteRune(t *testing.T) {
	// 한글은 UTF-8에서 3바이트이므로, maxCellBytes가 3의 배수가 아니면
	// 단순 바이트 슬라이싱은 글자 중간을 잘라 깨진 문자를 만든다.
	big := strings.Repeat("가", maxCellBytes) // 훨씬 더 많은 바이트
	got := truncateCell(big)

	marker := "...(잘림, 원본"
	idx := strings.Index(got, marker)
	if idx == -1 {
		t.Fatalf("expected truncation marker in %q", got)
	}
	cut := got[:idx]
	for _, r := range cut {
		if r == '�' {
			t.Fatalf("truncated value contains a broken rune: %q", cut)
		}
	}
}

func TestExecuteRead_TruncatesLargeCell(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec("CREATE TABLE t (id INTEGER, payload TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	big := strings.Repeat("x", maxCellBytes+100)
	if _, err := db.Exec("INSERT INTO t (id, payload) VALUES (1, ?)", big); err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := Execute(db, "SELECT id, payload FROM t")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	payload, ok := result.Rows[0][1].(string)
	if !ok {
		t.Fatalf("expected payload to be a string, got %T", result.Rows[0][1])
	}
	if len(payload) >= len(big) {
		t.Errorf("expected payload cell to be truncated, got length %d", len(payload))
	}
	if !strings.Contains(payload, "잘림") {
		t.Errorf("expected truncation marker in payload cell, got %q", payload)
	}
}

func TestExecuteRead_LeavesSmallCellUntouched(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec("CREATE TABLE t (id INTEGER, name TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t (id, name) VALUES (1, 'hello')"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := Execute(db, "SELECT id, name FROM t")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := result.Rows[0][1]; got != "hello" {
		t.Errorf("name cell = %v, want %q", got, "hello")
	}
}

package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite database holding app metadata: users, connections,
// permissions, and the audit log. It is deliberately separate from any
// target Connection's own database.
type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	is_admin      INTEGER NOT NULL DEFAULT 0,
	created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS connections (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL UNIQUE,
	kind       TEXT NOT NULL,
	host       TEXT NOT NULL,
	port       INTEGER NOT NULL,
	username   TEXT NOT NULL,
	password   TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS permissions (
	user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	connection_id INTEGER NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
	level         TEXT NOT NULL,
	PRIMARY KEY (user_id, connection_id)
);

CREATE TABLE IF NOT EXISTS audit_log (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id       INTEGER NOT NULL REFERENCES users(id),
	connection_id INTEGER NOT NULL REFERENCES connections(id),
	statement     TEXT NOT NULL,
	allowed       INTEGER NOT NULL,
	error_message TEXT NOT NULL DEFAULT '',
	executed_at   TEXT NOT NULL DEFAULT (datetime('now'))
);
`
	_, err := s.db.Exec(schema)
	return err
}

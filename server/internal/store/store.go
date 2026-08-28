package store

import (
	"database/sql"
	"fmt"
	"strings"

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
	// database/sql pools multiple underlying SQLite connections, and
	// PRAGMA foreign_keys is per-connection — setting it via db.Exec only
	// reaches whichever connection handles that one call. A second
	// connection later borrowed from the pool would silently run with
	// foreign keys OFF (SQLite's default), so foreign key errors like the
	// one relaxAuditLogForeignKeysIfNeeded fixed could go unenforced again.
	// Pinning the pool to one connection makes the pragma reliably apply
	// to every statement; it also sidesteps SQLite's poor concurrent-write
	// story, which this app's SQLite-backed metadata store doesn't need
	// more than one writer for anyway.
	db.SetMaxOpenConns(1)
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
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	name         TEXT NOT NULL UNIQUE,
	kind         TEXT NOT NULL,
	host         TEXT NOT NULL,
	port         INTEGER NOT NULL,
	username     TEXT NOT NULL,
	password     TEXT NOT NULL,
	service_name TEXT NOT NULL DEFAULT '',
	created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS permissions (
	user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	connection_id INTEGER NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
	level         TEXT NOT NULL,
	PRIMARY KEY (user_id, connection_id)
);

-- user_id/connection_id are nullable with ON DELETE SET NULL: deleting a
-- User or Connection must not be blocked by (nor erase) its audit history,
-- since Audit Log Entry is meant to be a persistent record.
CREATE TABLE IF NOT EXISTS audit_log (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id       INTEGER REFERENCES users(id) ON DELETE SET NULL,
	connection_id INTEGER REFERENCES connections(id) ON DELETE SET NULL,
	statement     TEXT NOT NULL,
	allowed       INTEGER NOT NULL,
	error_message TEXT NOT NULL DEFAULT '',
	executed_at   TEXT NOT NULL DEFAULT (datetime('now'))
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	if err := s.addServiceNameColumnIfMissing(); err != nil {
		return err
	}
	return s.relaxAuditLogForeignKeysIfNeeded()
}

// addServiceNameColumnIfMissing upgrades a connections table created before
// service_name existed (CREATE TABLE IF NOT EXISTS above is a no-op against
// an already-existing table, so new installs get the column but old ones
// need this explicit migration).
func (s *Store) addServiceNameColumnIfMissing() error {
	_, err := s.db.Exec(`ALTER TABLE connections ADD COLUMN service_name TEXT NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	return nil
}

// relaxAuditLogForeignKeysIfNeeded upgrades an audit_log table created
// before its foreign keys were ON DELETE SET NULL (CREATE TABLE IF NOT
// EXISTS above is a no-op against an already-existing table). Under the old
// NO ACTION default, deleting a User or Connection that had ever appeared
// in the audit log failed with a foreign key constraint violation. SQLite
// can't ALTER a column's foreign key clause in place, so this rebuilds the
// table.
func (s *Store) relaxAuditLogForeignKeysIfNeeded() error {
	rows, err := s.db.Query(`PRAGMA foreign_key_list(audit_log)`)
	if err != nil {
		return err
	}
	needsMigration := false
	for rows.Next() {
		var id, seq int
		var table, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			rows.Close()
			return err
		}
		if onDelete != "SET NULL" {
			needsMigration = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !needsMigration {
		return nil
	}

	_, err = s.db.Exec(`
BEGIN TRANSACTION;
ALTER TABLE audit_log RENAME TO audit_log_old;
CREATE TABLE audit_log (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id       INTEGER REFERENCES users(id) ON DELETE SET NULL,
	connection_id INTEGER REFERENCES connections(id) ON DELETE SET NULL,
	statement     TEXT NOT NULL,
	allowed       INTEGER NOT NULL,
	error_message TEXT NOT NULL DEFAULT '',
	executed_at   TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO audit_log (id, user_id, connection_id, statement, allowed, error_message, executed_at)
	SELECT o.id,
	       CASE WHEN u.id IS NULL THEN NULL ELSE o.user_id END,
	       CASE WHEN c.id IS NULL THEN NULL ELSE o.connection_id END,
	       o.statement, o.allowed, o.error_message, o.executed_at
	FROM audit_log_old o
	LEFT JOIN users u ON u.id = o.user_id
	LEFT JOIN connections c ON c.id = o.connection_id;
DROP TABLE audit_log_old;
COMMIT;
`)
	return err
}

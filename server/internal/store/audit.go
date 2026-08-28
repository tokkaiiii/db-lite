package store

import (
	"database/sql"
	"time"
)

func (s *Store) InsertAuditLogEntry(e AuditLogEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO audit_log (user_id, connection_id, statement, allowed, error_message) VALUES (?, ?, ?, ?, ?)`,
		e.UserID, e.ConnectionID, e.Statement, e.Allowed, e.ErrorMessage,
	)
	return err
}

func (s *Store) ListAuditLog(limit int) ([]AuditLogEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, connection_id, statement, allowed, error_message, executed_at
		 FROM audit_log ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AuditLogEntry{}
	for rows.Next() {
		var e AuditLogEntry
		var userID, connectionID sql.NullInt64
		var executedAt string
		if err := rows.Scan(&e.ID, &userID, &connectionID, &e.Statement, &e.Allowed, &e.ErrorMessage, &executedAt); err != nil {
			return nil, err
		}
		// 0 means the User or Connection this entry originally referenced
		// has since been deleted (user_id/connection_id are ON DELETE SET
		// NULL so the entry itself survives).
		e.UserID = userID.Int64
		e.ConnectionID = connectionID.Int64
		e.ExecutedAt, _ = time.Parse(time.DateTime, executedAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

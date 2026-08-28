package store

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

	var out []AuditLogEntry
	for rows.Next() {
		var e AuditLogEntry
		var executedAt string
		if err := rows.Scan(&e.ID, &e.UserID, &e.ConnectionID, &e.Statement, &e.Allowed, &e.ErrorMessage, &executedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

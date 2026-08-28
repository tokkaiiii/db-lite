package store

import "database/sql"

// SetPermission upserts the access level a User has on a Connection.
// Setting PermissionNone removes the grant entirely rather than storing it,
// so ListPermissionsForUser only ever returns Connections the user can see.
func (s *Store) SetPermission(userID, connectionID int64, level PermissionLevel) error {
	if level == PermissionNone {
		_, err := s.db.Exec(`DELETE FROM permissions WHERE user_id = ? AND connection_id = ?`, userID, connectionID)
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO permissions (user_id, connection_id, level) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, connection_id) DO UPDATE SET level = excluded.level`,
		userID, connectionID, level,
	)
	return err
}

// GetPermission returns the User's level on the Connection, defaulting to
// PermissionNone when no grant row exists.
func (s *Store) GetPermission(userID, connectionID int64) (PermissionLevel, error) {
	var level PermissionLevel
	err := s.db.QueryRow(
		`SELECT level FROM permissions WHERE user_id = ? AND connection_id = ?`,
		userID, connectionID,
	).Scan(&level)
	if err == sql.ErrNoRows {
		return PermissionNone, nil
	}
	if err != nil {
		return "", err
	}
	return level, nil
}

// ListPermissionsForUser returns every Connection the given User has any
// (non-none) access to, together with the granted level.
func (s *Store) ListPermissionsForUser(userID int64) ([]Permission, error) {
	rows, err := s.db.Query(
		`SELECT user_id, connection_id, level FROM permissions WHERE user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.UserID, &p.ConnectionID, &p.Level); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

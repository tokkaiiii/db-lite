package store

import (
	"database/sql"
	"errors"
	"time"
)

func (s *Store) CreateConnection(c Connection) (*Connection, error) {
	res, err := s.db.Exec(
		`INSERT INTO connections (name, kind, host, port, username, password, service_name) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Kind, c.Host, c.Port, c.Username, c.Password, c.ServiceName,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetConnection(id)
}

func (s *Store) GetConnection(id int64) (*Connection, error) {
	row := s.db.QueryRow(
		`SELECT id, name, kind, host, port, username, password, service_name, created_at FROM connections WHERE id = ?`,
		id,
	)
	return scanConnection(row)
}

func (s *Store) ListConnections() ([]Connection, error) {
	rows, err := s.db.Query(`SELECT id, name, kind, host, port, username, password, service_name, created_at FROM connections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Connection{}
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *Store) UpdateConnection(c Connection) (*Connection, error) {
	_, err := s.db.Exec(
		`UPDATE connections SET name = ?, kind = ?, host = ?, port = ?, username = ?, password = ?, service_name = ? WHERE id = ?`,
		c.Name, c.Kind, c.Host, c.Port, c.Username, c.Password, c.ServiceName, c.ID,
	)
	if err != nil {
		return nil, err
	}
	return s.GetConnection(c.ID)
}

func (s *Store) DeleteConnection(id int64) error {
	_, err := s.db.Exec(`DELETE FROM connections WHERE id = ?`, id)
	return err
}

func scanConnection(row rowScanner) (*Connection, error) {
	var c Connection
	var createdAt string
	if err := row.Scan(&c.ID, &c.Name, &c.Kind, &c.Host, &c.Port, &c.Username, &c.Password, &c.ServiceName, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.DateTime, createdAt)
	return &c, nil
}

package store

import "testing"

// newTestStore opens an isolated in-memory SQLite store for a single test.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mustCreateUser(t *testing.T, s *Store, username string) *User {
	t.Helper()
	u, err := s.CreateUser(username, "hash", false)
	if err != nil {
		t.Fatalf("CreateUser(%q): %v", username, err)
	}
	return u
}

func mustCreateConnection(t *testing.T, s *Store, name string) *Connection {
	t.Helper()
	c, err := s.CreateConnection(Connection{
		Name: name, Kind: DBKindMySQL, Host: "localhost", Port: 3306,
		Username: "u", Password: "p",
	})
	if err != nil {
		t.Fatalf("CreateConnection(%q): %v", name, err)
	}
	return c
}

func TestGetPermissionDefaultsToNone(t *testing.T) {
	s := newTestStore(t)
	u := mustCreateUser(t, s, "alice")
	c := mustCreateConnection(t, s, "conn1")

	level, err := s.GetPermission(u.ID, c.ID)
	if err != nil {
		t.Fatalf("GetPermission: %v", err)
	}
	if level != PermissionNone {
		t.Errorf("GetPermission on ungranted pair = %q, want %q", level, PermissionNone)
	}
}

func TestSetPermissionGrantsAndUpdates(t *testing.T) {
	s := newTestStore(t)
	u := mustCreateUser(t, s, "alice")
	c := mustCreateConnection(t, s, "conn1")

	if err := s.SetPermission(u.ID, c.ID, PermissionRead); err != nil {
		t.Fatalf("SetPermission(read): %v", err)
	}
	if level, _ := s.GetPermission(u.ID, c.ID); level != PermissionRead {
		t.Fatalf("after granting read, GetPermission = %q", level)
	}

	// Re-granting at a different level should update in place, not duplicate.
	if err := s.SetPermission(u.ID, c.ID, PermissionWrite); err != nil {
		t.Fatalf("SetPermission(write): %v", err)
	}
	if level, _ := s.GetPermission(u.ID, c.ID); level != PermissionWrite {
		t.Fatalf("after upgrading to write, GetPermission = %q", level)
	}

	perms, err := s.ListPermissionsForUser(u.ID)
	if err != nil {
		t.Fatalf("ListPermissionsForUser: %v", err)
	}
	if len(perms) != 1 {
		t.Fatalf("ListPermissionsForUser returned %d entries, want 1", len(perms))
	}
}

func TestSetPermissionNoneRevokesGrant(t *testing.T) {
	s := newTestStore(t)
	u := mustCreateUser(t, s, "alice")
	c := mustCreateConnection(t, s, "conn1")

	if err := s.SetPermission(u.ID, c.ID, PermissionWrite); err != nil {
		t.Fatalf("SetPermission(write): %v", err)
	}
	if err := s.SetPermission(u.ID, c.ID, PermissionNone); err != nil {
		t.Fatalf("SetPermission(none): %v", err)
	}

	level, err := s.GetPermission(u.ID, c.ID)
	if err != nil {
		t.Fatalf("GetPermission: %v", err)
	}
	if level != PermissionNone {
		t.Errorf("GetPermission after revoking = %q, want %q", level, PermissionNone)
	}

	perms, err := s.ListPermissionsForUser(u.ID)
	if err != nil {
		t.Fatalf("ListPermissionsForUser: %v", err)
	}
	if len(perms) != 0 {
		t.Errorf("ListPermissionsForUser after revoking returned %d entries, want 0", len(perms))
	}
}

func TestListUsersReturnsEmptySliceNotNil(t *testing.T) {
	s := newTestStore(t)
	users, err := s.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if users == nil {
		t.Error("ListUsers on an empty store returned nil, want an empty (non-nil) slice so it serializes as [] not null")
	}
}

func TestAuditLogRoundtripsExecutedAt(t *testing.T) {
	s := newTestStore(t)
	u := mustCreateUser(t, s, "alice")
	c := mustCreateConnection(t, s, "conn1")

	if err := s.InsertAuditLogEntry(AuditLogEntry{
		UserID: u.ID, ConnectionID: c.ID, Statement: "DELETE FROM t", Allowed: false, ErrorMessage: "denied",
	}); err != nil {
		t.Fatalf("InsertAuditLogEntry: %v", err)
	}

	entries, err := s.ListAuditLog(10)
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListAuditLog returned %d entries, want 1", len(entries))
	}
	if entries[0].ExecutedAt.IsZero() {
		t.Error("ExecutedAt is zero-valued; the executed_at column was not parsed into the entry")
	}
}

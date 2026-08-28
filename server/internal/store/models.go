package store

import "time"

// DBKind identifies which wire protocol/driver a Connection uses.
type DBKind string

const (
	DBKindMSSQL    DBKind = "mssql"
	DBKindMySQL    DBKind = "mysql"
	DBKindPostgres DBKind = "postgres"
	DBKindOracle   DBKind = "oracle"
)

// PermissionLevel is the access grade a User has on a Connection.
type PermissionLevel string

const (
	PermissionNone  PermissionLevel = "none"
	PermissionRead  PermissionLevel = "read"
	PermissionWrite PermissionLevel = "write"
)

// User is an app login account. It carries no DB access by itself —
// access comes only through a Permission on a Connection.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	IsAdmin      bool      `json:"isAdmin"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Connection is one registered DB server instance (host+port+kind+shared
// credential). It is the unit Permissions are granted against — not a
// specific database/catalog, schema, or table on that server.
type Connection struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind DBKind `json:"kind"`
	Host string `json:"host"`
	Port int    `json:"port"`
	// ServiceName is the Oracle service name/SID to connect to. Oracle's
	// CDB/PDB architecture requires one at connect time (unlike
	// MySQL/Postgres/MSSQL, which can connect at the instance level and
	// have the query name a database instead), so it is required for
	// DBKindOracle connections and unused otherwise.
	ServiceName string    `json:"serviceName"`
	Username    string    `json:"username"`
	Password    string    `json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Permission is the access grade a specific User has on a specific Connection.
type Permission struct {
	UserID       int64           `json:"userId"`
	ConnectionID int64           `json:"connectionId"`
	Level        PermissionLevel `json:"level"`
}

// AuditLogEntry records an attempted Write Query — including attempts
// denied for lack of permission. Read-only (SELECT) executions are not
// recorded.
type AuditLogEntry struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"userId"`
	ConnectionID int64     `json:"connectionId"`
	Statement    string    `json:"statement"`
	Allowed      bool      `json:"allowed"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
	ExecutedAt   time.Time `json:"executedAt"`
}

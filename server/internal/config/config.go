package config

import (
	"errors"
	"os"
)

type Config struct {
	ListenAddr             string
	SQLitePath             string
	JWTSecret              []byte
	BootstrapAdminUser     string
	BootstrapAdminPassword string
}

// Load reads the server configuration from the environment. DBTOOL_JWT_SECRET
// has no fallback — a shared default would let anyone forge a valid session
// token against any self-hosted instance still running it — so its absence
// is a fatal error rather than a silently insecure default.
func Load() (Config, error) {
	secret := os.Getenv("DBTOOL_JWT_SECRET")
	if secret == "" {
		return Config{}, errors.New("DBTOOL_JWT_SECRET environment variable must be set")
	}
	return Config{
		ListenAddr:             envOr("DBTOOL_LISTEN_ADDR", ":8080"),
		SQLitePath:             envOr("DBTOOL_SQLITE_PATH", "dbtool.sqlite"),
		JWTSecret:              []byte(secret),
		BootstrapAdminUser:     envOr("DBTOOL_BOOTSTRAP_ADMIN_USER", ""),
		BootstrapAdminPassword: envOr("DBTOOL_BOOTSTRAP_ADMIN_PASSWORD", ""),
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

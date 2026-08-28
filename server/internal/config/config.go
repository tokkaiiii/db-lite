package config

import (
	"errors"
	"fmt"
	"os"
)

type Config struct {
	ListenAddr             string
	SQLitePath             string
	JWTSecret              []byte
	BootstrapAdminUser     string
	BootstrapAdminPassword string
}

// Load reads the server configuration from the environment, first loading
// a .env file (path from DBTOOL_ENV_FILE, default ".env" in the working
// directory) if one is present — see loadDotEnv. DBTOOL_JWT_SECRET has no
// fallback — a shared default would let anyone forge a valid session token
// against any self-hosted instance still running it — so its absence is a
// fatal error rather than a silently insecure default.
func Load() (Config, error) {
	envFile := os.Getenv("DBTOOL_ENV_FILE")
	if envFile == "" {
		envFile = ".env"
	}
	if err := loadDotEnv(envFile); err != nil {
		return Config{}, fmt.Errorf("load %s: %w", envFile, err)
	}

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

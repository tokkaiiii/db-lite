package config

import "os"

type Config struct {
	ListenAddr             string
	SQLitePath             string
	JWTSecret              []byte
	BootstrapAdminUser     string
	BootstrapAdminPassword string
}

func Load() Config {
	return Config{
		ListenAddr:             envOr("DBTOOL_LISTEN_ADDR", ":8080"),
		SQLitePath:             envOr("DBTOOL_SQLITE_PATH", "dbtool.sqlite"),
		JWTSecret:              []byte(envOr("DBTOOL_JWT_SECRET", "dev-secret-change-me")),
		BootstrapAdminUser:     envOr("DBTOOL_BOOTSTRAP_ADMIN_USER", ""),
		BootstrapAdminPassword: envOr("DBTOOL_BOOTSTRAP_ADMIN_PASSWORD", ""),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

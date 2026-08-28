package main

import (
	"log"
	"net/http"
	"time"

	"dbtool/server/internal/auth"
	"dbtool/server/internal/config"
	"dbtool/server/internal/httpapi"
	"dbtool/server/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	s, err := store.Open(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	if err := bootstrapAdmin(s, cfg); err != nil {
		log.Fatalf("failed to bootstrap admin: %v", err)
	}

	tokens := auth.NewTokenIssuer(cfg.JWTSecret, 12*time.Hour)
	server := httpapi.NewServer(s, tokens)

	log.Printf("listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, server.Router()); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// bootstrapAdmin creates the first Admin account from DBTOOL_BOOTSTRAP_ADMIN_USER
// / DBTOOL_BOOTSTRAP_ADMIN_PASSWORD, but only when the store has no users at
// all yet — otherwise there would be no way to create the very first
// account through the admin-only /api/admin/users endpoint.
func bootstrapAdmin(s *store.Store, cfg config.Config) error {
	if cfg.BootstrapAdminUser == "" || cfg.BootstrapAdminPassword == "" {
		return nil
	}
	existing, err := s.ListUsers()
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	hash, err := auth.HashPassword(cfg.BootstrapAdminPassword)
	if err != nil {
		return err
	}
	_, err = s.CreateUser(cfg.BootstrapAdminUser, hash, true)
	if err != nil {
		return err
	}
	log.Printf("bootstrapped admin user %q", cfg.BootstrapAdminUser)
	return nil
}

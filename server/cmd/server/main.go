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
	cfg := config.Load()

	s, err := store.Open(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	tokens := auth.NewTokenIssuer(cfg.JWTSecret, 12*time.Hour)
	server := httpapi.NewServer(s, tokens)

	log.Printf("listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, server.Router()); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

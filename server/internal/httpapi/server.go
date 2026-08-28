// Package httpapi wires the REST API: routing, auth middleware, and
// handlers for login, Connection/Permission administration, and query
// execution.
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"dbtool/server/internal/auth"
	"dbtool/server/internal/store"
)

type Server struct {
	store  *store.Store
	tokens *auth.TokenIssuer
}

func NewServer(s *store.Store, tokens *auth.TokenIssuer) *Server {
	return &Server{store: s, tokens: tokens}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Post("/api/login", s.handleLogin)

	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)

		r.Get("/api/connections", s.handleListConnections)
		r.Post("/api/connections/{connectionID}/query", s.handleExecuteQuery)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAdmin)

			r.Post("/api/admin/users", s.handleCreateUser)
			r.Get("/api/admin/users", s.handleListUsers)

			r.Post("/api/admin/connections", s.handleCreateConnection)
			r.Delete("/api/admin/connections/{connectionID}", s.handleDeleteConnection)

			r.Put("/api/admin/permissions", s.handleSetPermission)

			r.Get("/api/admin/audit-log", s.handleListAuditLog)
		})
	})

	return r
}

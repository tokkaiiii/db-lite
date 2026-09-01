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
		r.Get("/api/connections/{connectionID}/catalogs", s.handleListCatalogs)
		r.Get("/api/connections/{connectionID}/schema", s.handleDescribeSchema)
		r.Post("/api/connections/{connectionID}/query", s.handleExecuteQuery)
		r.Post("/api/connections/{connectionID}/cell", s.handleFetchCell)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAdmin)

			r.Post("/api/admin/users", s.handleCreateUser)
			r.Get("/api/admin/users", s.handleListUsers)
			r.Put("/api/admin/users/{userID}/password", s.handleResetUserPassword)
			r.Delete("/api/admin/users/{userID}", s.handleDeleteUser)

			r.Get("/api/admin/connections", s.handleAdminListConnections)
			r.Post("/api/admin/connections", s.handleCreateConnection)
			r.Put("/api/admin/connections/{connectionID}", s.handleUpdateConnection)
			r.Delete("/api/admin/connections/{connectionID}", s.handleDeleteConnection)

			r.Get("/api/admin/permissions", s.handleListAllPermissions)
			r.Put("/api/admin/permissions", s.handleSetPermission)

			r.Get("/api/admin/audit-log", s.handleListAuditLog)
		})
	})

	r.NotFound(spaHandler(staticFS()))

	return r
}

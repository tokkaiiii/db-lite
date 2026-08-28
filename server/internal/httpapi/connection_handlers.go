package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"dbtool/server/internal/store"
)

type createConnectionRequest struct {
	Name     string       `json:"name"`
	Kind     store.DBKind `json:"kind"`
	Host     string       `json:"host"`
	Port     int          `json:"port"`
	Username string       `json:"username"`
	Password string       `json:"password"`
}

func (s *Server) handleCreateConnection(w http.ResponseWriter, r *http.Request) {
	var req createConnectionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	conn, err := s.store.CreateConnection(store.Connection{
		Name:     req.Name,
		Kind:     req.Kind,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		writeError(w, http.StatusConflict, "connection name already exists")
		return
	}
	writeJSON(w, http.StatusCreated, conn)
}

func (s *Server) handleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "connectionID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection id")
		return
	}
	if err := s.store.DeleteConnection(id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete connection")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListConnections returns only the Connections the calling User has
// a non-none Permission on, alongside that level — Admins included, since
// having the Admin role does not itself grant DB access.
func (s *Server) handleListConnections(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())

	perms, err := s.store.ListPermissionsForUser(claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list permissions")
		return
	}

	type connectionView struct {
		store.Connection
		Level store.PermissionLevel `json:"level"`
	}

	views := make([]connectionView, 0, len(perms))
	for _, p := range perms {
		conn, err := s.store.GetConnection(p.ConnectionID)
		if err != nil {
			continue
		}
		views = append(views, connectionView{Connection: *conn, Level: p.Level})
	}
	writeJSON(w, http.StatusOK, views)
}

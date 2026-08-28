package httpapi

import (
	"net/http"

	"dbtool/server/internal/store"
)

type setPermissionRequest struct {
	UserID       int64                 `json:"userId"`
	ConnectionID int64                 `json:"connectionId"`
	Level        store.PermissionLevel `json:"level"`
}

func (s *Server) handleSetPermission(w http.ResponseWriter, r *http.Request) {
	var req setPermissionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch req.Level {
	case store.PermissionNone, store.PermissionRead, store.PermissionWrite:
	default:
		writeError(w, http.StatusBadRequest, "level must be none, read, or write")
		return
	}
	if err := s.store.SetPermission(req.UserID, req.ConnectionID, req.Level); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set permission")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

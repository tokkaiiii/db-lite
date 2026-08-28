package httpapi

import "net/http"

func (s *Server) handleListAuditLog(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ListAuditLog(500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list audit log")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

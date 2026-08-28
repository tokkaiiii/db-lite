package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"dbtool/server/internal/dbconn"
	"dbtool/server/internal/query"
	"dbtool/server/internal/store"
)

type executeQueryRequest struct {
	Statement string `json:"statement"`
}

// handleExecuteQuery enforces Permission before running the statement, and
// records a Write Query attempt to the Audit Log Entry table regardless of
// whether it was allowed — a denied write attempt is itself a signal worth
// keeping.
func (s *Server) handleExecuteQuery(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())

	connectionID, err := strconv.ParseInt(chi.URLParam(r, "connectionID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection id")
		return
	}

	var req executeQueryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	level, err := s.store.GetPermission(claims.UserID, connectionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check permission")
		return
	}
	if level == store.PermissionNone {
		writeError(w, http.StatusForbidden, "no access to this connection")
		return
	}

	isWrite := query.IsWrite(req.Statement)
	if isWrite && level != store.PermissionWrite {
		s.recordAudit(claims.UserID, connectionID, req.Statement, false, "write attempted without write permission")
		writeError(w, http.StatusForbidden, "read-only access: write statements are not allowed")
		return
	}

	conn, err := s.store.GetConnection(connectionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}

	db, err := dbconn.Open(*conn)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to open target connection")
		return
	}
	defer db.Close()

	result, err := query.Execute(db, req.Statement)
	if isWrite {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		s.recordAudit(claims.UserID, connectionID, req.Statement, err == nil, errMsg)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) recordAudit(userID, connectionID int64, statement string, allowed bool, errMsg string) {
	// Best-effort: a logging failure must not block the query response.
	_ = s.store.InsertAuditLogEntry(store.AuditLogEntry{
		UserID:       userID,
		ConnectionID: connectionID,
		Statement:    statement,
		Allowed:      allowed,
		ErrorMessage: errMsg,
	})
}

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
	Catalog   string `json:"catalog"`
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

	level, ok := s.requireConnectionAccess(w, claims.UserID, connectionID)
	if !ok {
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

	db, err := dbconn.Open(*conn, req.Catalog)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to open target connection")
		return
	}
	defer db.Close()

	result, err := query.Execute(db, conn.Kind, req.Statement)
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

// requireConnectionAccess checks that userID has at least read Permission on
// connectionID, writing the appropriate error response and returning
// ok=false if not (or if the check itself failed).
func (s *Server) requireConnectionAccess(w http.ResponseWriter, userID, connectionID int64) (level store.PermissionLevel, ok bool) {
	level, err := s.store.GetPermission(userID, connectionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check permission")
		return "", false
	}
	if level == store.PermissionNone {
		writeError(w, http.StatusForbidden, "no access to this connection")
		return "", false
	}
	return level, true
}

// handleListCatalogs returns the Catalogs (individual databases) available
// on the target Connection's server instance, so the client can offer a
// picker before running a query. Oracle Connections always get an empty
// list back — see dbconn.ListCatalogs.
func (s *Server) handleListCatalogs(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())

	connectionID, err := strconv.ParseInt(chi.URLParam(r, "connectionID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection id")
		return
	}

	if _, ok := s.requireConnectionAccess(w, claims.UserID, connectionID); !ok {
		return
	}

	conn, err := s.store.GetConnection(connectionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}

	// Oracle has no Catalog concept (its service name already fixes the
	// target PDB) — skip opening a connection just to learn that.
	if conn.Kind == store.DBKindOracle {
		writeJSON(w, http.StatusOK, map[string][]string{"catalogs": {}})
		return
	}

	db, err := dbconn.Open(*conn, "")
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to open target connection")
		return
	}
	defer db.Close()

	catalogs, err := dbconn.ListCatalogs(db, conn.Kind)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string][]string{"catalogs": catalogs})
}

// handleDescribeSchema returns every table's columns in the target
// Connection's default schema within catalog (Oracle ignores catalog and
// returns the connecting user's own tables), for the client to feed into
// its SQL editor's autocomplete.
func (s *Server) handleDescribeSchema(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())

	connectionID, err := strconv.ParseInt(chi.URLParam(r, "connectionID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid connection id")
		return
	}

	if _, ok := s.requireConnectionAccess(w, claims.UserID, connectionID); !ok {
		return
	}

	conn, err := s.store.GetConnection(connectionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}

	db, err := dbconn.Open(*conn, r.URL.Query().Get("catalog"))
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to open target connection")
		return
	}
	defer db.Close()

	schema, err := dbconn.DescribeSchema(db, conn.Kind)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]map[string][]string{"schema": schema})
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

package dbconn

import (
	"strings"
	"testing"

	"dbtool/server/internal/store"
)

func testConn(kind store.DBKind) store.Connection {
	return store.Connection{
		Name: "test", Kind: kind, Host: "dbhost", Port: 1234,
		Username: "u", Password: "p", ServiceName: "SVC",
	}
}

func TestDriverAndDSN_MySQL(t *testing.T) {
	_, dsn, err := driverAndDSN(testConn(store.DBKindMySQL), "")
	if err != nil {
		t.Fatalf("driverAndDSN: %v", err)
	}
	if !strings.HasSuffix(dsn, "@tcp(dbhost:1234)/") {
		t.Errorf("empty catalog: dsn = %q, want it to end with an empty database segment", dsn)
	}

	_, dsn, err = driverAndDSN(testConn(store.DBKindMySQL), "mydb")
	if err != nil {
		t.Fatalf("driverAndDSN: %v", err)
	}
	if !strings.HasSuffix(dsn, "/mydb") {
		t.Errorf("catalog=mydb: dsn = %q, want it to end with /mydb", dsn)
	}
}

func TestDriverAndDSN_MSSQL(t *testing.T) {
	_, dsn, err := driverAndDSN(testConn(store.DBKindMSSQL), "")
	if err != nil {
		t.Fatalf("driverAndDSN: %v", err)
	}
	if strings.Contains(dsn, "database=") {
		t.Errorf("empty catalog: dsn = %q, should not set a database param", dsn)
	}

	_, dsn, err = driverAndDSN(testConn(store.DBKindMSSQL), "mydb")
	if err != nil {
		t.Fatalf("driverAndDSN: %v", err)
	}
	if !strings.Contains(dsn, "?database=mydb") {
		t.Errorf("catalog=mydb: dsn = %q, want it to contain ?database=mydb", dsn)
	}
}

func TestDriverAndDSN_Postgres(t *testing.T) {
	_, dsn, err := driverAndDSN(testConn(store.DBKindPostgres), "")
	if err != nil {
		t.Fatalf("driverAndDSN: %v", err)
	}
	if !strings.HasSuffix(dsn, "/"+postgresDefaultCatalog) {
		t.Errorf("empty catalog: dsn = %q, want it to fall back to /%s", dsn, postgresDefaultCatalog)
	}

	_, dsn, err = driverAndDSN(testConn(store.DBKindPostgres), "mydb")
	if err != nil {
		t.Fatalf("driverAndDSN: %v", err)
	}
	if !strings.HasSuffix(dsn, "/mydb") {
		t.Errorf("catalog=mydb: dsn = %q, want it to end with /mydb", dsn)
	}
}

func TestDriverAndDSN_OracleRequiresServiceName(t *testing.T) {
	c := testConn(store.DBKindOracle)
	c.ServiceName = ""
	if _, _, err := driverAndDSN(c, ""); err == nil {
		t.Error("expected an error for an Oracle connection with no service name, got nil")
	}

	c.ServiceName = "SVC"
	_, dsn, err := driverAndDSN(c, "ignored-catalog")
	if err != nil {
		t.Fatalf("driverAndDSN: %v", err)
	}
	if !strings.HasSuffix(dsn, "/SVC") {
		t.Errorf("dsn = %q, want it to end with the service name /SVC regardless of catalog", dsn)
	}
}

func TestDriverAndDSN_UnsupportedKind(t *testing.T) {
	if _, _, err := driverAndDSN(testConn("nosql"), ""); err == nil {
		t.Error("expected an error for an unsupported DB kind, got nil")
	}
}

package dbconn

import (
	"net/url"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"

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

// TestDriverAndDSN_PasswordWithSpecialCharacters reproduces a real bug: a
// password containing '/' (or '@', '?', '#', ...) broke DSN construction
// when it was built by interpolating the raw password into a URL string —
// the '/' was read as the start of the URL path before the real '@'
// separating credentials from the host was ever reached, so the host was
// lost and the driver saw a nonsense "host" made of the username and part
// of the password instead. Each DSN must round-trip back to the original
// host/port regardless of what the password contains.
func TestDriverAndDSN_PasswordWithSpecialCharacters(t *testing.T) {
	const nastyPassword = `1qw2/rest@of#pass?word`

	t.Run("mssql", func(t *testing.T) {
		c := testConn(store.DBKindMSSQL)
		c.Password = nastyPassword
		_, dsn, err := driverAndDSN(c, "")
		if err != nil {
			t.Fatalf("driverAndDSN: %v", err)
		}
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("resulting dsn %q does not even parse as a URL: %v", dsn, err)
		}
		if u.Host != "dbhost:1234" {
			t.Errorf("host = %q, want dbhost:1234 (dsn: %q)", u.Host, dsn)
		}
		if pw, _ := u.User.Password(); pw != nastyPassword {
			t.Errorf("password round-tripped as %q, want %q", pw, nastyPassword)
		}
	})

	t.Run("postgres", func(t *testing.T) {
		c := testConn(store.DBKindPostgres)
		c.Password = nastyPassword
		_, dsn, err := driverAndDSN(c, "mydb")
		if err != nil {
			t.Fatalf("driverAndDSN: %v", err)
		}
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("resulting dsn %q does not even parse as a URL: %v", dsn, err)
		}
		if u.Host != "dbhost:1234" {
			t.Errorf("host = %q, want dbhost:1234 (dsn: %q)", u.Host, dsn)
		}
		if pw, _ := u.User.Password(); pw != nastyPassword {
			t.Errorf("password round-tripped as %q, want %q", pw, nastyPassword)
		}
	})

	t.Run("oracle", func(t *testing.T) {
		c := testConn(store.DBKindOracle)
		c.Password = nastyPassword
		_, dsn, err := driverAndDSN(c, "")
		if err != nil {
			t.Fatalf("driverAndDSN: %v", err)
		}
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("resulting dsn %q does not even parse as a URL: %v", dsn, err)
		}
		if u.Host != "dbhost:1234" {
			t.Errorf("host = %q, want dbhost:1234 (dsn: %q)", u.Host, dsn)
		}
	})

	t.Run("mysql", func(t *testing.T) {
		c := testConn(store.DBKindMySQL)
		c.Password = nastyPassword
		_, dsn, err := driverAndDSN(c, "mydb")
		if err != nil {
			t.Fatalf("driverAndDSN: %v", err)
		}
		cfg, err := mysqldriver.ParseDSN(dsn)
		if err != nil {
			t.Fatalf("resulting dsn %q does not even parse: %v", dsn, err)
		}
		if cfg.Addr != "dbhost:1234" {
			t.Errorf("addr = %q, want dbhost:1234 (dsn: %q)", cfg.Addr, dsn)
		}
		if cfg.Passwd != nastyPassword {
			t.Errorf("password round-tripped as %q, want %q", cfg.Passwd, nastyPassword)
		}
	})
}

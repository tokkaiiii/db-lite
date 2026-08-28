package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempEnvFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadDotEnvSetsUnsetVars(t *testing.T) {
	path := writeTempEnvFile(t, "DBTOOL_TEST_A=hello\nDBTOOL_TEST_B=\"quoted value\"\n")
	t.Cleanup(func() {
		os.Unsetenv("DBTOOL_TEST_A")
		os.Unsetenv("DBTOOL_TEST_B")
	})

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv("DBTOOL_TEST_A"); got != "hello" {
		t.Errorf("DBTOOL_TEST_A = %q, want %q", got, "hello")
	}
	if got := os.Getenv("DBTOOL_TEST_B"); got != "quoted value" {
		t.Errorf("DBTOOL_TEST_B = %q, want %q (quotes should be stripped)", got, "quoted value")
	}
}

func TestLoadDotEnvDoesNotOverrideRealEnv(t *testing.T) {
	t.Setenv("DBTOOL_TEST_C", "from-real-env")
	path := writeTempEnvFile(t, "DBTOOL_TEST_C=from-dotenv\n")

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv("DBTOOL_TEST_C"); got != "from-real-env" {
		t.Errorf("DBTOOL_TEST_C = %q, want %q (real env must win over .env)", got, "from-real-env")
	}
}

func TestLoadDotEnvIgnoresCommentsAndBlankLines(t *testing.T) {
	path := writeTempEnvFile(t, "# a comment\n\nDBTOOL_TEST_D=value\n")
	t.Cleanup(func() { os.Unsetenv("DBTOOL_TEST_D") })

	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv("DBTOOL_TEST_D"); got != "value" {
		t.Errorf("DBTOOL_TEST_D = %q, want %q", got, "value")
	}
}

func TestLoadDotEnvMissingFileIsNotAnError(t *testing.T) {
	if err := loadDotEnv(filepath.Join(t.TempDir(), "does-not-exist.env")); err != nil {
		t.Errorf("loadDotEnv on a missing file returned %v, want nil", err)
	}
}

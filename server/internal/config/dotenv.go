package config

import (
	"bufio"
	"os"
	"strings"
)

// loadDotEnv reads KEY=value pairs from a .env file, if one exists, and
// applies them via os.Setenv — but only for keys not already set in the
// real environment, which always wins. This exists purely so operators
// don't have to fight each shell's own syntax for setting env vars (bash
// export, PowerShell $env:, cmd set) just to configure DBTOOL_JWT_SECRET
// and friends — see README.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)

		if _, alreadySet := os.LookupEnv(key); alreadySet {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}

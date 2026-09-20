package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadDotEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	body := "# comment\nPLAIN=value\nSINGLE='value$part'\nDOUBLE=\"value with spaces\"\nEMPTY=\nEQUAL=a=b\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	readDotEnv(path, got)
	want := map[string]string{
		"PLAIN":  "value",
		"SINGLE": "value$part",
		"DOUBLE": "value with spaces",
		"EMPTY":  "",
		"EQUAL":  "a=b",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s=%q, want %q", key, got[key], value)
		}
	}
}

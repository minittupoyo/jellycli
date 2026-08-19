package debuglog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerWritesPrivateRedactedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "debug.log")
	logger, err := Open(path, "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	logger.Error("play", errors.New("request api_key=query-secret Token=secret-token"))
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "query-secret") || strings.Contains(text, "secret-token") || !strings.Contains(text, "[REDACTED]") || !strings.Contains(text, `"operation":"play"`) {
		t.Fatalf("log = %s", text)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, err = %v", info.Mode().Perm(), err)
	}
}

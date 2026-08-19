package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	return NewStore(Paths{
		Settings: filepath.Join(root, "config", "jellycli", "config.json"),
		State:    filepath.Join(root, "state", "jellycli", "state.json"),
	})
}

func TestSettingsValidationAndNormalization(t *testing.T) {
	s := Settings{ServerURL: "https://media.example.test/jellyfin///"}
	got, err := s.NormalizedServerURL()
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://media.example.test/jellyfin"; got != want {
		t.Fatalf("NormalizedServerURL() = %q, want %q", got, want)
	}

	for _, invalid := range []string{"", "media.example.test", "ftp://media.example.test", "https://u:p@media.example.test", "https://media.example.test?q=token"} {
		if err := (Settings{ServerURL: invalid}).Validate(); err == nil {
			t.Errorf("Validate(%q) error = nil, want error", invalid)
		}
	}
	if err := (Settings{ServerURL: "https://media.example.test", LogFile: "relative.log"}).Validate(); err == nil {
		t.Fatal("relative log file validation error = nil")
	}
}

func TestSaveAndLoadSettings(t *testing.T) {
	store := testStore(t)
	want := Settings{ServerURL: "https://media.example.test", Debug: true, LogFile: "/tmp/jellycli.log"}
	if err := store.SaveSettings(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("LoadSettings() = %#v, want %#v", got, want)
	}
	assertPrivatePermissions(t, store.Paths().Settings)
}

func TestEnsureDeviceIDIsStable(t *testing.T) {
	store := testStore(t)
	first, err := store.EnsureDeviceID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.EnsureDeviceID()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("device ID changed: %q != %q", first, second)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(first) {
		t.Fatalf("device ID %q is not a version 4 UUID", first)
	}
	assertPrivatePermissions(t, store.Paths().State)
}

func TestSaveAndClearAuthPreservesDeviceID(t *testing.T) {
	store := testStore(t)
	deviceID, err := store.EnsureDeviceID()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAuth(Auth{AccessToken: "secret-token", UserID: "user-id"}); err != nil {
		t.Fatal(err)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Auth == nil || state.Auth.AccessToken != "secret-token" {
		t.Fatalf("LoadState() auth = %#v", state.Auth)
	}
	if err := store.ClearAuth(); err != nil {
		t.Fatal(err)
	}
	state, err = store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Auth != nil {
		t.Fatalf("auth was not cleared: %#v", state.Auth)
	}
	if state.DeviceID != deviceID {
		t.Fatalf("device ID = %q, want %q", state.DeviceID, deviceID)
	}
}

func TestPersistentTypesCannotSerializePassword(t *testing.T) {
	data, err := json.Marshal(struct {
		Settings Settings `json:"settings"`
		State    State    `json:"state"`
	}{
		Settings: Settings{ServerURL: "https://media.example.test"},
		State: State{DeviceID: "device", Auth: &Auth{
			AccessToken: "token",
			UserID:      "user",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "password") {
		t.Fatalf("persistent JSON contains a password field: %s", data)
	}
}

func TestLoadRejectsOversizedFile(t *testing.T) {
	store := testStore(t)
	if err := os.MkdirAll(filepath.Dir(store.Paths().Settings), 0o700); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, maxFileSize+1)
	if err := os.WriteFile(store.Paths().Settings, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadSettings(); err == nil {
		t.Fatal("LoadSettings() error = nil, want oversized-file error")
	}
}

func assertPrivatePermissions(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode for %s = %o, want 600", path, got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("mode for %s = %o, want 700", filepath.Dir(path), got)
	}
}

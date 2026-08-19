// Package config persists jellycli preferences and authentication state using
// XDG base directories.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxFileSize = 1 << 20

// Settings contains non-secret user preferences.
type Settings struct {
	ServerURL string `json:"server_url"`
	Debug     bool   `json:"debug,omitempty"`
	LogFile   string `json:"log_file,omitempty"`
}

// Validate checks settings without making a network request.
func (s Settings) Validate() error {
	if strings.TrimSpace(s.ServerURL) == "" {
		return errors.New("server URL is required")
	}
	u, err := url.Parse(s.ServerURL)
	if err != nil {
		return fmt.Errorf("parse server URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("server URL must be an absolute http or https URL")
	}
	if u.User != nil {
		return errors.New("server URL must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return errors.New("server URL must not contain a query or fragment")
	}
	return nil
}

// NormalizedServerURL returns a validated URL without trailing slashes.
func (s Settings) NormalizedServerURL() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	return strings.TrimRight(s.ServerURL, "/"), nil
}

// Auth contains credentials returned by Jellyfin. Passwords deliberately have
// no representation in persistent configuration.
type Auth struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"user_id"`
}

func (a Auth) valid() bool {
	return a.AccessToken != "" && a.UserID != ""
}

// State contains private, machine-local state.
type State struct {
	DeviceID string `json:"device_id"`
	Auth     *Auth  `json:"auth,omitempty"`
}

// Store reads and atomically writes configuration files.
type Store struct {
	paths Paths
}

func NewStore(paths Paths) *Store {
	return &Store{paths: paths}
}

func (s *Store) Paths() Paths { return s.paths }

func (s *Store) LoadSettings() (Settings, error) {
	var settings Settings
	if err := readJSON(s.paths.Settings, &settings); err != nil {
		return Settings{}, fmt.Errorf("load settings: %w", err)
	}
	return settings, nil
}

func (s *Store) SaveSettings(settings Settings) error {
	if err := settings.Validate(); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	if err := writeJSONAtomic(s.paths.Settings, settings); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}

func (s *Store) LoadState() (State, error) {
	var state State
	if err := readJSON(s.paths.State, &state); err != nil {
		return State{}, fmt.Errorf("load state: %w", err)
	}
	if state.Auth != nil && !state.Auth.valid() {
		return State{}, errors.New("load state: authentication state is incomplete")
	}
	return state, nil
}

func (s *Store) SaveState(state State) error {
	if state.DeviceID == "" {
		return errors.New("save state: device ID is required")
	}
	if state.Auth != nil && !state.Auth.valid() {
		return errors.New("save state: authentication state is incomplete")
	}
	if err := writeJSONAtomic(s.paths.State, state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}

// EnsureDeviceID returns the existing stable ID or generates and saves one.
func (s *Store) EnsureDeviceID() (string, error) {
	state, err := s.LoadState()
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if state.DeviceID != "" {
		return state.DeviceID, nil
	}

	deviceID, err := newDeviceID()
	if err != nil {
		return "", fmt.Errorf("generate device ID: %w", err)
	}
	state.DeviceID = deviceID
	if err := s.SaveState(state); err != nil {
		return "", err
	}
	return deviceID, nil
}

// SaveAuth stores a token while preserving the stable device ID.
func (s *Store) SaveAuth(auth Auth) error {
	state, err := s.LoadState()
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if state.DeviceID == "" {
		state.DeviceID, err = newDeviceID()
		if err != nil {
			return fmt.Errorf("generate device ID: %w", err)
		}
	}
	state.Auth = &auth
	return s.SaveState(state)
}

// ClearAuth logs out locally while preserving the stable device identity.
func (s *Store) ClearAuth() error {
	state, err := s.LoadState()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	state.Auth = nil
	return s.SaveState(state)
}

func newDeviceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// RFC 9562 UUID version 4 and RFC 4122 variant bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(b[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}

func readJSON(path string, dst any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() > maxFileSize {
		return fmt.Errorf("decode %s: file exceeds %d bytes", path, maxFileSize)
	}

	decoder := json.NewDecoder(io.LimitReader(f, maxFileSize))
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode %s: multiple JSON values", path)
		}
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSONAtomic(path string, value any) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".jellycli-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure temporary file: %w", err)
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(value); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encode temporary file: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	if err = os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure %s: %w", path, err)
	}

	d, openErr := os.Open(dir)
	if openErr != nil {
		return fmt.Errorf("open directory for sync: %w", openErr)
	}
	defer d.Close()
	if syncErr := d.Sync(); syncErr != nil {
		return fmt.Errorf("sync directory: %w", syncErr)
	}
	return nil
}

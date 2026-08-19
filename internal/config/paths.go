package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const appDir = "jellycli"

// Paths contains the persistent files used by jellycli. Keeping path discovery
// separate makes XDG behavior testable and avoids spreading OS details.
type Paths struct {
	Settings string
	State    string
	Log      string
}

// ResolvePaths resolves XDG paths using getenv and homeDir. Empty XDG values
// use the specification's conventional defaults below the user's home.
func ResolvePaths(getenv func(string) string, homeDir string) (Paths, error) {
	if homeDir == "" {
		return Paths{}, fmt.Errorf("resolve config paths: user home directory is empty")
	}

	configHome := getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(homeDir, ".config")
	} else if !filepath.IsAbs(configHome) {
		return Paths{}, fmt.Errorf("resolve config paths: XDG_CONFIG_HOME must be absolute")
	}

	stateHome := getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(homeDir, ".local", "state")
	} else if !filepath.IsAbs(stateHome) {
		return Paths{}, fmt.Errorf("resolve config paths: XDG_STATE_HOME must be absolute")
	}

	return Paths{
		Settings: filepath.Join(configHome, appDir, "config.json"),
		State:    filepath.Join(stateHome, appDir, "state.json"),
		Log:      filepath.Join(stateHome, appDir, "debug.log"),
	}, nil
}

// DefaultPaths resolves paths from the current process environment.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve config paths: %w", err)
	}
	return ResolvePaths(os.Getenv, home)
}

package config

import (
	"path/filepath"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	env := map[string]string{
		"XDG_CONFIG_HOME": "/xdg/config",
		"XDG_STATE_HOME":  "/xdg/state",
	}
	paths, err := ResolvePaths(func(key string) string { return env[key] }, "/home/test")
	if err != nil {
		t.Fatal(err)
	}
	if want := "/xdg/config/jellycli/config.json"; paths.Settings != want {
		t.Fatalf("Settings = %q, want %q", paths.Settings, want)
	}
	if want := "/xdg/state/jellycli/state.json"; paths.State != want {
		t.Fatalf("State = %q, want %q", paths.State, want)
	}
}

func TestResolvePathsDefaults(t *testing.T) {
	paths, err := ResolvePaths(func(string) string { return "" }, "/home/test")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.FromSlash("/home/test/.config/jellycli/config.json"); paths.Settings != want {
		t.Fatalf("Settings = %q, want %q", paths.Settings, want)
	}
	if want := filepath.FromSlash("/home/test/.local/state/jellycli/state.json"); paths.State != want {
		t.Fatalf("State = %q, want %q", paths.State, want)
	}
}

func TestResolvePathsRejectsRelativeXDGPath(t *testing.T) {
	_, err := ResolvePaths(func(key string) string {
		if key == "XDG_CONFIG_HOME" {
			return "relative"
		}
		return ""
	}, "/home/test")
	if err == nil {
		t.Fatal("ResolvePaths() error = nil, want error")
	}
}

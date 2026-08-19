package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"jellycli/internal/config"
)

func TestLibrariesUsesSavedLogin(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") == "" {
			t.Error("Authorization header is empty")
		}
		switch r.URL.Path {
		case "/Users/Me":
			_, _ = w.Write([]byte(`{"Id":"user-id","Name":"alice"}`))
		case "/UserViews":
			if r.URL.Query().Get("userId") != "user-id" {
				t.Errorf("userId = %q", r.URL.Query().Get("userId"))
			}
			_, _ = w.Write([]byte(`{"Items":[{"Id":"movies","Name":"Movies","Type":"CollectionFolder","CollectionType":"movies"}],"TotalRecordCount":1,"StartIndex":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	store := config.NewStore(config.Paths{
		Settings: filepath.Join(root, "config", "config.json"),
		State:    filepath.Join(root, "state", "state.json"),
	})
	if err := store.SaveSettings(config.Settings{ServerURL: server.URL}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveState(config.State{
		DeviceID: "device-id",
		Auth:     &config.Auth{AccessToken: "token", UserID: "user-id"},
	}); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store, http.DefaultClient, "test device", "dev")
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.Libraries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "Movies" {
		t.Fatalf("Libraries() = %#v", items)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestLibrariesRequiresConfiguration(t *testing.T) {
	root := t.TempDir()
	store := config.NewStore(config.Paths{
		Settings: filepath.Join(root, "config.json"),
		State:    filepath.Join(root, "state.json"),
	})
	service, err := NewService(store, http.DefaultClient, "device", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Libraries(context.Background()); err == nil {
		t.Fatal("Libraries() error = nil, want configuration error")
	}
}

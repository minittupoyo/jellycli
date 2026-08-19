package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jellycli/internal/config"
	"jellycli/internal/player"
)

type fakePlayer struct{ media player.Media }

func (p *fakePlayer) Start(_ context.Context, media player.Media) (player.Session, error) {
	p.media = media
	return fakeSession{}, nil
}

type fakeSession struct{}

func (fakeSession) Pause(context.Context) error                     { return nil }
func (fakeSession) Resume(context.Context) error                    { return nil }
func (fakeSession) Seek(context.Context, time.Duration) error       { return nil }
func (fakeSession) Position(context.Context) (time.Duration, error) { return 12 * time.Second, nil }
func (fakeSession) Duration(context.Context) (time.Duration, error) { return time.Hour, nil }
func (fakeSession) Paused(context.Context) (bool, error)            { return false, nil }
func (fakeSession) Stop(context.Context) error                      { return nil }
func (fakeSession) Events() <-chan player.Event                     { return make(chan player.Event) }
func (fakeSession) Wait() error                                     { return nil }

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

func TestSearchAndPlayUseAuthenticatedApplicationFlow(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/Users/Me":
			_, _ = w.Write([]byte(`{"Id":"user-id"}`))
		case "/Items":
			if term := r.URL.Query().Get("searchTerm"); term != "" {
				if term != "pilot" || r.URL.Query().Get("includeItemTypes") != "Movie,Episode,Video" {
					t.Errorf("search query = %v", r.URL.Query())
				}
			}
			_, _ = w.Write([]byte(`{"Items":[{"Id":"item-id","Name":"Pilot","Type":"Movie","RunTimeTicks":36000000000,"UserData":{"PlaybackPositionTicks":100000000}}],"TotalRecordCount":1}`))
		case "/Items/item-id/PlaybackInfo":
			_, _ = w.Write([]byte(`{"PlaySessionId":"play-session","MediaSources":[{"Id":"source-id","SupportsDirectPlay":true}]}`))
		case "/Sessions/Playing", "/Sessions/Playing/Stopped":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	store := config.NewStore(config.Paths{Settings: filepath.Join(root, "config.json"), State: filepath.Join(root, "state.json")})
	if err := store.SaveSettings(config.Settings{ServerURL: server.URL}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveState(config.State{DeviceID: "device", Auth: &config.Auth{AccessToken: "token", UserID: "user-id"}}); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store, http.DefaultClient, "device", "dev")
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.Search(context.Background(), "pilot")
	if err != nil || len(items) != 1 {
		t.Fatalf("Search() = %#v, %v", items, err)
	}
	p := &fakePlayer{}
	if err := service.WithPlayer(p).Play(context.Background(), "item-id"); err != nil {
		t.Fatal(err)
	}
	if p.media.StartTime != 10*time.Second || !strings.Contains(p.media.URL, "/Videos/item-id/stream") || p.media.Headers["X-Emby-Token"] != "token" {
		t.Fatalf("media = %#v", p.media)
	}
	if !containsPath(paths, "/Sessions/Playing") || !containsPath(paths, "/Sessions/Playing/Stopped") {
		t.Fatalf("paths = %#v", paths)
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

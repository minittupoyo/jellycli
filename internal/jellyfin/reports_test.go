package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPlaybackReports(t *testing.T) {
	wantPaths := []string{"/Sessions/Playing", "/Sessions/Playing/Progress", "/Sessions/Playing/Stopped"}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != wantPaths[requests] {
			t.Errorf("request %d = %s %s", requests, r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["ItemId"] != "item-id" || body["MediaSourceId"] != "source-id" || body["PlaySessionId"] != "play-session" {
			t.Errorf("body = %#v", body)
		}
		if body["PositionTicks"] != float64(15_000_000) {
			t.Errorf("PositionTicks = %#v", body["PositionTicks"])
		}
		requests++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	c := newTestClient(t, server.URL).WithToken("token")
	state := PlaybackState{ItemID: "item-id", MediaSourceID: "source-id", PlaySessionID: "play-session", Position: 1500 * time.Millisecond, Paused: true, PlayMethod: "DirectPlay"}
	if err := c.ReportPlaybackStart(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if err := c.ReportPlaybackProgress(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	state.Failed = true
	if err := c.ReportPlaybackStopped(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want 3", requests)
	}
}

func TestPlaybackReportsValidateState(t *testing.T) {
	c := newTestClient(t, "https://media.example.test").WithToken("token")
	if err := c.ReportPlaybackStart(context.Background(), PlaybackState{ItemID: "item", PlayMethod: "invalid"}); err == nil {
		t.Fatal("invalid method error = nil")
	}
	if err := c.ReportPlaybackStopped(context.Background(), PlaybackState{PlayMethod: "DirectPlay"}); err == nil {
		t.Fatal("missing item error = nil")
	}
}

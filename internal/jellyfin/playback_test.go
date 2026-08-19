package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlaybackInfoPostsDeviceProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/Items/item-id/PlaybackInfo" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var request playbackInfoRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.UserID != "user-id" || !request.EnableDirectPlay || !request.EnableDirectStream || !request.EnableTranscoding {
			t.Errorf("request = %#v", request)
		}
		if request.StartTimeTicks == nil || *request.StartTimeTicks != 1234 {
			t.Errorf("StartTimeTicks = %#v", request.StartTimeTicks)
		}
		if len(request.DeviceProfile.DirectPlayProfiles) == 0 || len(request.DeviceProfile.TranscodingProfiles) == 0 {
			t.Errorf("DeviceProfile = %#v", request.DeviceProfile)
		}
		_, _ = w.Write([]byte(`{"MediaSources":[{"Id":"source-id","Container":"mkv","SupportsDirectPlay":true,"SupportsDirectStream":true,"SupportsTranscoding":true,"RequiredHttpHeaders":{"Referer":"https://example.test"}}],"PlaySessionId":"play-session"}`))
	}))
	defer server.Close()

	response, err := newTestClient(t, server.URL).WithToken("token").PlaybackInfo(context.Background(), "item-id", "user-id", PlaybackInfoOptions{StartTimeTicks: 1234, MaxStreamingBitrate: 20_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if response.PlaySessionID != "play-session" || len(response.MediaSources) != 1 || response.MediaSources[0].RequiredHTTPHeaders["Referer"] != "https://example.test" {
		t.Fatalf("PlaybackInfo() = %#v", response)
	}
}

func TestPlaybackInfoRejectsNoMediaSourceAndServerError(t *testing.T) {
	for _, response := range []string{`{"MediaSources":[]}`, `{"MediaSources":[],"ErrorCode":"NoCompatibleStream"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(response)) }))
		_, err := newTestClient(t, server.URL).WithToken("token").PlaybackInfo(context.Background(), "item", "user", PlaybackInfoOptions{})
		server.Close()
		if err == nil {
			t.Fatalf("PlaybackInfo() for %s error = nil", response)
		}
	}
}

func TestPlaybackInfoValidatesInput(t *testing.T) {
	client := newTestClient(t, "https://media.example.test").WithToken("token")
	for _, tc := range []struct{ item, user string }{{"", "user"}, {"bad/id", "user"}, {"item", ""}} {
		if _, err := client.PlaybackInfo(context.Background(), tc.item, tc.user, PlaybackInfoOptions{}); err == nil {
			t.Fatalf("PlaybackInfo(%q, %q) error = nil", tc.item, tc.user)
		}
	}
	if _, err := client.PlaybackInfo(context.Background(), "item", "user", PlaybackInfoOptions{StartTimeTicks: -1}); err == nil {
		t.Fatal("PlaybackInfo() negative start error = nil")
	}
}

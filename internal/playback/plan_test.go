package playback

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"jellycli/internal/jellyfin"
)

func TestBuildPlanRanksAllSources(t *testing.T) {
	info := jellyfin.PlaybackInfoResponse{PlaySessionID: "session-id", MediaSources: []jellyfin.MediaSource{
		{ID: "transcode-first", SupportsTranscoding: true, TranscodingURL: "/Videos/item/master.m3u8"},
		{ID: "direct-second", SupportsDirectPlay: true, RequiredHTTPHeaders: map[string]string{"Referer": "value"}},
	}}
	plan, err := BuildPlan("item-id", info)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != DirectPlay || plan.MediaSourceID != "direct-second" {
		t.Fatalf("BuildPlan() = %#v", plan)
	}
	u, err := url.Parse(plan.Resource)
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/Videos/item-id/stream" || u.Query().Get("static") != "true" || u.Query().Get("mediaSourceId") != "direct-second" || u.Query().Get("playSessionId") != "session-id" {
		t.Fatalf("Resource = %q", plan.Resource)
	}
	plan.RequiredHeaders["Referer"] = "changed"
	if info.MediaSources[1].RequiredHTTPHeaders["Referer"] != "value" {
		t.Fatal("BuildPlan did not clone source headers")
	}
}

func TestBuildPlanFallbacks(t *testing.T) {
	tests := []struct {
		name   string
		source jellyfin.MediaSource
		mode   Mode
	}{
		{"direct stream", jellyfin.MediaSource{ID: "source", SupportsDirectStream: true, TranscodingURL: "/stream.mp4"}, DirectStream},
		{"transcode", jellyfin.MediaSource{ID: "source", SupportsTranscoding: true, TranscodingURL: "/master.m3u8"}, Transcode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := BuildPlan("item", jellyfin.PlaybackInfoResponse{MediaSources: []jellyfin.MediaSource{tt.source}})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Mode != tt.mode || plan.Resource != tt.source.TranscodingURL {
				t.Fatalf("BuildPlan() = %#v", plan)
			}
		})
	}
}

func TestBuildPlanRejectsIncompatibleSources(t *testing.T) {
	_, err := BuildPlan("item", jellyfin.PlaybackInfoResponse{MediaSources: []jellyfin.MediaSource{{ID: "source"}}})
	if !errors.Is(err, ErrNoCompatibleMedia) {
		t.Fatalf("error = %v, want ErrNoCompatibleMedia", err)
	}
}

func TestPlanMediaPreservesBasePathAndUsesTokenHeader(t *testing.T) {
	plan := Plan{Resource: "/Videos/item/stream?static=true&api_key=server-leaked-token&X-Emby-Token=other", RequiredHeaders: map[string]string{"Referer": "value"}}
	media, err := plan.Media("https://media.example.test/jellyfin/", "secret-token", "Title", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if media.URL != "https://media.example.test/jellyfin/Videos/item/stream?static=true" {
		t.Fatalf("URL = %q", media.URL)
	}
	if media.Headers["X-Emby-Token"] != "secret-token" || media.Headers["Referer"] != "value" {
		t.Fatalf("Headers = %#v", media.Headers)
	}
	if media.StartTime != 30*time.Second || media.Title != "Title" {
		t.Fatalf("Media = %#v", media)
	}
}

func TestPlanMediaStripsTokenFromAbsoluteNegotiatedURL(t *testing.T) {
	media, err := (Plan{Resource: "https://media.example/video.m3u8?api_key=secret&playSessionId=session"}).Media("https://media.example", "token", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(media.URL, "secret") || media.URL != "https://media.example/video.m3u8?playSessionId=session" {
		t.Fatalf("URL = %q", media.URL)
	}
}

func TestPlanMediaRejectsCrossOriginAndTokenOverride(t *testing.T) {
	if _, err := (Plan{Resource: "https://evil.example/video"}).Media("https://media.example", "token", "", 0); err == nil {
		t.Fatal("Media() cross-origin error = nil")
	}
	if _, err := (Plan{Resource: "/video", RequiredHeaders: map[string]string{"x-emby-token": "evil"}}).Media("https://media.example", "token", "", 0); err == nil {
		t.Fatal("Media() token override error = nil")
	}
}

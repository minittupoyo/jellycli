package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"jellycli/internal/jellyfin"
)

type fakeLibraryLister struct {
	items []jellyfin.Item
	err   error
}

type fakeFrontendService struct {
	items  []jellyfin.Item
	term   string
	played string
	err    error
}

func (f *fakeFrontendService) Search(_ context.Context, term string) ([]jellyfin.Item, error) {
	f.term = term
	return f.items, f.err
}
func (f *fakeFrontendService) Play(_ context.Context, id string) error {
	f.played = id
	return f.err
}

func (f fakeLibraryLister) Libraries(context.Context) ([]jellyfin.Item, error) {
	return f.items, f.err
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("stdout = %q, want usage", stdout.String())
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"missing"}, &stdout, &stderr); code != 2 {
		t.Fatalf("Run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr = %q, want unknown-command error", stderr.String())
	}
}

func TestRunCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if code := Run(ctx, nil, &stdout, &stderr); code != 1 {
		t.Fatalf("Run() code = %d, want 1", code)
	}
}

func TestRunLibraries(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunWithDependencies(context.Background(), []string{"libraries"}, &stdout, &stderr, Dependencies{
		LibraryLister: fakeLibraryLister{items: []jellyfin.Item{{ID: "movies-id", Name: "Movies", CollectionType: "movies"}}},
	})
	if code != 0 {
		t.Fatalf("RunWithDependencies() code = %d; stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"NAME", "Movies", "movies", "movies-id"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunSearchAndPlay(t *testing.T) {
	percentage := 42.0
	season, episode := 2, 3
	service := &fakeFrontendService{items: []jellyfin.Item{{
		ID: "episode-id", Name: "Pilot", SeriesName: "Example", Type: jellyfin.ItemKindEpisode,
		ParentIndexNumber: &season, IndexNumber: &episode,
		UserData: &jellyfin.UserData{PlayedPercentage: &percentage},
	}}}
	var stdout, stderr bytes.Buffer
	deps := Dependencies{Searcher: service, Player: service}
	if code := RunWithDependencies(context.Background(), []string{"search", "pilot"}, &stdout, &stderr, deps); code != 0 {
		t.Fatalf("search code = %d, stderr = %q", code, stderr.String())
	}
	if service.term != "pilot" || !strings.Contains(stdout.String(), "S02E03") || !strings.Contains(stdout.String(), "42%") || !strings.Contains(stdout.String(), "episode-id") {
		t.Fatalf("search term/output = %q/%q", service.term, stdout.String())
	}
	if code := RunWithDependencies(context.Background(), []string{"play", "episode-id"}, &stdout, &stderr, deps); code != 0 {
		t.Fatalf("play code = %d, stderr = %q", code, stderr.String())
	}
	if service.played != "episode-id" {
		t.Fatalf("played = %q", service.played)
	}
}

func TestRunSearchAndPlayRequireArguments(t *testing.T) {
	for _, command := range []string{"search", "play"} {
		var stdout, stderr bytes.Buffer
		if code := RunWithDependencies(context.Background(), []string{command}, &stdout, &stderr, Dependencies{}); code != 2 {
			t.Fatalf("%s code = %d, want 2", command, code)
		}
	}
}

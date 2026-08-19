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

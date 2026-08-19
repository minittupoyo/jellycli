package mpv

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jellycli/internal/player"
)

type fakeProcess struct {
	startErr error
	waitErr  error
	onWait   func()
}

func (p *fakeProcess) Start() error { return p.startErr }
func (p *fakeProcess) Wait() error {
	if p.onWait != nil {
		p.onWait()
	}
	return p.waitErr
}

func TestPlayUsesPrivateConfigWithoutArgumentSecrets(t *testing.T) {
	root := t.TempDir()
	var gotArgs []string
	var configPath string
	p := New(Options{
		TempRoot: root,
		LookPath: func(string) (string, error) { return "/usr/bin/mpv", nil },
		Command: func(ctx context.Context, executable string, args []string, stdin io.Reader, stdout, stderr io.Writer) Process {
			gotArgs = append([]string(nil), args...)
			configPath = strings.TrimPrefix(args[0], "--include=")
			return &fakeProcess{onWait: func() {
				data, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatal(err)
				}
				text := string(data)
				if !strings.Contains(text, "X-Emby-Token: secret-token") || !strings.Contains(text, "Referer: value,with#special") {
					t.Fatalf("config = %q", text)
				}
				info, err := os.Stat(configPath)
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("config mode = %v, err = %v", info.Mode().Perm(), err)
				}
			}}
		},
	})
	err := p.Play(context.Background(), player.Media{
		URL: "https://media.example.test/video", StartTime: 90*time.Second + 250*time.Millisecond,
		Title: "Example", Headers: map[string]string{"X-Emby-Token": "secret-token", "Referer": "value,with#special"},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(gotArgs, " ")
	if strings.Contains(joined, "secret-token") || strings.Contains(joined, "Referer") {
		t.Fatalf("arguments leak headers: %q", joined)
	}
	for _, want := range []string{"--start=90.250", "--force-media-title=Example", "--", "https://media.example.test/video"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("arguments = %q, want %q", joined, want)
		}
	}
	if _, err := os.Stat(filepath.Dir(configPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary directory remains: %v", err)
	}
}

func TestPlayClassifiesFailures(t *testing.T) {
	p := New(Options{LookPath: func(string) (string, error) { return "", errors.New("missing") }})
	if err := p.Play(context.Background(), player.Media{URL: "https://example.test/video"}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("error = %v, want ErrNotInstalled", err)
	}

	p = New(Options{
		TempRoot: t.TempDir(), LookPath: func(string) (string, error) { return "/mpv", nil },
		Command: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) Process {
			return &fakeProcess{startErr: errors.New("boom")}
		},
	})
	if err := p.Play(context.Background(), player.Media{URL: "https://example.test/video"}); !errors.Is(err, ErrStart) {
		t.Fatalf("error = %v, want ErrStart", err)
	}
}

func TestPlayRejectsHeaderInjection(t *testing.T) {
	p := New(Options{LookPath: func(string) (string, error) { return "/mpv", nil }})
	err := p.Play(context.Background(), player.Media{URL: "https://example.test/video", Headers: map[string]string{"X-Test": "ok\r\nInjected: yes"}})
	if err == nil {
		t.Fatal("Play() error = nil, want invalid-header error")
	}
}

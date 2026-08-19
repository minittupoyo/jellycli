package mpv

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"jellycli/internal/player"
)

type fakeProcess struct {
	startErr error
	waitErr  error
	exit     chan struct{}
	once     sync.Once
}

func (p *fakeProcess) Start() error { return p.startErr }
func (p *fakeProcess) Wait() error {
	if p.exit != nil {
		<-p.exit
	}
	return p.waitErr
}
func (p *fakeProcess) Kill() error { p.finish(); return nil }
func (p *fakeProcess) finish() {
	if p.exit != nil {
		p.once.Do(func() { close(p.exit) })
	}
}

func TestStartUsesPrivateConfigWithoutArgumentSecrets(t *testing.T) {
	process := &fakeProcess{exit: make(chan struct{})}
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	var gotArgs []string
	var configPath string
	p := New(Options{
		TempRoot: t.TempDir(), LookPath: func(string) (string, error) { return "/usr/bin/mpv", nil },
		Command: func(ctx context.Context, executable string, args []string, stdin io.Reader, stdout, stderr io.Writer) Process {
			gotArgs = append([]string(nil), args...)
			configPath = strings.TrimPrefix(args[0], "--include=")
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
			return process
		},
		Dial: func(context.Context, string) (net.Conn, error) { return clientConn, nil },
	})
	session, err := p.Start(context.Background(), player.Media{
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
	for _, want := range []string{"--input-ipc-server=", "--start=90.250", "--force-media-title=Example", "--", "https://media.example.test/video"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("arguments = %q, want %q", joined, want)
		}
	}
	process.finish()
	if err := session.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(configPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary directory remains: %v", err)
	}
}

func TestSessionControlsAndProperties(t *testing.T) {
	process := &fakeProcess{exit: make(chan struct{})}
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	go serveIPC(serverConn, process)
	p := New(Options{
		TempRoot: t.TempDir(), LookPath: func(string) (string, error) { return "/mpv", nil },
		Command: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) Process { return process },
		Dial:    func(context.Context, string) (net.Conn, error) { return clientConn, nil },
	})
	s, err := p.Start(context.Background(), player.Media{URL: "https://example.test/video"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Seek(ctx, 42*time.Second); err != nil {
		t.Fatal(err)
	}
	position, err := s.Position(ctx)
	if err != nil || position != 12_500*time.Millisecond {
		t.Fatalf("Position() = %v, %v", position, err)
	}
	duration, err := s.Duration(ctx)
	if err != nil || duration != 120*time.Second {
		t.Fatalf("Duration() = %v, %v", duration, err)
	}
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}
}

func serveIPC(conn net.Conn, process *fakeProcess) {
	decoder := json.NewDecoder(bufio.NewReader(conn))
	encoder := json.NewEncoder(conn)
	for {
		var request ipcRequest
		if err := decoder.Decode(&request); err != nil {
			return
		}
		name, _ := request.Command[0].(string)
		response := map[string]any{"request_id": request.RequestID, "error": "success"}
		if name == "get_property" {
			if request.Command[1] == "time-pos" {
				response["data"] = 12.5
			} else {
				response["data"] = 120.0
			}
		}
		if err := encoder.Encode(response); err != nil {
			return
		}
		if name == "quit" {
			process.finish()
			return
		}
	}
}

func TestStartClassifiesFailures(t *testing.T) {
	p := New(Options{LookPath: func(string) (string, error) { return "", errors.New("missing") }})
	if _, err := p.Start(context.Background(), player.Media{URL: "https://example.test/video"}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("error = %v, want ErrNotInstalled", err)
	}
	p = New(Options{
		TempRoot: t.TempDir(), LookPath: func(string) (string, error) { return "/mpv", nil },
		Command: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) Process {
			return &fakeProcess{startErr: errors.New("boom")}
		},
	})
	if _, err := p.Start(context.Background(), player.Media{URL: "https://example.test/video"}); !errors.Is(err, ErrStart) {
		t.Fatalf("error = %v, want ErrStart", err)
	}
}

func TestStartRejectsHeaderInjection(t *testing.T) {
	p := New(Options{LookPath: func(string) (string, error) { return "/mpv", nil }})
	_, err := p.Start(context.Background(), player.Media{URL: "https://example.test/video", Headers: map[string]string{"X-Test": "ok\r\nInjected: yes"}})
	if err == nil {
		t.Fatal("Start() error = nil, want invalid-header error")
	}
}

func TestSessionReportsEndFileError(t *testing.T) {
	process := &fakeProcess{exit: make(chan struct{})}
	clientConn, serverConn := net.Pipe()
	p := New(Options{
		TempRoot: t.TempDir(), LookPath: func(string) (string, error) { return "/mpv", nil },
		Command: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) Process { return process },
		Dial:    func(context.Context, string) (net.Conn, error) { return clientConn, nil },
	})
	s, err := p.Start(context.Background(), player.Media{URL: "https://example.test/video"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(serverConn).Encode(map[string]any{"event": "end-file", "reason": "error", "file_error": "HTTP 500"}); err != nil {
		t.Fatal(err)
	}
	event := <-s.Events()
	if event.Name != "end-file" || event.FileError != "HTTP 500" {
		t.Fatalf("event = %#v", event)
	}
	process.finish()
	if err := s.Wait(); !errors.Is(err, ErrPlayback) {
		t.Fatalf("Wait() error = %v, want ErrPlayback", err)
	}
	_ = serverConn.Close()
}

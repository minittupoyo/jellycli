// Package mpv implements player.Player using mpv JSON IPC.
package mpv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"jellycli/internal/player"
)

var (
	ErrNotInstalled = errors.New("mpv is not installed")
	ErrStart        = errors.New("mpv failed to start")
	ErrIPC          = errors.New("mpv IPC failed")
	ErrPlayback     = errors.New("mpv playback failed")
)

type Process interface {
	Start() error
	Wait() error
	Kill() error
}

type CommandFactory func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) Process
type LookPathFunc func(string) (string, error)
type DialFunc func(context.Context, string) (net.Conn, error)

type Options struct {
	Binary         string
	TempRoot       string
	ConnectTimeout time.Duration
	RetryInterval  time.Duration
	LookPath       LookPathFunc
	Command        CommandFactory
	Dial           DialFunc
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
}

type Player struct{ options Options }

func New(options Options) *Player {
	if options.Binary == "" {
		options.Binary = "mpv"
	}
	if options.ConnectTimeout <= 0 {
		options.ConnectTimeout = 5 * time.Second
	}
	if options.RetryInterval <= 0 {
		options.RetryInterval = 25 * time.Millisecond
	}
	if options.LookPath == nil {
		options.LookPath = exec.LookPath
	}
	if options.Command == nil {
		options.Command = defaultCommand
	}
	if options.Dial == nil {
		options.Dial = dialUnix
	}
	if options.Stdin == nil {
		options.Stdin = os.Stdin
	}
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
	return &Player{options: options}
}

func (p *Player) Start(ctx context.Context, media player.Media) (player.Session, error) {
	if err := validateMedia(media); err != nil {
		return nil, fmt.Errorf("play media: %w", err)
	}
	executable, err := p.options.LookPath(p.options.Binary)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotInstalled, p.options.Binary)
	}

	tempDir, err := os.MkdirTemp(p.options.TempRoot, "jellycli-mpv-")
	if err != nil {
		return nil, fmt.Errorf("prepare mpv authentication: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	if err := os.Chmod(tempDir, 0o700); err != nil {
		cleanup()
		return nil, fmt.Errorf("secure mpv temporary directory: %w", err)
	}
	configPath := filepath.Join(tempDir, "network.conf")
	if err := writeHeaderConfig(configPath, media.Headers); err != nil {
		cleanup()
		return nil, err
	}
	ipcPath := filepath.Join(tempDir, "ipc.sock")

	args := []string{"--include=" + configPath, "--input-ipc-server=" + ipcPath}
	if media.StartTime > 0 {
		args = append(args, "--start="+strconv.FormatFloat(media.StartTime.Seconds(), 'f', 3, 64))
	}
	if media.Title != "" {
		args = append(args, "--force-media-title="+media.Title)
	}
	args = append(args, "--", media.URL)
	processCtx, cancelProcess := context.WithCancel(ctx)
	process := p.options.Command(processCtx, executable, args, p.options.Stdin, p.options.Stdout, p.options.Stderr)
	if err := process.Start(); err != nil {
		cancelProcess()
		cleanup()
		return nil, fmt.Errorf("%w: %v", ErrStart, err)
	}
	rawExit := make(chan error, 1)
	go func() { rawExit <- process.Wait() }()

	connectCtx, cancelConnect := context.WithTimeout(ctx, p.options.ConnectTimeout)
	defer cancelConnect()
	conn, err := p.connect(connectCtx, ipcPath, rawExit)
	if err != nil {
		cancelProcess()
		_ = process.Kill()
		cleanup()
		return nil, err
	}
	client := newIPCClient(conn)
	s := &session{
		ipc: client, cancelProcess: cancelProcess, cleanup: cleanup,
		rawExit: rawExit, done: make(chan error, 1), events: make(chan player.Event, 32),
	}
	go s.monitor()
	return s, nil
}

func (p *Player) connect(ctx context.Context, path string, rawExit <-chan error) (net.Conn, error) {
	ticker := time.NewTicker(p.options.RetryInterval)
	defer ticker.Stop()
	var lastErr error
	for {
		conn, err := p.options.Dial(ctx, path)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		select {
		case processErr := <-rawExit:
			return nil, fmt.Errorf("%w: mpv exited before IPC connected: %v", ErrIPC, processErr)
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: connect timeout: %v", ErrIPC, lastErr)
		case <-ticker.C:
		}
	}
}

type session struct {
	ipc           *ipcClient
	cancelProcess context.CancelFunc
	cleanup       func()
	rawExit       <-chan error
	done          chan error
	events        chan player.Event
}

func (s *session) monitor() {
	defer close(s.events)
	var playbackErr error
	ipcEvents := s.ipc.Events()
	for {
		select {
		case event, ok := <-ipcEvents:
			if !ok {
				ipcEvents = nil
				continue
			}
			converted := player.Event{Name: event.Event, Reason: event.Reason, FileError: event.FileError}
			if event.Event == "end-file" && event.Reason == "error" {
				playbackErr = fmt.Errorf("%w: %s", ErrPlayback, event.FileError)
			}
			select {
			case s.events <- converted:
			default:
			}
		case processErr := <-s.rawExit:
			s.ipc.Close()
			s.cancelProcess()
			s.cleanup()
			if playbackErr != nil {
				s.done <- playbackErr
			} else if processErr != nil {
				s.done <- fmt.Errorf("%w: %v", ErrPlayback, processErr)
			} else {
				s.done <- nil
			}
			return
		}
	}
}

func (s *session) Pause(ctx context.Context) error {
	return s.ipc.command(ctx, nil, "set_property", "pause", true)
}
func (s *session) Resume(ctx context.Context) error {
	return s.ipc.command(ctx, nil, "set_property", "pause", false)
}
func (s *session) Seek(ctx context.Context, position time.Duration) error {
	if position < 0 {
		return errors.New("seek: position must not be negative")
	}
	return s.ipc.command(ctx, nil, "seek", position.Seconds(), "absolute")
}
func (s *session) Position(ctx context.Context) (time.Duration, error) {
	return s.propertyDuration(ctx, "time-pos")
}
func (s *session) Duration(ctx context.Context) (time.Duration, error) {
	return s.propertyDuration(ctx, "duration")
}
func (s *session) Paused(ctx context.Context) (bool, error) {
	var paused bool
	if err := s.ipc.command(ctx, &paused, "get_property", "pause"); err != nil {
		return false, err
	}
	return paused, nil
}
func (s *session) propertyDuration(ctx context.Context, property string) (time.Duration, error) {
	var seconds float64
	if err := s.ipc.command(ctx, &seconds, "get_property", property); err != nil {
		return 0, err
	}
	if seconds < 0 {
		return 0, fmt.Errorf("mpv property %s was negative", property)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
func (s *session) Stop(ctx context.Context) error { return s.ipc.command(ctx, nil, "quit") }
func (s *session) Events() <-chan player.Event    { return s.events }
func (s *session) Wait() error                    { return <-s.done }

type execProcess struct{ *exec.Cmd }

func (p execProcess) Kill() error {
	if p.Process == nil {
		return nil
	}
	return p.Process.Kill()
}
func defaultCommand(ctx context.Context, executable string, args []string, stdin io.Reader, stdout, stderr io.Writer) Process {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return execProcess{cmd}
}
func dialUnix(ctx context.Context, path string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", path)
}

func validateMedia(media player.Media) error {
	u, err := url.Parse(media.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("media URL must be an absolute http or https URL")
	}
	if media.StartTime < 0 {
		return errors.New("start time must not be negative")
	}
	for name, value := range media.Headers {
		if !validHeaderName(name) {
			return fmt.Errorf("invalid HTTP header name %q", name)
		}
		if !validHeaderValue(value) {
			return fmt.Errorf("invalid value for HTTP header %q", name)
		}
	}
	return nil
}
func writeHeaderConfig(path string, headers map[string]string) error {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var config strings.Builder
	config.WriteString("http-header-fields-clr\n")
	for _, name := range names {
		field := name + ": " + headers[name]
		fmt.Fprintf(&config, "http-header-fields-append=%%%d%%%s\n", len([]byte(field)), field)
	}
	if err := os.WriteFile(path, []byte(config.String()), 0o600); err != nil {
		return fmt.Errorf("write mpv authentication config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure mpv authentication config: %w", err)
	}
	return nil
}
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
			return false
		}
	}
	return true
}
func validHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if (value[i] < 0x20 && value[i] != '\t') || value[i] == 0x7f {
			return false
		}
	}
	return true
}

var _ player.Player = (*Player)(nil)
var _ player.Session = (*session)(nil)

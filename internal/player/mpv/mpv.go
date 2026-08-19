// Package mpv implements player.Player using the mpv executable.
package mpv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"jellycli/internal/player"
)

var (
	ErrNotInstalled = errors.New("mpv is not installed")
	ErrStart        = errors.New("mpv failed to start")
	ErrPlayback     = errors.New("mpv playback failed")
)

type Process interface {
	Start() error
	Wait() error
}

type CommandFactory func(ctx context.Context, executable string, args []string, stdin io.Reader, stdout, stderr io.Writer) Process

type LookPathFunc func(string) (string, error)

type Options struct {
	Binary   string
	TempRoot string
	LookPath LookPathFunc
	Command  CommandFactory
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

type Player struct {
	options Options
}

func New(options Options) *Player {
	if options.Binary == "" {
		options.Binary = "mpv"
	}
	if options.LookPath == nil {
		options.LookPath = exec.LookPath
	}
	if options.Command == nil {
		options.Command = defaultCommand
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

func (p *Player) Play(ctx context.Context, media player.Media) error {
	if err := validateMedia(media); err != nil {
		return fmt.Errorf("play media: %w", err)
	}
	executable, err := p.options.LookPath(p.options.Binary)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotInstalled, p.options.Binary)
	}

	tempDir, err := os.MkdirTemp(p.options.TempRoot, "jellycli-mpv-")
	if err != nil {
		return fmt.Errorf("prepare mpv authentication: %w", err)
	}
	defer os.RemoveAll(tempDir)
	if err := os.Chmod(tempDir, 0o700); err != nil {
		return fmt.Errorf("secure mpv temporary directory: %w", err)
	}
	configPath := filepath.Join(tempDir, "network.conf")
	if err := writeHeaderConfig(configPath, media.Headers); err != nil {
		return err
	}

	args := []string{"--include=" + configPath}
	if media.StartTime > 0 {
		args = append(args, "--start="+strconv.FormatFloat(media.StartTime.Seconds(), 'f', 3, 64))
	}
	if media.Title != "" {
		args = append(args, "--force-media-title="+media.Title)
	}
	args = append(args, "--", media.URL)
	process := p.options.Command(ctx, executable, args, p.options.Stdin, p.options.Stdout, p.options.Stderr)
	if err := process.Start(); err != nil {
		return fmt.Errorf("%w: %v", ErrStart, err)
	}
	if err := process.Wait(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("%w: %v", ErrPlayback, err)
	}
	return nil
}

func defaultCommand(ctx context.Context, executable string, args []string, stdin io.Reader, stdout, stderr io.Writer) Process {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd
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
		if !httpgutsValidHeaderValue(value) {
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

func httpgutsValidHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if (value[i] < 0x20 && value[i] != '\t') || value[i] == 0x7f {
			return false
		}
	}
	return true
}

var _ player.Player = (*Player)(nil)

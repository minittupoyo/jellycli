// Package cli provides the command-line entry point independently of the TUI.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"jellycli/internal/application"
	"jellycli/internal/jellyfin"
)

const version = "dev"

// Run executes jellycli and returns a process exit code. Writers are injected so
// commands remain testable and the future TUI does not have to share stdout logs.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return RunWithDependencies(ctx, args, stdout, stderr, Dependencies{})
}

type LibraryLister interface {
	Libraries(context.Context) ([]jellyfin.Item, error)
}

type Searcher interface {
	Search(context.Context, string) ([]jellyfin.Item, error)
}

type Player interface {
	Play(context.Context, string) error
}

type Authenticator interface {
	Login(context.Context, string, string, string) error
	Logout(context.Context) error
}

type DebugLogger interface {
	Error(string, error)
}

type Dependencies struct {
	LibraryLister LibraryLister
	Searcher      Searcher
	Player        Player
	Authenticator Authenticator
	Stdin         io.Reader
	RunTUI        func(context.Context) error
	Debug         DebugLogger
}

func RunWithDependencies(ctx context.Context, args []string, stdout, stderr io.Writer, deps Dependencies) int {
	if err := ctx.Err(); err != nil {
		printError(stderr, deps.Debug, "startup", err)
		return 1
	}

	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "jellycli %s\n", version)
		return 0
	case "login":
		if len(args) != 3 {
			fmt.Fprintln(stderr, "jellycli: usage: jellycli login <server-url> <username> < password-file")
			return 2
		}
		if deps.Authenticator == nil {
			fmt.Fprintln(stderr, "jellycli: login command is unavailable")
			return 1
		}
		if deps.Stdin == nil {
			fmt.Fprintln(stderr, "jellycli: login requires a password on stdin")
			return 2
		}
		password, err := readPassword(deps.Stdin)
		if err != nil {
			printError(stderr, deps.Debug, "login", err)
			return 1
		}
		if err := deps.Authenticator.Login(ctx, args[1], args[2], password); err != nil {
			printError(stderr, deps.Debug, "login", err)
			return 1
		}
		fmt.Fprintln(stdout, "Logged in.")
		return 0
	case "logout":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "jellycli: usage: jellycli logout")
			return 2
		}
		if deps.Authenticator == nil {
			fmt.Fprintln(stderr, "jellycli: logout command is unavailable")
			return 1
		}
		if err := deps.Authenticator.Logout(ctx); err != nil {
			printError(stderr, deps.Debug, "logout", err)
			return 1
		}
		fmt.Fprintln(stdout, "Logged out.")
		return 0
	case "tui":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "jellycli: usage: jellycli tui")
			return 2
		}
		if deps.RunTUI == nil {
			fmt.Fprintln(stderr, "jellycli: tui command is unavailable")
			return 1
		}
		if err := deps.RunTUI(ctx); err != nil {
			printError(stderr, deps.Debug, "tui", err)
			return 1
		}
		return 0
	case "libraries":
		if deps.LibraryLister == nil {
			fmt.Fprintln(stderr, "jellycli: libraries command is unavailable")
			return 1
		}
		items, err := deps.LibraryLister.Libraries(ctx)
		if err != nil {
			printError(stderr, deps.Debug, "libraries", err)
			return 1
		}
		printLibraries(stdout, items)
		return 0
	case "search":
		if len(args) != 2 || args[1] == "" {
			fmt.Fprintln(stderr, "jellycli: usage: jellycli search <term>")
			return 2
		}
		if deps.Searcher == nil {
			fmt.Fprintln(stderr, "jellycli: search command is unavailable")
			return 1
		}
		items, err := deps.Searcher.Search(ctx, args[1])
		if err != nil {
			printError(stderr, deps.Debug, "search", err)
			return 1
		}
		printItems(stdout, items)
		return 0
	case "play":
		if len(args) != 2 || args[1] == "" {
			fmt.Fprintln(stderr, "jellycli: usage: jellycli play <item-id>")
			return 2
		}
		if deps.Player == nil {
			fmt.Fprintln(stderr, "jellycli: play command is unavailable")
			return 1
		}
		if err := deps.Player.Play(ctx, args[1]); err != nil {
			printError(stderr, deps.Debug, "play", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "jellycli: unknown command %q\n", args[0])
		fmt.Fprintln(stderr, "Run 'jellycli help' for usage.")
		return 2
	}
}

func printError(w io.Writer, debug DebugLogger, operation string, err error) {
	if debug != nil {
		debug.Error(operation, err)
	}
	fmt.Fprintf(w, "jellycli: %s: %s\n", operation, application.UserError(operation, err))
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `jellycli - browse and play media from a Jellyfin server

Usage:
  jellycli <command>

Commands:
  help       Show this help
  version    Show the version
  login      Log in; reads password from stdin
  logout     Revoke and remove the saved token
  libraries  List Jellyfin libraries
  search     Search videos by title
  play       Play an item by ID
  tui        Start the terminal interface`)
}

func readPassword(r io.Reader) (string, error) {
	const maxPasswordBytes = 4096
	data, err := io.ReadAll(io.LimitReader(r, maxPasswordBytes+1))
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if len(data) > maxPasswordBytes {
		return "", errors.New("password exceeds 4096 bytes")
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if password == "" {
		return "", errors.New("password is empty")
	}
	return password, nil
}

func printItems(w io.Writer, items []jellyfin.Item) {
	if len(items) == 0 {
		fmt.Fprintln(w, "No items found.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TITLE\tTYPE\tPROGRESS\tRUNTIME\tID")
	for _, item := range items {
		progress := "-"
		if item.UserData != nil {
			if item.UserData.Played {
				progress = "played"
			} else if item.UserData.PlayedPercentage != nil {
				progress = fmt.Sprintf("%.0f%%", *item.UserData.PlayedPercentage)
			}
		}
		runtime := "-"
		if item.RunTimeTicks != nil {
			runtime = (time.Duration(*item.RunTimeTicks/10_000_000) * time.Second).Round(time.Minute).String()
		}
		title := item.Name
		if item.Type == jellyfin.ItemKindEpisode && item.ParentIndexNumber != nil && item.IndexNumber != nil {
			title = fmt.Sprintf("%s S%02dE%02d %s", item.SeriesName, *item.ParentIndexNumber, *item.IndexNumber, item.Name)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", title, item.Type, progress, runtime, item.ID)
	}
	_ = tw.Flush()
}

func printLibraries(w io.Writer, items []jellyfin.Item) {
	if len(items) == 0 {
		fmt.Fprintln(w, "No libraries found.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tTYPE\tID")
	for _, item := range items {
		itemType := item.CollectionType
		if itemType == "" {
			itemType = string(item.Type)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", item.Name, itemType, item.ID)
	}
	_ = tw.Flush()
}

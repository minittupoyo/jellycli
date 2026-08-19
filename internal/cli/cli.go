// Package cli provides the command-line entry point independently of the TUI.
package cli

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

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

type Dependencies struct {
	LibraryLister LibraryLister
}

func RunWithDependencies(ctx context.Context, args []string, stdout, stderr io.Writer, deps Dependencies) int {
	if err := ctx.Err(); err != nil {
		fmt.Fprintf(stderr, "jellycli: %v\n", err)
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
	case "libraries":
		if deps.LibraryLister == nil {
			fmt.Fprintln(stderr, "jellycli: libraries command is unavailable")
			return 1
		}
		items, err := deps.LibraryLister.Libraries(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "jellycli: list libraries: %v\n", err)
			return 1
		}
		printLibraries(stdout, items)
		return 0
	default:
		fmt.Fprintf(stderr, "jellycli: unknown command %q\n", args[0])
		fmt.Fprintln(stderr, "Run 'jellycli help' for usage.")
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `jellycli - browse and play media from a Jellyfin server

Usage:
  jellycli <command>

Commands:
  help       Show this help
  version    Show the version
  libraries  List Jellyfin libraries

Planned commands:
  login, logout, search, play, tui`)
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

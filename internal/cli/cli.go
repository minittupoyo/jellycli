// Package cli provides the command-line entry point independently of the TUI.
package cli

import (
	"context"
	"fmt"
	"io"
)

const version = "dev"

// Run executes jellycli and returns a process exit code. Writers are injected so
// commands remain testable and the future TUI does not have to share stdout logs.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
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

Planned commands:
  login, logout, libraries, search, play, tui`)
}

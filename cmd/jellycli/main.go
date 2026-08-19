package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"jellycli/internal/application"
	"jellycli/internal/cli"
	"jellycli/internal/config"
	"jellycli/internal/debuglog"
	"jellycli/internal/platform"
	"jellycli/internal/player/mpv"
	"jellycli/internal/tui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), platform.TerminationSignals()...)
	defer stop()
	os.Exit(run(ctx, os.Args[1:]))
}

func run(ctx context.Context, args []string) int {
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jellycli: %v\n", err)
		return 1
	}
	deviceName, err := os.Hostname()
	if err != nil || deviceName == "" {
		deviceName = "Linux device"
	}
	var logger *debuglog.Logger
	if settings, loadErr := config.NewStore(paths).LoadSettings(); loadErr == nil && settings.Debug {
		logPath := settings.LogFile
		if logPath == "" {
			logPath = paths.Log
		}
		var token string
		if state, stateErr := config.NewStore(paths).LoadState(); stateErr == nil && state.Auth != nil {
			token = state.Auth.AccessToken
		}
		logger, err = debuglog.Open(logPath, token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jellycli: %v\n", err)
			return 1
		}
		defer logger.Close()
	}
	service, err := application.NewService(
		config.NewStore(paths),
		&http.Client{Timeout: 30 * time.Second},
		deviceName,
		"dev",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jellycli: %v\n", err)
		return 1
	}
	service.WithPlayer(mpv.New(mpv.Options{}))
	return cli.RunWithDependencies(ctx, args, os.Stdout, os.Stderr, cli.Dependencies{
		LibraryLister: service, Searcher: service, Player: service,
		Authenticator: service, Stdin: os.Stdin,
		PasswordReader: cli.TerminalPasswordReader(os.Stdin, os.Stderr),
		RunTUI:         func(ctx context.Context) error { return tui.Run(ctx, service, logger) },
		Debug:          logger,
	})
}

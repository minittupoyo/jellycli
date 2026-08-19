package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jellycli/internal/application"
	"jellycli/internal/cli"
	"jellycli/internal/config"
	"jellycli/internal/player/mpv"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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
	})
}

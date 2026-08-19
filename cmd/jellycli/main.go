package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"jellycli/internal/application"
	"jellycli/internal/cli"
	"jellycli/internal/config"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:]))
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
	return cli.RunWithDependencies(ctx, args, os.Stdout, os.Stderr, cli.Dependencies{LibraryLister: service})
}

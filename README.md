# jellycli

`jellycli` is a Linux-first CLI/TUI client for browsing a Jellyfin library and
playing video with mpv while synchronizing playback state with Jellyfin.

The project is being implemented in verified phases. Phase 1 establishes a
small, testable Go command without committing later layers to a CLI framework.

## Development

Requires Go 1.24 or newer.

```sh
go test ./...
go build ./cmd/jellycli
./jellycli help
```

After a server URL and login state have been saved by the forthcoming login
command, the current application path can list libraries with:

```sh
./jellycli libraries
```

See [docs/design.md](docs/design.md) for the researched API surface,
architecture, directory plan, and implementation phases.

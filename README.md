# jellycli

`jellycli` is a Linux-first CLI/TUI client for browsing a Jellyfin library and
playing video with mpv while synchronizing playback state with Jellyfin.

The project is being implemented in verified phases with a testable CLI and a
separate application layer shared by the forthcoming TUI.

## Development

Requires Go 1.24 or newer.

```sh
go test ./...
go build ./cmd/jellycli
./jellycli help
```

Log in without placing the password in process arguments, then browse, search,
and play by item ID:

```sh
printf '%s\n' 'your-password' | ./jellycli login https://jellyfin.example alice
./jellycli libraries
./jellycli search "title"
./jellycli play ITEM_ID
./jellycli logout
```

See [docs/design.md](docs/design.md) for the researched API surface,
architecture, directory plan, and implementation phases.

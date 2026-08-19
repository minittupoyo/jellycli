# jellycli

`jellycli` is a Linux-first CLI/TUI client for browsing a Jellyfin library and
playing video with mpv while synchronizing playback state with Jellyfin.

## Requirements

- Go 1.25 or newer (to build)
- A reachable Jellyfin server
- `mpv` available on `PATH`

## Build

```sh
go test ./...
go build -o jellycli ./cmd/jellycli
```

## Login and commands

The password is requested interactively without echoing it and is never stored:

```sh
./jellycli login https://jellyfin.example alice
```

For automation, redirect a password file or pipe stdin; non-terminal input is
read without displaying a prompt.

```text
jellycli
jellycli libraries
jellycli search "title"
jellycli play ITEM_ID
jellycli tui
jellycli logout
```

Running `jellycli` without a command starts the TUI. Search results include
movies, series, episodes, and other videos; selecting a series opens its seasons.

The TUI keys are `↑`/`k`, `↓`/`j`, `Enter`, `Esc`, `/`, and `q`. While entering
a search query, `q` is ordinary text and `Esc` cancels the query. mpv runs with
its normal UI and key bindings; the TUI releases the terminal for playback and
restores it afterward.

## Configuration and security

Settings are stored below `${XDG_CONFIG_HOME:-~/.config}/jellycli`; the stable
device ID and access token are below
`${XDG_STATE_HOME:-~/.local/state}/jellycli`. Files use mode `0600` and managed
directories use mode `0700`. Passwords are not persisted. Tokens are sent to mpv
through a private temporary config file, not through its command arguments or
media URL.

Debug logging is disabled by default. To enable JSON logs without writing over
the TUI, edit `config.json`:

```json
{
  "server_url": "https://jellyfin.example",
  "debug": true
}
```

The default log is `${XDG_STATE_HOME:-~/.local/state}/jellycli/debug.log` with
mode `0600`. Set `log_file` in `config.json` to choose another path. Known access
tokens and authentication query parameters are redacted.

## End-to-end smoke test

1. Run `jellycli login`, then `jellycli libraries` and `jellycli search TITLE`.
2. Run `jellycli play ITEM_ID`; confirm mpv opens and its pause/seek keys work.
3. Stop near the middle, reopen the item, and confirm playback resumes.
4. In Jellyfin Web, confirm Now Playing, progress, Continue Watching, and the
   final stopped position update.
5. Run `jellycli tui`; traverse Movies and Series → Seasons → Episodes, search
   with `/`, play a result, return with `Esc`, and quit with `q`.
6. Interrupt playback with Ctrl-C and confirm Jellyfin receives the final
   position. Temporarily disconnect the server and confirm a concise network
   message is shown; re-login after invalidating the token and confirm recovery.

Linux is the supported playback target. Process/player interfaces and
platform-specific IPC/signal adapters are separated for future macOS and Windows
support; Windows named-pipe playback is not implemented yet.

See [docs/design.md](docs/design.md) for API choices, architecture, and the
implementation phases.

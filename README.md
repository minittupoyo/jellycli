# jellycli

[![Build](https://github.com/minittupoyo/jellycli/actions/workflows/build.yml/badge.svg)](https://github.com/minittupoyo/jellycli/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/minittupoyo/jellycli)](https://github.com/minittupoyo/jellycli/releases/latest)

`jellycli` is a Go-based CLI/TUI client for browsing and watching media from a Jellyfin server. It uses [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the terminal interface and `mpv` for playback.

## Features

- Browse libraries, series, seasons, episodes, movies, and other media in a hierarchical TUI
- Open Continue Watching, Next Up, and Recently Added views
- Search episodes and top-level works such as series and movies
- Select Direct Play, Direct Stream, or Transcode playback sources
- Control `mpv` through JSON IPC and synchronize progress with Jellyfin
- Resume playback and report start, progress, stop, and played state
- Store configuration and credentials in private XDG-compatible files
- Use script-friendly commands when a full TUI is unnecessary

## Platform support

| Platform | Build | Playback support |
| --- | --- | --- |
| Linux amd64/arm64 | Released | Supported |
| macOS amd64/arm64 | Released | Experimental |
| Windows amd64/arm64 | Released | Not yet supported; named-pipe IPC remains to be implemented |

## Install

Download an archive for your platform from the [latest release](https://github.com/minittupoyo/jellycli/releases/latest).

For example, on Linux amd64:

```sh
version=0.1.0
curl -LO "https://github.com/minittupoyo/jellycli/releases/download/v${version}/jellycli-linux-amd64.tar.gz"
curl -LO "https://github.com/minittupoyo/jellycli/releases/download/v${version}/checksums.txt"
sha256sum --check --ignore-missing checksums.txt
tar -xzf jellycli-linux-amd64.tar.gz
install -m 0755 jellycli ~/.local/bin/jellycli
```

To build from source, install Go 1.25 or newer and run:

```sh
git clone https://github.com/minittupoyo/jellycli.git
cd jellycli
go build ./cmd/jellycli
```

Install `mpv` separately and ensure it is available on `PATH` before starting playback.

## Quick start

Log in interactively:

```sh
jellycli login
```

Then launch the TUI. Running without a subcommand starts it by default:

```sh
jellycli
```

Useful non-interactive commands include:

```sh
jellycli libraries
jellycli search "title"
jellycli play ITEM_ID
jellycli logout
```

Run `jellycli help` or `jellycli help COMMAND` for the complete command reference.

## TUI controls

| Key | Action |
| --- | --- |
| `↑`/`k`, `↓`/`j` | Move selection |
| `enter` | Open or play the selected item |
| `esc` | Go back or cancel search |
| `/` | Search |
| `q`/`ctrl+c` | Quit |

## Configuration and security

Configuration follows the XDG base-directory convention. Authentication data is written with owner-only permissions. Do not commit, publish, or paste files containing server URLs, access tokens, usernames, or device identifiers.

If credentials may have been exposed, revoke the Jellyfin access token and run `jellycli login` again.

## Documentation

- [Architecture](docs/architecture.md)
- [Detailed design and implementation history](docs/design.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Contributing](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

## Releases

Version tags matching `v*` trigger GitHub Actions to build archives for supported targets, generate checksums, and publish a GitHub release.

## License

No software license has been granted for this repository yet. Public availability does not by itself grant permission to copy, modify, or redistribute the code. A license may be added in a future release.

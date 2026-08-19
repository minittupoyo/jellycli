# Contributing

Thank you for your interest in improving `jellycli`.

## Development setup

You will need Go 1.25 or newer. Playback development also requires `mpv` and access to a Jellyfin server you are authorized to use.

```sh
git clone https://github.com/minittupoyo/jellycli.git
cd jellycli
go test ./...
go build ./cmd/jellycli
```

Before submitting a change, run:

```sh
gofmt -w .
go vet ./...
go test ./...
```

## Project structure

Keep command parsing and presentation separate from Jellyfin API access and playback control. New Jellyfin operations belong in the client layer, while terminal state and key handling belong in the TUI layer. See [the architecture guide](docs/architecture.md) for package responsibilities.

Prefer small, focused changes with tests. Do not include access tokens, server URLs, personal media names, or other private Jellyfin data in source files, fixtures, logs, screenshots, issues, or pull requests.

## Pull requests

A pull request should:

- Explain the user-visible behavior and motivation
- Include tests for new logic or bug fixes where practical
- Update documentation for commands, controls, configuration, or platform support
- Pass formatting, vetting, tests, and the GitHub Actions build matrix
- Note any manual Jellyfin or `mpv` verification performed

For playback changes, manually check start, pause/resume, seeking, natural completion, and early exit when possible. Confirm that Jellyfin receives sensible progress and played-state updates.

## Releases

Maintainers publish releases by updating the changelog, committing the release state, and pushing a semantic-version tag:

```sh
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin main vX.Y.Z
```

The release workflow builds archives and checksums and creates the GitHub release.

## License status

The repository currently has no software license. Contributions are not accepted under an open-source license until the maintainer selects and adds one. Please coordinate with the maintainer before contributing substantial code.

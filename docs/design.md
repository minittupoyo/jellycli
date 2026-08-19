# Design and implementation plan

## Scope and priorities

The client will authenticate with a user account, persist a device identity and
token (never the password), browse and search the user's library, select a
server-supported playback source, control mpv over JSON IPC, and report start,
progress, pause, position, and stop state to Jellyfin. Linux is the first target;
filesystem IPC and process-specific behavior will sit behind platform-neutral
interfaces.

Correct playback synchronization is a core feature. A successful mpv launch is
not treated as a successful Jellyfin playback session until the orchestration
layer can observe it and send PlaybackStart. The final known position is retained
for a best-effort PlaybackStopped report on normal exit, error, cancellation, and
signals.

## Researched Jellyfin API surface

The implementation will target the server's generated stable OpenAPI contract.
Before each API phase, request and response fields will be checked against that
contract rather than inferred from examples.

| Capability | Endpoint / contract |
| --- | --- |
| Server discovery | `GET /System/Info/Public` |
| Password login | `POST /Users/AuthenticateByName` with `Username` and `Pw` |
| Current user | `GET /Users/Me` |
| Logout/token revoke | `POST /Sessions/Logout` |
| Libraries/views | `GET /Users/{userId}/Views` |
| Browse hierarchy | `GET /Users/{userId}/Items` (parent, recursive, types, sorting, and user-data fields) |
| Recently added | `GET /Users/{userId}/Items/Latest` |
| Continue watching | `GET /Users/{userId}/Items/Resume` |
| Next up | `GET /Shows/NextUp` |
| Search | `GET /Items` using `searchTerm`, recursive/type filters, and user ID |
| Playback selection | `POST /Items/{itemId}/PlaybackInfo` with a device profile and playback capabilities |
| Direct media | `GET /Videos/{itemId}/stream` using the selected media source and static/direct-play options |
| Playback start | `POST /Sessions/Playing` with `PlaybackStartInfo` |
| Playback progress | `POST /Sessions/Playing/Progress` with `PlaybackProgressInfo` |
| Playback stop | `POST /Sessions/Playing/Stopped` with `PlaybackStopInfo` |

Requests use Jellyfin's MediaBrowser authorization metadata (client, device,
stable device ID, version, and token after authentication). The login request
also needs valid client/device metadata. Tokens in headers or query strings must
be redacted from diagnostics. Jellyfin represents playback positions and runtime
in 100 ns ticks, so conversion will be centralized and tested.

`PlaybackInfo` returns media sources and play-session information. Selection is
mode-neutral: the playback planner produces one of `DirectPlay`, `DirectStream`,
or `Transcode` plus a URL and headers. Phase 7 initially implements only a valid
Direct Play plan, but the result type will not encode Direct Play as the only
possibility.

## Researched mpv JSON IPC behavior

mpv will be started with `--input-ipc-server=<path>` while preserving its normal
UI and default key bindings. The client must not parse human-facing terminal
output or simulate keys on stdin.

JSON IPC exchanges newline-delimited JSON objects. Commands include a unique
`request_id`; replies contain the corresponding ID, `error`, and optional `data`.
The connection also carries asynchronous events, so a single reader goroutine
will demultiplex replies and events rather than allowing concurrent reads.

Required operations map to:

| Client operation | mpv command/property |
| --- | --- |
| Pause/resume | `set_property` for `pause` |
| Seek | `seek` (absolute or relative, explicitly selected) |
| Position | `time-pos` / `playback-time` |
| Duration | `duration` |
| State updates | `observe_property` for position, duration, pause, and core idle state |
| Stop | `stop` or `quit`, depending on lifecycle policy |
| Exit/error | process wait result plus `end-file` and `shutdown` events |

The IPC socket is private (0700 parent directory where applicable), uniquely
named per process, created by mpv, retried with a bounded startup timeout, and
removed on cleanup. A platform adapter will later replace Unix sockets with
Windows named pipes without changing the Player contract.

## Architecture

Dependencies point inward as follows:

```text
CLI / Bubble Tea TUI
        |
        v
application services (browse, search, playback orchestration)
        |                         |
        v                         v
Jellyfin API port             Player port
                                  |
                                  v
                              mpv JSON IPC
```

The TUI sends intents to application services and renders returned state; it
does not construct Jellyfin URLs or mpv commands. Playback orchestration owns
source selection, resume position, periodic reports, shutdown, and tick
conversion. Interfaces are introduced at these real test seams: HTTP transport,
Jellyfin service, clock/ticker, and Player.

Configuration follows XDG base directories. Non-secret preferences and server
URL go under `$XDG_CONFIG_HOME/jellycli`; stable device ID and authentication
state go under `$XDG_STATE_HOME/jellycli` with user-only permissions. If the
variables are unset, the standard user config/state fallbacks are used. Passwords
are held only for the login request and never persisted. File logging is opt-in,
written outside stdout, and uses structured redaction.

User-facing errors will have stable categories (configuration, authentication,
network, API, media unavailable, player missing/start/IPC/playback) while wrapping
the original cause for debug logs.

## Planned directory layout

```text
cmd/jellycli/             executable wiring only
internal/cli/             ordinary CLI parsing and presentation
internal/config/          XDG paths, settings, credentials, device identity
internal/jellyfin/        HTTP client, API DTOs, errors, URL/auth handling
internal/playback/        planning and Jellyfin/player synchronization
internal/player/          player-neutral contract and events
internal/player/mpv/      process lifecycle and JSON IPC
internal/tui/             Bubble Tea models, messages, and screens
internal/platform/        small OS-specific process/IPC helpers if required
docs/                     decisions and implementation notes
```

Packages will be added when their phase begins; empty placeholder packages are
intentionally avoided.

## Concrete phases and verification gates

1. **Project initialization (complete):** module, executable, injectable CLI
   runner, help/version behavior, unit tests, and this design. Gate: format,
   `go test ./...`, `go vet ./...`, and build.
2. **Configuration (complete):** XDG resolution, atomic permission-safe
   persistence, device ID generation, settings/auth split, and validation tests.
3. **Authentication:** HTTP foundation, MediaBrowser header, login, token/user
   capture, saved-token validation, logout; `httptest` coverage.
4. **Jellyfin browsing API:** typed pagination and DTOs for views, item hierarchy,
   resume, next-up, and latest; fixture/contract tests.
5. **CLI libraries:** commands and readable list output with exit-code mapping.
6. **PlaybackInfo:** explicit device profile, source parsing, and mode-neutral
   playback-plan selection tests.
7. **mpv Direct Play:** Player contract, process runner, URL/header handling,
   resume option, missing-binary/startup errors.
8. **mpv JSON IPC:** request correlation, event reader, property observation,
   pause/resume/seek/position/duration/stop, fake-socket tests.
9. **Playback sync:** start/progress/stopped state machine, cancellation/signals,
   periodic reporting, exactly-once stop, fake clock/player/API tests.
10. **Resume:** server ticks to player start position and boundary behavior.
11. **Search:** API and CLI search/play integration.
12. **Bubble Tea TUI:** Home and hierarchy screens, input-aware key handling, and
    asynchronous commands through application services.
13. **Direct Stream/transcode:** profile negotiation and server-provided URLs;
    lifecycle/session cleanup and integration tests.
14. **Hardening:** network interruption, token expiry UX, debug file logging,
    race tests, cross-platform seams, and end-to-end smoke documentation.

## Phase 1 decisions

The module currently has no third-party dependency. Bubble Tea is deferred until
the TUI phase so early API/player work remains small. The CLI parser is likewise
standard-library code; a framework will only be considered if command complexity
demonstrates a concrete need. The module path is local (`jellycli`) until a
canonical repository URL is chosen; changing it before a public release is
mechanical and avoids guessing ownership now.

## Phase 2 decisions

Settings and private state are separate JSON files. Both use mode 0600 and their
application directories use mode 0700; using the stricter permissions for the
non-secret settings file keeps the persistence path uniform. Writes use a
same-directory temporary file, file and directory sync, then atomic rename.

The device identity is an RFC 9562 version 4 UUID generated with `crypto/rand`.
It survives logout, while logout removes only the access token and user ID.
Persistent types deliberately contain no password field. Relative XDG base paths
are rejected because the XDG specification requires absolute paths. Server URLs
accept only absolute HTTP(S) URLs without embedded credentials, queries, or
fragments; a Jellyfin base path remains supported.

## Primary references

- Jellyfin generated API documentation: <https://api.jellyfin.org/>
- Jellyfin stable OpenAPI document: <https://api.jellyfin.org/openapi/jellyfin-openapi-stable.json>
- Jellyfin client codec behavior: <https://jellyfin.org/docs/general/clients/codec-support/>
- mpv reference manual, JSON IPC and command interface: <https://mpv.io/manual/master/>

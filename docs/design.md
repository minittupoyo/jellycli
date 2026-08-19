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
| Libraries/views | `GET /UserViews?userId=...` |
| Browse hierarchy | `GET /Items?userId=...` (parent, recursive, types, sorting, and user-data fields) |
| Recently added | `GET /Items/Latest?userId=...` |
| Continue watching | `GET /UserItems/Resume?userId=...` |
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
3. **Authentication (complete):** HTTP foundation, MediaBrowser header, login,
   token/user capture, saved-token validation, logout, and `httptest` coverage.
4. **Jellyfin browsing API (complete):** typed pagination and DTOs for views,
   item hierarchy, resume, next-up, and latest; fixture/contract tests.
5. **CLI libraries (complete):** application service, saved-login reconnection,
   token validation, and readable list output with exit-code mapping.
6. **PlaybackInfo (complete):** explicit mpv device profile, source parsing, and
   mode-neutral playback-plan selection tests.
7. **mpv Direct Play (complete):** Player contract, process runner, URL/header
   handling, resume option, and missing-binary/startup errors.
8. **mpv JSON IPC (complete):** request correlation, event reader,
   pause/resume/seek/position/duration/stop, and fake-socket tests.
9. **Playback sync (complete):** start/progress/stopped state machine,
   cancellation/signals, periodic reporting, exactly-once stop, and fake
   clock/player/API tests.
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

## Phase 3 decisions

The Jellyfin client owns URL joining, JSON size limits, MediaBrowser authorization,
and HTTP status classification. Its HTTP executor is injected, and authenticated
clients are immutable copies created with `WithToken`, avoiding a token mutation
race. Login sends only the OpenAPI `Username` and `Pw` fields and never includes
the password in an error. Non-success response bodies are not echoed because they
are untrusted and may contain reflected secrets.

Authorization metadata is sent using the modern `Authorization: MediaBrowser ...`
form. Values are URL-encoded before entering the structured header, preventing
quotes or control characters in device metadata from altering its structure.
Saved-token validation uses `GET /Users/Me`; server logout uses
`POST /Sessions/Logout`. Persistence remains in the config package so the HTTP
client does not gain filesystem responsibilities.

## Phase 4 decisions

The browsing API follows the Jellyfin 12.0 stable OpenAPI published in July 2026.
That contract uses top-level `/UserViews`, `/Items`, `/UserItems/Resume`, and
`/Items/Latest` routes with a `userId` query parameter; older user-ID-in-path
routes are therefore not baked into the client. Endpoint contract tests assert
the exact paths and query serialization.

Only the BaseItemDto fields needed by the CLI/TUI are modeled. Nullable runtime,
episode, season, and percentage fields use pointers so a real zero remains
different from a missing value. Unknown item kinds and future response fields
remain forward-compatible. The general Items query supports the hierarchy and
sorting needed for movies, series, seasons, and episodes, while home-screen
queries have smaller endpoint-specific option types.

## Phase 5 decisions

The CLI depends on an application-level `LibraryLister` rather than constructing
HTTP calls. The application service reads persisted XDG settings, builds an
authenticated client, validates the token with `/Users/Me`, verifies the returned
user matches the saved user ID, and then fetches `/UserViews`. This same service
can later be injected into Bubble Tea commands.

`jellycli libraries` prints stable tabular columns for name, collection type, and
ID and returns a nonzero status for configuration, authentication, network, or
API errors. The process-wide HTTP client has a 30-second timeout. Login is still
Phase 3 API capability rather than a public CLI command; its interactive command
will be added before claiming end-user authentication UX complete.

## Phase 6 decisions

Playback negotiation uses `POST /Items/{itemId}/PlaybackInfo` with a
`PlaybackInfoDto` body; the OpenAPI marks equivalent query parameters obsolete.
The explicit mpv profile advertises broad ffmpeg-backed containers/codecs, an
HLS/H.264 fallback, all three delivery modes, and stream-copy support. Jellyfin
remains authoritative about whether each returned media source is playable.

The playback planner examines every source rather than accepting the first one.
It globally prefers Direct Play, then Direct Stream, then Transcode, retaining
the server order within a mode. Direct Play gets a static `/Videos/{id}/stream`
resource with media-source and play-session IDs. Direct Stream and Transcode use
the URL negotiated by Jellyfin. Plans retain required upstream HTTP headers and
runtime, but do not yet launch a process; absolute URL/authentication assembly is
part of the mpv Direct Play phase.

## Phase 7 decisions

The Player boundary accepts player-neutral media. mpv receives the media URL,
optional resume time, and title while retaining its normal terminal/UI behavior
and user key bindings.

Access tokens are never added to URLs or process arguments. HTTP headers are
written to a mode-0600 mpv include file inside a mode-0700 temporary directory.
Each header uses mpv's fixed-length quoting and the `http-header-fields-append`
list operation, so commas, `#`, spaces, and quotes cannot change configuration
structure. Header names and values are validated against injection, and the
temporary directory is removed after start failure or process exit. Required
media-source headers cannot override `X-Emby-Token`.

Server-relative stream paths preserve a configured Jellyfin base path. Absolute
negotiated URLs are accepted only when their scheme and host match the configured
server, preventing authentication from being forwarded cross-origin. Process
errors distinguish missing mpv, launch failure, context cancellation, and an
abnormal playback exit.

## Phase 8 decisions

`Player.Start` returns a live `Session`. Pause, resume, absolute seek, position,
duration, and stop are structured JSON IPC commands, each with a monotonically
increasing request ID. A single decoder goroutine owns the socket read side and
correlates responses to concurrent callers while delivering asynchronous events
separately.

mpv creates a unique Unix socket in the existing private temporary directory.
Connection attempts use a bounded five-second timeout and stop early if the mpv
process exits. The session combines process exit status with `end-file` error
events, closes IPC, cancels the child context, and removes the socket and secret
configuration. IPC and process factories are injected so the full control path
runs in tests with `net.Pipe`, without mpv or a Jellyfin server.

## Phase 9 decisions

The Jellyfin client implements the stable OpenAPI payloads for
`/Sessions/Playing`, `/Sessions/Playing/Progress`, and
`/Sessions/Playing/Stopped`. Player durations are converted centrally to
Jellyfin's 100 ns ticks. Every report carries item, media-source, play-session,
position, and play method; start/progress also include seek and pause state, and
stop records abnormal failure.

The synchronizer sends Start before entering its event loop, samples mpv
position and pause state every ten seconds, and preserves the last successful
sample. A transient Progress failure does not terminate playback and is cleared
after a later successful report. Once Start succeeds, every terminal branch
sends Stopped exactly once. Normal/abnormal process exit, caller cancellation,
and IPC failure all use a separate bounded cleanup context so a canceled parent
does not suppress the final server notification. The executable translates
SIGINT and SIGTERM into cancellation of this same application context.

## Primary references

- Jellyfin generated API documentation: <https://api.jellyfin.org/>
- Jellyfin stable OpenAPI document: <https://api.jellyfin.org/openapi/jellyfin-openapi-stable.json>
- Jellyfin client codec behavior: <https://jellyfin.org/docs/general/clients/codec-support/>
- mpv reference manual, JSON IPC and command interface: <https://mpv.io/manual/master/>

# Architecture

`jellycli` keeps terminal presentation, Jellyfin communication, and player control separated so each can be tested and evolved independently.

```text
cmd/jellycli
    |
    +-- command and configuration layer
    |       |
    |       +-- Jellyfin HTTP client
    |       +-- playback orchestration
    |
    +-- Bubble Tea TUI
            |
            +-- navigation and view state
            +-- commands dispatched to the same client/playback services

playback orchestration
    |
    +-- source selection (direct play / direct stream / transcode)
    +-- mpv process and JSON IPC
    +-- Jellyfin playback-session reporting
```

## Responsibilities

| Area | Responsibility |
| --- | --- |
| CLI entry point | Parse commands, load configuration, and choose CLI or TUI execution |
| TUI | Model navigation, search input, loading/error states, and user actions |
| Jellyfin client | Authenticate and map Jellyfin HTTP APIs into Go types |
| Playback | Select media sources, launch and observe `mpv`, and report session state |
| Configuration | Persist server and authentication settings with private permissions |

## Playback flow

1. Resolve the selected Jellyfin item and request playback information.
2. Choose a compatible direct-play, direct-stream, or transcoded URL.
3. Start `mpv` with a local IPC endpoint.
4. Notify Jellyfin that playback started.
5. Observe position, pause, seeking, and termination through IPC.
6. Send periodic progress updates and a final stopped or completed state.

Platform-specific process and IPC behavior should remain behind the playback boundary. In particular, Windows named-pipe support should not leak into TUI or Jellyfin client code.

For detailed design decisions and implementation phases, see [design.md](design.md).

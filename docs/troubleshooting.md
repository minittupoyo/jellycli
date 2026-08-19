# Troubleshooting

## Login expires or requests are unauthorized

Run `jellycli login` again. If the old token may be compromised, revoke it from the Jellyfin dashboard first.

## The server cannot be reached

Confirm the configured URL includes the scheme (`http://` or `https://`) and is reachable from the same machine. Check reverse-proxy, TLS certificate, firewall, and VPN settings. Avoid disabling TLS verification; fix the certificate chain instead.

## Playback does not start

Verify that `mpv` is installed and discoverable:

```sh
mpv --version
```

Then confirm the selected item is playable in another Jellyfin client. Server logs can reveal missing transcode tools, permissions, or unsupported codecs.

## Playback starts but progress does not synchronize

Ensure only one `jellycli` instance is controlling the playback session. Check that the local IPC endpoint can be created and that its parent directory is writable. Abrupt process termination can prevent the final stopped event, although periodic progress should still preserve a recent position.

## Windows playback

Windows binaries are published, but playback is not currently supported because `mpv` named-pipe IPC has not been implemented. Browsing and other non-playback commands may still work.

## Reset local authentication

Locate the `jellycli` configuration under your platform's XDG-compatible configuration directory, remove the credential file, and run `jellycli login`. Back up non-sensitive preferences first if the configuration is combined in your installed version.

Never include access tokens or private server details when sharing diagnostic output.

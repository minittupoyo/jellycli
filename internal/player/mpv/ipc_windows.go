//go:build windows

package mpv

import (
	"context"
	"errors"
	"net"
)

// The player boundary remains portable; a Windows named-pipe adapter can
// replace this default without changing playback orchestration.
func dialIPC(context.Context, string) (net.Conn, error) {
	return nil, errors.New("mpv IPC named pipes are not implemented on Windows")
}

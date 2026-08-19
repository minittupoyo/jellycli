// Package player defines the media-player boundary used by playback services.
package player

import (
	"context"
	"time"
)

type Media struct {
	URL       string
	Headers   map[string]string
	StartTime time.Duration
	Title     string
}

// Player is deliberately small for the process-launch phase. JSON IPC control
// will extend this boundary with a live Session in the next phase.
type Player interface {
	Play(context.Context, Media) error
}

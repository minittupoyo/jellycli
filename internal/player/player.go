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

// Player launches media and returns a live, controllable playback session.
type Player interface {
	Start(context.Context, Media) (Session, error)
}

type Event struct {
	Name      string
	Reason    string
	FileError string
}

type Session interface {
	Pause(context.Context) error
	Resume(context.Context) error
	Seek(context.Context, time.Duration) error
	Position(context.Context) (time.Duration, error)
	Duration(context.Context) (time.Duration, error)
	Stop(context.Context) error
	Events() <-chan Event
	Wait() error
}

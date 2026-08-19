package playback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"jellycli/internal/jellyfin"
	"jellycli/internal/player"
)

type Reporter interface {
	ReportPlaybackStart(context.Context, jellyfin.PlaybackState) error
	ReportPlaybackProgress(context.Context, jellyfin.PlaybackState) error
	ReportPlaybackStopped(context.Context, jellyfin.PlaybackState) error
}

type Ticker interface {
	Chan() <-chan time.Time
	Stop()
}

type Clock interface {
	NewTicker(time.Duration) Ticker
}

type realClock struct{}
type realTicker struct{ *time.Ticker }

func (realClock) NewTicker(interval time.Duration) Ticker {
	return realTicker{time.NewTicker(interval)}
}
func (ticker realTicker) Chan() <-chan time.Time { return ticker.C }

type Synchronizer struct {
	Reporter       Reporter
	Clock          Clock
	ProgressPeriod time.Duration
	CleanupTimeout time.Duration
}

func NewSynchronizer(reporter Reporter) *Synchronizer {
	return &Synchronizer{
		Reporter: reporter, Clock: realClock{}, ProgressPeriod: 10 * time.Second,
		CleanupTimeout: 5 * time.Second,
	}
}

// Run owns Jellyfin reporting for an already-started player session. It returns
// only after playback ends or ctx is canceled.
func (s *Synchronizer) Run(ctx context.Context, session player.Session, initial jellyfin.PlaybackState) error {
	if s.Reporter == nil || session == nil {
		return errors.New("synchronize playback: reporter and player session are required")
	}
	if s.Clock == nil {
		s.Clock = realClock{}
	}
	if s.ProgressPeriod <= 0 {
		return errors.New("synchronize playback: progress period must be positive")
	}
	if s.CleanupTimeout <= 0 {
		s.CleanupTimeout = 5 * time.Second
	}
	if err := s.Reporter.ReportPlaybackStart(ctx, initial); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), s.CleanupTimeout)
		defer cancel()
		_ = session.Stop(cleanupCtx)
		return fmt.Errorf("synchronize playback: %w", err)
	}

	state := initial
	ticker := s.Clock.NewTicker(s.ProgressPeriod)
	defer ticker.Stop()
	waitResult := make(chan error, 1)
	go func() { waitResult <- session.Wait() }()
	var progressErr error

	for {
		select {
		case <-ticker.Chan():
			if err := snapshot(ctx, session, &state); err != nil {
				progressErr = err
				continue
			}
			if err := s.Reporter.ReportPlaybackProgress(ctx, state); err != nil {
				progressErr = err
			} else {
				progressErr = nil
			}
		case waitErr := <-waitResult:
			cleanupCtx, cancel := context.WithTimeout(context.Background(), s.CleanupTimeout)
			_ = snapshot(cleanupCtx, session, &state)
			state.Failed = waitErr != nil
			stopErr := s.Reporter.ReportPlaybackStopped(cleanupCtx, state)
			cancel()
			return errors.Join(waitErr, progressErr, stopErr)
		case <-ctx.Done():
			cleanupCtx, cancel := context.WithTimeout(context.Background(), s.CleanupTimeout)
			_ = snapshot(cleanupCtx, session, &state)
			_ = session.Stop(cleanupCtx)
			state.Failed = false
			stopErr := s.Reporter.ReportPlaybackStopped(cleanupCtx, state)
			cancel()
			return errors.Join(ctx.Err(), progressErr, stopErr)
		}
	}
}

func snapshot(ctx context.Context, session player.Session, state *jellyfin.PlaybackState) error {
	position, err := session.Position(ctx)
	if err != nil {
		return fmt.Errorf("read player position: %w", err)
	}
	paused, err := session.Paused(ctx)
	if err != nil {
		return fmt.Errorf("read player pause state: %w", err)
	}
	state.Position = position
	state.Paused = paused
	return nil
}

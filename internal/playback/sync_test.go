package playback

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"jellycli/internal/jellyfin"
	"jellycli/internal/player"
)

type fakeTicker struct{ ch chan time.Time }

func (ticker *fakeTicker) Chan() <-chan time.Time { return ticker.ch }
func (ticker *fakeTicker) Stop()                  {}

type fakeClock struct{ ticker *fakeTicker }

func (clock fakeClock) NewTicker(time.Duration) Ticker { return clock.ticker }

type fakeReporter struct {
	mu           sync.Mutex
	starts       []jellyfin.PlaybackState
	progress     []jellyfin.PlaybackState
	stops        []jellyfin.PlaybackState
	progressSeen chan struct{}
}

func (r *fakeReporter) ReportPlaybackStart(_ context.Context, state jellyfin.PlaybackState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts = append(r.starts, state)
	return nil
}
func (r *fakeReporter) ReportPlaybackProgress(_ context.Context, state jellyfin.PlaybackState) error {
	r.mu.Lock()
	r.progress = append(r.progress, state)
	r.mu.Unlock()
	if r.progressSeen != nil {
		select {
		case r.progressSeen <- struct{}{}:
		default:
		}
	}
	return nil
}
func (r *fakeReporter) ReportPlaybackStopped(_ context.Context, state jellyfin.PlaybackState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stops = append(r.stops, state)
	return nil
}

type fakeSession struct {
	mu        sync.Mutex
	position  time.Duration
	paused    bool
	wait      chan error
	stopCalls int
}

func (s *fakeSession) Pause(context.Context) error {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
	return nil
}
func (s *fakeSession) Resume(context.Context) error {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
	return nil
}
func (s *fakeSession) Seek(_ context.Context, p time.Duration) error {
	s.mu.Lock()
	s.position = p
	s.mu.Unlock()
	return nil
}
func (s *fakeSession) Position(context.Context) (time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.position, nil
}
func (s *fakeSession) Duration(context.Context) (time.Duration, error) { return time.Hour, nil }
func (s *fakeSession) Paused(context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused, nil
}
func (s *fakeSession) Stop(context.Context) error {
	s.mu.Lock()
	s.stopCalls++
	s.mu.Unlock()
	select {
	case s.wait <- nil:
	default:
	}
	return nil
}
func (s *fakeSession) Events() <-chan player.Event { return make(chan player.Event) }
func (s *fakeSession) Wait() error                 { return <-s.wait }

func TestSynchronizerReportsLifecycle(t *testing.T) {
	ticker := &fakeTicker{ch: make(chan time.Time, 1)}
	reporter := &fakeReporter{progressSeen: make(chan struct{}, 1)}
	session := &fakeSession{position: 5 * time.Second, paused: true, wait: make(chan error, 1)}
	syncer := &Synchronizer{Reporter: reporter, Clock: fakeClock{ticker}, ProgressPeriod: time.Second, CleanupTimeout: time.Second}
	done := make(chan error, 1)
	go func() {
		done <- syncer.Run(context.Background(), session, jellyfin.PlaybackState{ItemID: "item", MediaSourceID: "source", PlaySessionID: "play", PlayMethod: "DirectPlay"})
	}()
	ticker.ch <- time.Now()
	select {
	case <-reporter.progressSeen:
	case <-time.After(time.Second):
		t.Fatal("progress was not reported")
	}
	session.mu.Lock()
	session.position = 9 * time.Second
	session.paused = false
	session.mu.Unlock()
	session.wait <- nil
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	reporter.mu.Lock()
	defer reporter.mu.Unlock()
	if len(reporter.starts) != 1 || len(reporter.progress) != 1 || len(reporter.stops) != 1 {
		t.Fatalf("reports = start %d, progress %d, stop %d", len(reporter.starts), len(reporter.progress), len(reporter.stops))
	}
	if reporter.progress[0].Position != 5*time.Second || !reporter.progress[0].Paused {
		t.Fatalf("progress = %#v", reporter.progress[0])
	}
	if reporter.stops[0].Position != 9*time.Second || reporter.stops[0].Failed {
		t.Fatalf("stop = %#v", reporter.stops[0])
	}
}

func TestSynchronizerCancellationStopsAndReportsOnce(t *testing.T) {
	ticker := &fakeTicker{ch: make(chan time.Time)}
	reporter := &fakeReporter{}
	session := &fakeSession{position: 7 * time.Second, wait: make(chan error, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	syncer := &Synchronizer{Reporter: reporter, Clock: fakeClock{ticker}, ProgressPeriod: time.Second, CleanupTimeout: time.Second}
	go func() {
		done <- syncer.Run(ctx, session, jellyfin.PlaybackState{ItemID: "item", PlayMethod: "DirectPlay"})
	}()
	for {
		reporter.mu.Lock()
		started := len(reporter.starts) > 0
		reporter.mu.Unlock()
		if started {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	reporter.mu.Lock()
	stops := len(reporter.stops)
	reporter.mu.Unlock()
	session.mu.Lock()
	stopCalls := session.stopCalls
	session.mu.Unlock()
	if stops != 1 || stopCalls != 1 {
		t.Fatalf("stops = %d, player stops = %d", stops, stopCalls)
	}
}

func TestSynchronizerMarksAbnormalExitFailed(t *testing.T) {
	ticker := &fakeTicker{ch: make(chan time.Time)}
	reporter := &fakeReporter{}
	session := &fakeSession{wait: make(chan error, 1)}
	done := make(chan error, 1)
	syncer := &Synchronizer{Reporter: reporter, Clock: fakeClock{ticker}, ProgressPeriod: time.Second, CleanupTimeout: time.Second}
	go func() {
		done <- syncer.Run(context.Background(), session, jellyfin.PlaybackState{ItemID: "item", PlayMethod: "Transcode"})
	}()
	session.wait <- errors.New("crashed")
	if err := <-done; err == nil {
		t.Fatal("error = nil")
	}
	reporter.mu.Lock()
	defer reporter.mu.Unlock()
	if len(reporter.stops) != 1 || !reporter.stops[0].Failed {
		t.Fatalf("stops = %#v", reporter.stops)
	}
}

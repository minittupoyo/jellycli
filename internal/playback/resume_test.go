package playback

import (
	"math"
	"testing"
	"time"

	"jellycli/internal/jellyfin"
)

func ticks(duration time.Duration) int64 { return duration.Nanoseconds() / nanosecondsPerJellyfinTick }

func resumeItem(position, runtime time.Duration, played bool) jellyfin.Item {
	runtimeTicks := ticks(runtime)
	return jellyfin.Item{
		RunTimeTicks: &runtimeTicks,
		UserData:     &jellyfin.UserData{Played: played, PlaybackPositionTicks: ticks(position)},
	}
}

func TestDecideResumeUsesSavedPosition(t *testing.T) {
	item := resumeItem(23*time.Minute+456*time.Millisecond, 90*time.Minute, false)
	decision, err := DecideResume(item, DefaultResumePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Resume || decision.Position != 23*time.Minute+456*time.Millisecond || decision.StartTimeTicks != item.UserData.PlaybackPositionTicks {
		t.Fatalf("DecideResume() = %#v", decision)
	}
	options := decision.PlaybackInfoOptions(20_000_000)
	if options.StartTimeTicks != item.UserData.PlaybackPositionTicks || options.MaxStreamingBitrate != 20_000_000 {
		t.Fatalf("PlaybackInfoOptions() = %#v", options)
	}
}

func TestDecideResumeStartsOverAtBoundaries(t *testing.T) {
	tests := []struct {
		name string
		item jellyfin.Item
	}{
		{"missing user data", jellyfin.Item{}},
		{"played", resumeItem(20*time.Minute, time.Hour, true)},
		{"zero", resumeItem(0, time.Hour, false)},
		{"near beginning", resumeItem(4*time.Second, time.Hour, false)},
		{"near end", resumeItem(59*time.Minute+57*time.Second, time.Hour, false)},
		{"at runtime", resumeItem(time.Hour, time.Hour, false)},
		{"beyond runtime", resumeItem(2*time.Hour, time.Hour, false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := DecideResume(tt.item, DefaultResumePolicy())
			if err != nil {
				t.Fatal(err)
			}
			if decision.Resume || decision.Position != 0 || decision.StartTimeTicks != 0 || decision.Reason == "" {
				t.Fatalf("DecideResume() = %#v", decision)
			}
		})
	}
}

func TestDecideResumeAllowsUnknownRuntime(t *testing.T) {
	item := jellyfin.Item{UserData: &jellyfin.UserData{PlaybackPositionTicks: ticks(10 * time.Minute)}}
	decision, err := DecideResume(item, DefaultResumePolicy())
	if err != nil || !decision.Resume || decision.Position != 10*time.Minute {
		t.Fatalf("DecideResume() = %#v, %v", decision, err)
	}
}

func TestDecideResumeRejectsOverflowAndInvalidPolicy(t *testing.T) {
	item := jellyfin.Item{UserData: &jellyfin.UserData{PlaybackPositionTicks: math.MaxInt64}}
	if _, err := DecideResume(item, DefaultResumePolicy()); err == nil {
		t.Fatal("overflow error = nil")
	}
	if _, err := DecideResume(jellyfin.Item{}, ResumePolicy{MinimumPosition: -time.Second}); err == nil {
		t.Fatal("invalid policy error = nil")
	}
}

func TestResumeBoundaryIsInclusiveAtThreshold(t *testing.T) {
	policy := DefaultResumePolicy()
	item := resumeItem(policy.MinimumPosition, time.Hour, false)
	decision, err := DecideResume(item, policy)
	if err != nil || !decision.Resume {
		t.Fatalf("minimum position decision = %#v, %v", decision, err)
	}
	item = resumeItem(time.Hour-policy.MinimumRemaining, time.Hour, false)
	decision, err = DecideResume(item, policy)
	if err != nil || !decision.Resume {
		t.Fatalf("minimum remaining decision = %#v, %v", decision, err)
	}
}

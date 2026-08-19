package playback

import (
	"errors"
	"fmt"
	"math"
	"time"

	"jellycli/internal/jellyfin"
)

const nanosecondsPerJellyfinTick = 100

type ResumePolicy struct {
	// MinimumPosition avoids resuming a video that was stopped almost
	// immediately after it began.
	MinimumPosition time.Duration
	// MinimumRemaining avoids reopening at the final frames. Jellyfin remains
	// authoritative about whether the item is marked played.
	MinimumRemaining time.Duration
}

func DefaultResumePolicy() ResumePolicy {
	return ResumePolicy{MinimumPosition: 5 * time.Second, MinimumRemaining: 5 * time.Second}
}

type ResumeDecision struct {
	Position       time.Duration
	StartTimeTicks int64
	Resume         bool
	Reason         string
}

// PlaybackInfoOptions keeps Jellyfin's negotiation position identical to the
// position that will later be passed to the player.
func (d ResumeDecision) PlaybackInfoOptions(maxStreamingBitrate int) jellyfin.PlaybackInfoOptions {
	return jellyfin.PlaybackInfoOptions{
		StartTimeTicks:      d.StartTimeTicks,
		MaxStreamingBitrate: maxStreamingBitrate,
	}
}

// DecideResume converts Jellyfin ticks exactly once and applies conservative
// boundary checks. A zero-position decision always starts from the beginning.
func DecideResume(item jellyfin.Item, policy ResumePolicy) (ResumeDecision, error) {
	if policy.MinimumPosition < 0 || policy.MinimumRemaining < 0 {
		return ResumeDecision{}, errors.New("decide resume: policy durations must not be negative")
	}
	if item.UserData == nil {
		return fromBeginning("no user playback data"), nil
	}
	if item.UserData.Played {
		return fromBeginning("item is marked played"), nil
	}
	ticks := item.UserData.PlaybackPositionTicks
	if ticks <= 0 {
		return fromBeginning("no saved position"), nil
	}
	position, err := ticksToDuration(ticks)
	if err != nil {
		return ResumeDecision{}, err
	}
	if position < policy.MinimumPosition {
		return fromBeginning("saved position is near the beginning"), nil
	}

	if item.RunTimeTicks != nil && *item.RunTimeTicks > 0 {
		runtime, err := ticksToDuration(*item.RunTimeTicks)
		if err != nil {
			return ResumeDecision{}, fmt.Errorf("decide resume: invalid runtime: %w", err)
		}
		if position >= runtime {
			return fromBeginning("saved position is at or beyond runtime"), nil
		}
		if runtime-position < policy.MinimumRemaining {
			return fromBeginning("saved position is near the end"), nil
		}
	}

	return ResumeDecision{
		Position: position, StartTimeTicks: ticks, Resume: true,
		Reason: "saved position is resumable",
	}, nil
}

func fromBeginning(reason string) ResumeDecision { return ResumeDecision{Reason: reason} }

func ticksToDuration(ticks int64) (time.Duration, error) {
	if ticks < 0 {
		return 0, errors.New("negative Jellyfin ticks")
	}
	if ticks > math.MaxInt64/nanosecondsPerJellyfinTick {
		return 0, errors.New("Jellyfin ticks exceed time.Duration range")
	}
	return time.Duration(ticks * nanosecondsPerJellyfinTick), nil
}

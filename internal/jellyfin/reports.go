package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type PlaybackState struct {
	ItemID        string
	MediaSourceID string
	PlaySessionID string
	Position      time.Duration
	Paused        bool
	PlayMethod    string
	Failed        bool
}

type playbackStartInfo struct {
	CanSeek       bool   `json:"CanSeek"`
	ItemID        string `json:"ItemId"`
	MediaSourceID string `json:"MediaSourceId,omitempty"`
	IsPaused      bool   `json:"IsPaused"`
	PositionTicks *int64 `json:"PositionTicks"`
	PlayMethod    string `json:"PlayMethod"`
	PlaySessionID string `json:"PlaySessionId,omitempty"`
}

type playbackProgressInfo playbackStartInfo

type playbackStopInfo struct {
	ItemID        string `json:"ItemId"`
	MediaSourceID string `json:"MediaSourceId,omitempty"`
	PositionTicks *int64 `json:"PositionTicks"`
	PlaySessionID string `json:"PlaySessionId,omitempty"`
	Failed        bool   `json:"Failed"`
}

func (c *Client) ReportPlaybackStart(ctx context.Context, state PlaybackState) error {
	if err := c.validatePlaybackState(state); err != nil {
		return fmt.Errorf("report playback start: %w", err)
	}
	ticks := durationToTicks(state.Position)
	body := playbackStartInfo{
		CanSeek: true, ItemID: state.ItemID, MediaSourceID: state.MediaSourceID,
		IsPaused: state.Paused, PositionTicks: &ticks, PlayMethod: state.PlayMethod,
		PlaySessionID: state.PlaySessionID,
	}
	if err := c.doJSON(ctx, http.MethodPost, "/Sessions/Playing", body, nil); err != nil {
		return fmt.Errorf("report playback start: %w", err)
	}
	return nil
}

func (c *Client) ReportPlaybackProgress(ctx context.Context, state PlaybackState) error {
	if err := c.validatePlaybackState(state); err != nil {
		return fmt.Errorf("report playback progress: %w", err)
	}
	ticks := durationToTicks(state.Position)
	body := playbackProgressInfo{
		CanSeek: true, ItemID: state.ItemID, MediaSourceID: state.MediaSourceID,
		IsPaused: state.Paused, PositionTicks: &ticks, PlayMethod: state.PlayMethod,
		PlaySessionID: state.PlaySessionID,
	}
	if err := c.doJSON(ctx, http.MethodPost, "/Sessions/Playing/Progress", body, nil); err != nil {
		return fmt.Errorf("report playback progress: %w", err)
	}
	return nil
}

func (c *Client) ReportPlaybackStopped(ctx context.Context, state PlaybackState) error {
	if err := c.validatePlaybackState(state); err != nil {
		return fmt.Errorf("report playback stopped: %w", err)
	}
	ticks := durationToTicks(state.Position)
	body := playbackStopInfo{
		ItemID: state.ItemID, MediaSourceID: state.MediaSourceID, PositionTicks: &ticks,
		PlaySessionID: state.PlaySessionID, Failed: state.Failed,
	}
	if err := c.doJSON(ctx, http.MethodPost, "/Sessions/Playing/Stopped", body, nil); err != nil {
		return fmt.Errorf("report playback stopped: %w", err)
	}
	return nil
}

func (c *Client) validatePlaybackState(state PlaybackState) error {
	if c.token == "" {
		return errors.New("access token is required")
	}
	if state.ItemID == "" {
		return errors.New("item ID is required")
	}
	if state.Position < 0 {
		return errors.New("position must not be negative")
	}
	switch state.PlayMethod {
	case "DirectPlay", "DirectStream", "Transcode":
		return nil
	default:
		return errors.New("play method must be DirectPlay, DirectStream, or Transcode")
	}
}

func durationToTicks(duration time.Duration) int64 { return duration.Nanoseconds() / 100 }

// Package playback selects media and orchestrates Jellyfin/player state.
package playback

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"jellycli/internal/jellyfin"
)

type Mode string

const (
	DirectPlay   Mode = "DirectPlay"
	DirectStream Mode = "DirectStream"
	Transcode    Mode = "Transcode"
)

type Plan struct {
	Mode            Mode
	ItemID          string
	MediaSourceID   string
	PlaySessionID   string
	Resource        string
	RequiredHeaders map[string]string
	RunTimeTicks    *int64
}

var ErrNoCompatibleMedia = errors.New("no compatible media source")

func BuildPlan(itemID string, info jellyfin.PlaybackInfoResponse) (Plan, error) {
	if itemID == "" {
		return Plan{}, errors.New("build playback plan: item ID is required")
	}
	for _, mode := range []Mode{DirectPlay, DirectStream, Transcode} {
		for _, source := range info.MediaSources {
			resource, ok := resourceFor(mode, itemID, info.PlaySessionID, source)
			if !ok {
				continue
			}
			return Plan{
				Mode: mode, ItemID: itemID, MediaSourceID: source.ID,
				PlaySessionID: info.PlaySessionID, Resource: resource,
				RequiredHeaders: cloneHeaders(source.RequiredHTTPHeaders), RunTimeTicks: source.RunTimeTicks,
			}, nil
		}
	}
	return Plan{}, fmt.Errorf("build playback plan: %w", ErrNoCompatibleMedia)
}

func resourceFor(mode Mode, itemID, playSessionID string, source jellyfin.MediaSource) (string, bool) {
	switch mode {
	case DirectPlay:
		if !source.SupportsDirectPlay || source.ID == "" {
			return "", false
		}
		query := url.Values{"static": {strconv.FormatBool(true)}, "mediaSourceId": {source.ID}}
		if playSessionID != "" {
			query.Set("playSessionId", playSessionID)
		}
		return "/Videos/" + itemID + "/stream?" + query.Encode(), true
	case DirectStream:
		return source.TranscodingURL, source.SupportsDirectStream && source.TranscodingURL != ""
	case Transcode:
		return source.TranscodingURL, source.SupportsTranscoding && source.TranscodingURL != ""
	default:
		return "", false
	}
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	copy := make(map[string]string, len(headers))
	for key, value := range headers {
		copy[key] = value
	}
	return copy
}

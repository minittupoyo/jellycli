package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type DirectPlayProfile struct {
	Container  string `json:"Container"`
	AudioCodec string `json:"AudioCodec,omitempty"`
	VideoCodec string `json:"VideoCodec,omitempty"`
	Type       string `json:"Type"`
}

type TranscodingProfile struct {
	Container        string `json:"Container"`
	Type             string `json:"Type"`
	VideoCodec       string `json:"VideoCodec"`
	AudioCodec       string `json:"AudioCodec"`
	Protocol         string `json:"Protocol"`
	Context          string `json:"Context"`
	MaxAudioChannels string `json:"MaxAudioChannels,omitempty"`
}

type DeviceProfile struct {
	Name                string               `json:"Name,omitempty"`
	MaxStreamingBitrate *int                 `json:"MaxStreamingBitrate,omitempty"`
	DirectPlayProfiles  []DirectPlayProfile  `json:"DirectPlayProfiles"`
	TranscodingProfiles []TranscodingProfile `json:"TranscodingProfiles"`
}

type PlaybackInfoOptions struct {
	StartTimeTicks      int64
	MaxStreamingBitrate int
}

type playbackInfoRequest struct {
	UserID               string        `json:"UserId"`
	StartTimeTicks       *int64        `json:"StartTimeTicks,omitempty"`
	MaxStreamingBitrate  *int          `json:"MaxStreamingBitrate,omitempty"`
	DeviceProfile        DeviceProfile `json:"DeviceProfile"`
	EnableDirectPlay     bool          `json:"EnableDirectPlay"`
	EnableDirectStream   bool          `json:"EnableDirectStream"`
	EnableTranscoding    bool          `json:"EnableTranscoding"`
	AllowVideoStreamCopy bool          `json:"AllowVideoStreamCopy"`
	AllowAudioStreamCopy bool          `json:"AllowAudioStreamCopy"`
}

type MediaSource struct {
	ID                         string            `json:"Id"`
	Name                       string            `json:"Name"`
	Path                       string            `json:"Path"`
	Protocol                   string            `json:"Protocol"`
	Container                  string            `json:"Container"`
	RunTimeTicks               *int64            `json:"RunTimeTicks"`
	SupportsDirectPlay         bool              `json:"SupportsDirectPlay"`
	SupportsDirectStream       bool              `json:"SupportsDirectStream"`
	SupportsTranscoding        bool              `json:"SupportsTranscoding"`
	RequiredHTTPHeaders        map[string]string `json:"RequiredHttpHeaders"`
	TranscodingURL             string            `json:"TranscodingUrl"`
	TranscodingSubProtocol     string            `json:"TranscodingSubProtocol"`
	TranscodingContainer       string            `json:"TranscodingContainer"`
	DefaultAudioStreamIndex    *int              `json:"DefaultAudioStreamIndex"`
	DefaultSubtitleStreamIndex *int              `json:"DefaultSubtitleStreamIndex"`
}

type PlaybackInfoResponse struct {
	MediaSources  []MediaSource `json:"MediaSources"`
	PlaySessionID string        `json:"PlaySessionId"`
	ErrorCode     string        `json:"ErrorCode"`
}

func (c *Client) PlaybackInfo(ctx context.Context, itemID, userID string, options PlaybackInfoOptions) (PlaybackInfoResponse, error) {
	if c.token == "" {
		return PlaybackInfoResponse{}, errors.New("get playback info: access token is required")
	}
	if itemID == "" || strings.ContainsAny(itemID, "/?#") {
		return PlaybackInfoResponse{}, errors.New("get playback info: valid item ID is required")
	}
	if userID == "" {
		return PlaybackInfoResponse{}, errors.New("get playback info: user ID is required")
	}
	if options.StartTimeTicks < 0 || options.MaxStreamingBitrate < 0 {
		return PlaybackInfoResponse{}, errors.New("get playback info: start time and bitrate must not be negative")
	}

	request := playbackInfoRequest{
		UserID: userID, DeviceProfile: MPVDeviceProfile(options.MaxStreamingBitrate),
		EnableDirectPlay: true, EnableDirectStream: true, EnableTranscoding: true,
		AllowVideoStreamCopy: true, AllowAudioStreamCopy: true,
	}
	if options.StartTimeTicks > 0 {
		request.StartTimeTicks = &options.StartTimeTicks
	}
	if options.MaxStreamingBitrate > 0 {
		request.MaxStreamingBitrate = &options.MaxStreamingBitrate
	}

	var response PlaybackInfoResponse
	if err := c.doJSON(ctx, http.MethodPost, "/Items/"+itemID+"/PlaybackInfo", request, &response); err != nil {
		return PlaybackInfoResponse{}, fmt.Errorf("get playback info: %w", err)
	}
	if response.ErrorCode != "" {
		return PlaybackInfoResponse{}, fmt.Errorf("get playback info: server reported %s", response.ErrorCode)
	}
	if len(response.MediaSources) == 0 {
		return PlaybackInfoResponse{}, errors.New("get playback info: server returned no media sources")
	}
	return response, nil
}

func MPVDeviceProfile(maxStreamingBitrate int) DeviceProfile {
	profile := DeviceProfile{
		Name: "jellycli mpv",
		DirectPlayProfiles: []DirectPlayProfile{{
			Container:  "mkv,webm,mp4,m4v,mov,avi,mpeg,mpg,m2ts,ts,flv,ogv,ogg,wmv,asf,3gp",
			AudioCodec: "aac,ac3,eac3,dts,dtshd,truehd,mp3,flac,opus,vorbis,pcm_s16le,pcm_s24le",
			VideoCodec: "h264,hevc,vp8,vp9,av1,mpeg1video,mpeg2video,mpeg4,vc1,wmv3,theora",
			Type:       "Video",
		}},
		TranscodingProfiles: []TranscodingProfile{{
			Container: "ts", Type: "Video", VideoCodec: "h264", AudioCodec: "aac,ac3",
			Protocol: "hls", Context: "Streaming", MaxAudioChannels: "8",
		}},
	}
	if maxStreamingBitrate > 0 {
		profile.MaxStreamingBitrate = &maxStreamingBitrate
	}
	return profile
}

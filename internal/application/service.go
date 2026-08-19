// Package application coordinates configuration and external services for CLI
// and TUI frontends.
package application

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"jellycli/internal/config"
	"jellycli/internal/jellyfin"
	"jellycli/internal/playback"
	"jellycli/internal/player"
)

type Service struct {
	store   *config.Store
	http    jellyfin.HTTPDoer
	client  string
	device  string
	version string
	player  player.Player
}

type HomeContent struct {
	ContinueWatching []jellyfin.Item
	NextUp           []jellyfin.Item
	RecentlyAdded    []jellyfin.Item
}

// WithPlayer configures media playback and returns the service.
func (s *Service) WithPlayer(p player.Player) *Service {
	s.player = p
	return s
}

// Login authenticates without persisting the password. Settings and the token
// are saved only after the server accepts the credentials.
func (s *Service) Login(ctx context.Context, serverURL, username, password string) error {
	settings := config.Settings{ServerURL: serverURL}
	normalized, err := settings.NormalizedServerURL()
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if username == "" {
		return errors.New("login: username is required")
	}
	deviceID, err := s.store.EnsureDeviceID()
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	client, err := jellyfin.NewClient(normalized, s.http, jellyfin.DeviceInfo{
		Client: s.client, Device: s.device, DeviceID: deviceID, Version: s.version,
	})
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	result, err := client.AuthenticateByName(ctx, username, password)
	password = ""
	if err != nil {
		return err
	}
	if err := s.store.SaveSettings(settings); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if err := s.store.SaveAuth(config.Auth{AccessToken: result.AccessToken, UserID: result.User.ID}); err != nil {
		_ = s.store.ClearAuth()
		return fmt.Errorf("login: %w", err)
	}
	return nil
}

// Logout attempts server-side revocation and always clears the local token.
func (s *Service) Logout(ctx context.Context) error {
	var remoteErr error
	settings, settingsErr := s.store.LoadSettings()
	state, stateErr := s.store.LoadState()
	if settingsErr == nil && stateErr == nil && state.Auth != nil && state.DeviceID != "" {
		serverURL, err := settings.NormalizedServerURL()
		if err == nil {
			client, createErr := jellyfin.NewClient(serverURL, s.http, jellyfin.DeviceInfo{
				Client: s.client, Device: s.device, DeviceID: state.DeviceID, Version: s.version,
			})
			if createErr == nil {
				remoteErr = client.WithToken(state.Auth.AccessToken).Logout(ctx)
			} else {
				remoteErr = createErr
			}
		} else {
			remoteErr = err
		}
	} else if settingsErr != nil && !errors.Is(settingsErr, fs.ErrNotExist) {
		remoteErr = settingsErr
	} else if stateErr != nil && !errors.Is(stateErr, fs.ErrNotExist) {
		remoteErr = stateErr
	}
	if err := s.store.ClearAuth(); err != nil {
		return errors.Join(remoteErr, fmt.Errorf("clear local login: %w", err))
	}
	if remoteErr != nil {
		return fmt.Errorf("logout from server (local login cleared): %w", remoteErr)
	}
	return nil
}

func NewService(store *config.Store, httpClient jellyfin.HTTPDoer, deviceName, version string) (*Service, error) {
	if store == nil || httpClient == nil {
		return nil, errors.New("create application service: config store and HTTP client are required")
	}
	if deviceName == "" {
		deviceName = "Unknown device"
	}
	if version == "" {
		version = "dev"
	}
	return &Service{store: store, http: httpClient, client: "jellycli", device: deviceName, version: version}, nil
}

// Libraries reconnects with persisted credentials, validates the token, and
// returns the currently visible Jellyfin views.
func (s *Service) Libraries(ctx context.Context) ([]jellyfin.Item, error) {
	client, userID, _, _, err := s.authenticated(ctx)
	if err != nil {
		return nil, err
	}
	page, err := client.UserViews(ctx, userID)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// Home loads the three video collections shown on the TUI home screen.
func (s *Service) Home(ctx context.Context) (HomeContent, error) {
	client, userID, _, _, err := s.authenticated(ctx)
	if err != nil {
		return HomeContent{}, err
	}
	resume, err := client.ResumeItems(ctx, userID, jellyfin.ResumeQuery{
		Page:         jellyfin.PageOptions{Limit: 24},
		IncludeTypes: []jellyfin.ItemKind{jellyfin.ItemKindMovie, jellyfin.ItemKindEpisode},
	})
	if err != nil {
		return HomeContent{}, fmt.Errorf("load continue watching: %w", err)
	}
	next, err := client.NextUp(ctx, userID, jellyfin.NextUpQuery{Page: jellyfin.PageOptions{Limit: 24}})
	if err != nil {
		return HomeContent{}, fmt.Errorf("load next up: %w", err)
	}
	latest, err := client.Latest(ctx, userID, jellyfin.LatestQuery{
		Limit: 24, IncludeTypes: []jellyfin.ItemKind{jellyfin.ItemKindMovie, jellyfin.ItemKindEpisode},
	})
	if err != nil {
		return HomeContent{}, fmt.Errorf("load recently added: %w", err)
	}
	return HomeContent{ContinueWatching: resume.Items, NextUp: next.Items, RecentlyAdded: latest}, nil
}

// Browse returns the direct children appropriate for a library, series, or
// season. Callers choose the desired item kinds; URL construction stays here.
func (s *Service) Browse(ctx context.Context, parentID string, kinds []jellyfin.ItemKind) ([]jellyfin.Item, error) {
	if parentID == "" {
		return nil, errors.New("browse: parent ID is required")
	}
	client, userID, _, _, err := s.authenticated(ctx)
	if err != nil {
		return nil, err
	}
	page, err := client.Items(ctx, userID, jellyfin.ItemsQuery{
		Page: jellyfin.PageOptions{Limit: 500}, ParentID: parentID,
		IncludeTypes: kinds, Recursive: false, SortBy: []string{"SortName"}, SortOrder: "Ascending",
	})
	if err != nil {
		return nil, fmt.Errorf("browse: %w", err)
	}
	return page.Items, nil
}

// Search returns playable video results matching term.
func (s *Service) Search(ctx context.Context, term string) ([]jellyfin.Item, error) {
	if term == "" {
		return nil, errors.New("search term is required")
	}
	client, userID, _, _, err := s.authenticated(ctx)
	if err != nil {
		return nil, err
	}
	page, err := client.Items(ctx, userID, jellyfin.ItemsQuery{
		Page: jellyfin.PageOptions{Limit: 100}, SearchTerm: term, Recursive: true,
		IncludeTypes: []jellyfin.ItemKind{
			jellyfin.ItemKindMovie, jellyfin.ItemKindSeries,
			jellyfin.ItemKindEpisode, jellyfin.ItemKindVideo,
		},
		SortBy: []string{"SortName"}, SortOrder: "Ascending",
	})
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return page.Items, nil
}

// Play negotiates an item, starts the player, and synchronizes its state until
// playback exits or ctx is canceled.
func (s *Service) Play(ctx context.Context, itemID string) error {
	if itemID == "" {
		return errors.New("play: item ID is required")
	}
	if s.player == nil {
		return errors.New("play: player is unavailable")
	}
	client, userID, serverURL, token, err := s.authenticated(ctx)
	if err != nil {
		return err
	}
	page, err := client.Items(ctx, userID, jellyfin.ItemsQuery{
		Page: jellyfin.PageOptions{Limit: 1}, ItemIDs: []string{itemID}, Recursive: true,
	})
	if err != nil {
		return fmt.Errorf("play: load item: %w", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != itemID {
		return errors.New("play: item was not found")
	}
	item := page.Items[0]
	resume, err := playback.DecideResume(item, playback.DefaultResumePolicy())
	if err != nil {
		return fmt.Errorf("play: %w", err)
	}
	info, err := client.PlaybackInfo(ctx, itemID, userID, resume.PlaybackInfoOptions(0))
	if err != nil {
		return fmt.Errorf("play: %w", err)
	}
	plan, err := playback.BuildPlan(itemID, info)
	if err != nil {
		return fmt.Errorf("play: %w", err)
	}
	media, err := plan.Media(serverURL, token, item.Name, resume.Position)
	if err != nil {
		return fmt.Errorf("play: %w", err)
	}
	session, err := s.player.Start(ctx, media)
	if err != nil {
		return err
	}
	state := jellyfin.PlaybackState{
		ItemID: itemID, MediaSourceID: plan.MediaSourceID, PlaySessionID: plan.PlaySessionID,
		Position: resume.Position, PlayMethod: string(plan.Mode),
	}
	return playback.NewSynchronizer(client).Run(ctx, session, state)
}

func (s *Service) authenticated(ctx context.Context) (*jellyfin.Client, string, string, string, error) {
	settings, err := s.store.LoadSettings()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, "", "", "", errors.New("not configured: save a Jellyfin server URL first")
		}
		return nil, "", "", "", fmt.Errorf("load configuration: %w", err)
	}
	serverURL, err := settings.NormalizedServerURL()
	if err != nil {
		return nil, "", "", "", fmt.Errorf("load configuration: %w", err)
	}
	state, err := s.store.LoadState()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, "", "", "", errors.New("not logged in: authentication state was not found")
		}
		return nil, "", "", "", fmt.Errorf("load authentication state: %w", err)
	}
	if state.DeviceID == "" || state.Auth == nil {
		return nil, "", "", "", errors.New("not logged in: run the login command first")
	}

	client, err := jellyfin.NewClient(serverURL, s.http, jellyfin.DeviceInfo{
		Client: s.client, Device: s.device, DeviceID: state.DeviceID, Version: s.version,
	})
	if err != nil {
		return nil, "", "", "", err
	}
	client = client.WithToken(state.Auth.AccessToken)
	user, err := client.CurrentUser(ctx)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("validate saved login: %w", err)
	}
	if user.ID != state.Auth.UserID {
		return nil, "", "", "", errors.New("validate saved login: token user does not match saved user")
	}
	return client, user.ID, serverURL, state.Auth.AccessToken, nil
}

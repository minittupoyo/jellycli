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
)

type Service struct {
	store   *config.Store
	http    jellyfin.HTTPDoer
	client  string
	device  string
	version string
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
	settings, err := s.store.LoadSettings()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errors.New("not configured: save a Jellyfin server URL first")
		}
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	serverURL, err := settings.NormalizedServerURL()
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	state, err := s.store.LoadState()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errors.New("not logged in: authentication state was not found")
		}
		return nil, fmt.Errorf("load authentication state: %w", err)
	}
	if state.DeviceID == "" || state.Auth == nil {
		return nil, errors.New("not logged in: run the login command first")
	}

	client, err := jellyfin.NewClient(serverURL, s.http, jellyfin.DeviceInfo{
		Client: s.client, Device: s.device, DeviceID: state.DeviceID, Version: s.version,
	})
	if err != nil {
		return nil, err
	}
	client = client.WithToken(state.Auth.AccessToken)
	user, err := client.CurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("validate saved login: %w", err)
	}
	if user.ID != state.Auth.UserID {
		return nil, errors.New("validate saved login: token user does not match saved user")
	}
	page, err := client.UserViews(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

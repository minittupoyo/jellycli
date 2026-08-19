// Package jellyfin implements the Jellyfin REST API boundary.
package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxResponseSize = 4 << 20

var (
	ErrAuthentication = errors.New("Jellyfin authentication failed")
	ErrNetwork        = errors.New("Jellyfin network request failed")
)

// HTTPDoer is implemented by *http.Client and can be replaced in tests.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// DeviceInfo identifies this client session to Jellyfin.
type DeviceInfo struct {
	Client   string
	Device   string
	DeviceID string
	Version  string
}

func (d DeviceInfo) validate() error {
	if d.Client == "" || d.Device == "" || d.DeviceID == "" || d.Version == "" {
		return errors.New("client, device, device ID, and version are required")
	}
	return nil
}

// Client is safe for concurrent use when its HTTPDoer is safe for concurrent use.
// Authentication is immutable; WithToken returns a copy.
type Client struct {
	baseURL *url.URL
	http    HTTPDoer
	device  DeviceInfo
	token   string
}

func NewClient(serverURL string, httpClient HTTPDoer, device DeviceInfo) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(serverURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("create Jellyfin client: parse server URL: %w", err)
	}
	if (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return nil, errors.New("create Jellyfin client: server URL must be an absolute http or https URL")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("create Jellyfin client: server URL must not contain credentials, query, or fragment")
	}
	if httpClient == nil {
		return nil, errors.New("create Jellyfin client: HTTP client is required")
	}
	if err := device.validate(); err != nil {
		return nil, fmt.Errorf("create Jellyfin client: %w", err)
	}
	return &Client{baseURL: baseURL, http: httpClient, device: device}, nil
}

// WithToken returns a client that sends token authentication. The original is
// unchanged, which avoids races between login and in-flight requests.
func (c *Client) WithToken(token string) *Client {
	copy := *c
	copy.token = token
	return &copy
}

type authenticateRequest struct {
	Username string `json:"Username"`
	Password string `json:"Pw"`
}

// User is the subset of UserDto needed by the application layer.
type User struct {
	ID       string `json:"Id"`
	Name     string `json:"Name"`
	ServerID string `json:"ServerId"`
}

type SessionInfo struct {
	ID string `json:"Id"`
}

// AuthenticationResult is returned by AuthenticateByName.
type AuthenticationResult struct {
	User        User        `json:"User"`
	SessionInfo SessionInfo `json:"SessionInfo"`
	AccessToken string      `json:"AccessToken"`
	ServerID    string      `json:"ServerId"`
}

// AuthenticateByName exchanges a password for a user access token. The caller
// must discard the password after this call and persist only the returned token.
func (c *Client) AuthenticateByName(ctx context.Context, username, password string) (AuthenticationResult, error) {
	if username == "" {
		return AuthenticationResult{}, errors.New("authenticate: username is required")
	}
	var result AuthenticationResult
	err := c.doJSON(ctx, http.MethodPost, "/Users/AuthenticateByName", authenticateRequest{
		Username: username,
		Password: password,
	}, &result)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			return AuthenticationResult{}, fmt.Errorf("%w: %v", ErrAuthentication, apiErr)
		}
		return AuthenticationResult{}, fmt.Errorf("authenticate: %w", err)
	}
	if result.AccessToken == "" || result.User.ID == "" {
		return AuthenticationResult{}, errors.New("authenticate: server response omitted access token or user ID")
	}
	return result, nil
}

// CurrentUser validates the configured token and returns its user.
func (c *Client) CurrentUser(ctx context.Context) (User, error) {
	if c.token == "" {
		return User{}, errors.New("get current user: access token is required")
	}
	var user User
	if err := c.doJSON(ctx, http.MethodGet, "/Users/Me", nil, &user); err != nil {
		return User{}, fmt.Errorf("get current user: %w", err)
	}
	if user.ID == "" {
		return User{}, errors.New("get current user: server response omitted user ID")
	}
	return user, nil
}

// Logout invalidates the current access token on the server.
func (c *Client) Logout(ctx context.Context) error {
	if c.token == "" {
		return errors.New("logout: access token is required")
	}
	if err := c.doJSON(ctx, http.MethodPost, "/Sessions/Logout", nil, nil); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}

// APIError represents a non-2xx response without including request secrets or
// echoing an untrusted response body.
type APIError struct {
	StatusCode int
	Status     string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Jellyfin API returned %s", e.Status)
}

// Is lets callers consistently recognize an expired/invalid token as an
// authentication error while retaining the HTTP status through errors.As.
func (e *APIError) Is(target error) bool {
	return target == ErrAuthentication &&
		(e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden)
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, requestBody, responseBody any) error {
	return c.doJSONQuery(ctx, method, endpoint, nil, requestBody, responseBody)
}

func (c *Client) doJSONQuery(ctx context.Context, method, endpoint string, query url.Values, requestBody, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	requestURL := *c.baseURL
	requestURL.Path = strings.TrimRight(c.baseURL.Path, "/") + endpoint
	requestURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", c.authorizationHeader())
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNetwork, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseSize))
		return &APIError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	if responseBody == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseSize))
		return nil
	}

	limited := io.LimitReader(resp.Body, maxResponseSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(data) > maxResponseSize {
		return fmt.Errorf("read response: body exceeds %d bytes", maxResponseSize)
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) authorizationHeader() string {
	values := []string{
		"Client=\"" + encodeAuthValue(c.device.Client) + "\"",
		"Device=\"" + encodeAuthValue(c.device.Device) + "\"",
		"DeviceId=\"" + encodeAuthValue(c.device.DeviceID) + "\"",
		"Version=\"" + encodeAuthValue(c.device.Version) + "\"",
	}
	if c.token != "" {
		values = append([]string{"Token=\"" + encodeAuthValue(c.token) + "\""}, values...)
	}
	return "MediaBrowser " + strings.Join(values, ", ")
}

func encodeAuthValue(value string) string {
	return url.QueryEscape(value)
}

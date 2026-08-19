package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := NewClient(serverURL, http.DefaultClient, DeviceInfo{
		Client:   "jellycli",
		Device:   "test device",
		DeviceID: "device-id",
		Version:  "dev",
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestAuthenticateByName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/base/Users/AuthenticateByName" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != `MediaBrowser Client="jellycli", Device="test+device", DeviceId="device-id", Version="dev"` {
			t.Errorf("Authorization = %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["Username"] != "alice" || body["Pw"] != "password" {
			t.Errorf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"User":{"Id":"user-id","Name":"alice"},"SessionInfo":{"Id":"session-id"},"AccessToken":"secret-token","ServerId":"server-id"}`))
	}))
	defer server.Close()

	got, err := newTestClient(t, server.URL+"/base/").AuthenticateByName(context.Background(), "alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "secret-token" || got.User.ID != "user-id" || got.SessionInfo.ID != "session-id" {
		t.Fatalf("AuthenticateByName() = %#v", got)
	}
}

func TestAuthenticationFailureIsClassifiedAndRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `password=very-secret`, http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL).AuthenticateByName(context.Background(), "alice", "very-secret")
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
	if strings.Contains(err.Error(), "very-secret") {
		t.Fatalf("error leaked a secret: %v", err)
	}
}

func TestCurrentUserAndLogoutSendToken(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("Authorization"); !strings.Contains(got, `Token="secret-token"`) {
			t.Errorf("Authorization = %q, want token", got)
		}
		switch r.URL.Path {
		case "/Users/Me":
			_, _ = w.Write([]byte(`{"Id":"user-id","Name":"alice"}`))
		case "/Sessions/Logout":
			if r.Method != http.MethodPost {
				t.Errorf("logout method = %s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL).WithToken("secret-token")
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "user-id" {
		t.Fatalf("CurrentUser() = %#v", user)
	}
	if err := client.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestClientRequiresTokenForAuthenticatedCalls(t *testing.T) {
	client := newTestClient(t, "https://media.example.test")
	if _, err := client.CurrentUser(context.Background()); err == nil {
		t.Fatal("CurrentUser() error = nil, want error")
	}
	if err := client.Logout(context.Background()); err == nil {
		t.Fatal("Logout() error = nil, want error")
	}
}

func TestAPIErrorDoesNotExposeTokenOrResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `token=secret-token`, http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL).WithToken("secret-token").CurrentUser(context.Background())
	if err == nil {
		t.Fatal("CurrentUser() error = nil, want error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error leaked token: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("error = %v, want APIError 500", err)
	}
}

func TestExpiredTokenIsAuthenticationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL).WithToken("expired-token").CurrentUser(context.Background())
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("error = %v, want ErrAuthentication", err)
	}
}

type failingHTTPClient struct{ err error }

func (f failingHTTPClient) Do(*http.Request) (*http.Response, error) { return nil, f.err }

func TestNetworkFailureIsClassifiedAndWrapped(t *testing.T) {
	cause := errors.New("connection reset")
	client, err := NewClient("https://media.example.test", failingHTTPClient{err: cause}, DeviceInfo{
		Client: "client", Device: "device", DeviceID: "id", Version: "dev",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WithToken("token").CurrentUser(context.Background())
	if !errors.Is(err, ErrNetwork) || !errors.Is(err, cause) {
		t.Fatalf("error = %v", err)
	}
}

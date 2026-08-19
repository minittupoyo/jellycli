package application

import (
	"errors"
	"strings"
	"testing"

	"jellycli/internal/jellyfin"
)

func TestUserErrorClassifiesAuthenticationAndNetwork(t *testing.T) {
	if got := UserError("login", jellyfin.ErrAuthentication); !strings.Contains(got, "username and password") {
		t.Fatalf("login error = %q", got)
	}
	if got := UserError("libraries", jellyfin.ErrAuthentication); !strings.Contains(got, "login") {
		t.Fatalf("expired error = %q", got)
	}
	if got := UserError("search", fmtWrap(jellyfin.ErrNetwork)); !strings.Contains(got, "network connection") {
		t.Fatalf("network error = %q", got)
	}
	if got := UserError("play", errors.New("specific failure")); got != "specific failure" {
		t.Fatalf("generic error = %q", got)
	}
}

func fmtWrap(err error) error { return errors.Join(errors.New("outer"), err) }

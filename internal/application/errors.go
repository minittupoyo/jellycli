package application

import (
	"context"
	"errors"

	"jellycli/internal/jellyfin"
)

// UserError translates technical failures into stable, actionable terminal
// messages. The original error remains available to the debug logger.
func UserError(operation string, err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "operation canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, jellyfin.ErrNetwork) {
		return "cannot reach the Jellyfin server; check the URL and network connection"
	}
	if errors.Is(err, jellyfin.ErrAuthentication) {
		if operation == "login" {
			return "authentication failed; check the username and password"
		}
		return "saved login is no longer valid; run 'jellycli login' again"
	}
	return err.Error()
}

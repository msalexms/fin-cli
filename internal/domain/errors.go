package domain

import "errors"

// Sentinel errors wrapped by providers and services. Callers match them with
// errors.Is to pick exit codes and user-facing messages.
var (
	ErrNotFound     = errors.New("not found")
	ErrRateLimited  = errors.New("rate limited")
	ErrUnauthorized = errors.New("unauthorized")
	ErrUnavailable  = errors.New("provider unavailable")
	ErrNetwork      = errors.New("network error")
	ErrNoData       = errors.New("no data")
	ErrNoAPIKey     = errors.New("api key not configured")
	ErrInvalidInput = errors.New("invalid input")
)

// IsTransient reports whether err is worth retrying later or with another
// provider (connectivity, rate limits, outages).
func IsTransient(err error) bool {
	return errors.Is(err, ErrNetwork) || errors.Is(err, ErrUnavailable) || errors.Is(err, ErrRateLimited)
}

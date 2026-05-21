package domain

import "errors"

// Sentinel errors exposed by the domain and wrapped by infrastructure/providers.
// The UI can match via errors.Is to produce actionable messages.
var (
	// ErrNotFound: the ticker/ISIN is unknown to the provider.
	ErrNotFound = errors.New("not found")

	// ErrRateLimited: the provider rejected the call due to rate limits.
	ErrRateLimited = errors.New("rate limited")

	// ErrUnauthorized: the API key is missing, invalid, or lacks permission.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrUnavailable: the provider is temporarily unavailable (5xx, maintenance).
	ErrUnavailable = errors.New("provider unavailable")

	// ErrNetwork: connectivity failure (DNS, timeout, TLS).
	ErrNetwork = errors.New("network error")

	// ErrPartialData: the call succeeded but some expected fields are missing.
	ErrPartialData = errors.New("partial data")

	// ErrNoAPIKey: operation requires a configured API key.
	ErrNoAPIKey = errors.New("api key not configured")

	// ErrInvalidInput: user-provided input failed validation.
	ErrInvalidInput = errors.New("invalid input")

	// ErrCacheMiss: cache lookup miss (not an error per se, used by callers).
	ErrCacheMiss = errors.New("cache miss")
)

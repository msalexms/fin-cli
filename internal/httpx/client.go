// Package httpx provides a shared HTTP client with sane defaults: timeouts,
// bounded retries for idempotent requests, and User-Agent injection.
package httpx

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"time"

	"fin-cli/internal/version"
)

// Client wraps http.Client with retry logic for GET/HEAD requests.
type Client struct {
	http    *http.Client
	retries int
	ua      string
}

// New returns a Client with:
//   - total request timeout: 10s
//   - dial timeout: 3s
//   - max 3 attempts on transient errors and 5xx for idempotent methods
func New() *Client {
	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   4,
		ForceAttemptHTTP2:     true,
	}
	return &Client{
		http:    &http.Client{Transport: tr, Timeout: 10 * time.Second},
		retries: 3,
		ua:      version.UserAgent(),
	}
}

// Do executes req with retries for idempotent methods (GET/HEAD) on transient
// errors (network failures) and on 5xx responses. Non-idempotent methods are
// executed exactly once.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", c.ua)
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	idempotent := req.Method == http.MethodGet || req.Method == http.MethodHead

	var lastErr error
	attempts := 1
	if idempotent {
		attempts = c.retries
	}

	for i := 0; i < attempts; i++ {
		if i > 0 {
			if err := sleepJitter(req.Context(), i); err != nil {
				return nil, err
			}
		}

		resp, err := c.http.Do(req)
		if err != nil {
			if !shouldRetryErr(err) || !idempotent {
				return nil, err
			}
			lastErr = err
			continue
		}

		if resp.StatusCode >= 500 && resp.StatusCode < 600 && idempotent && i < attempts-1 {
			// Consume and close body to allow connection reuse, then retry.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			lastErr = &HTTPError{StatusCode: resp.StatusCode}
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

// HTTPError describes a non-2xx response. Providers use it to classify errors.
type HTTPError struct {
	StatusCode int
	Body       string // optional, populated by helpers that read the body
}

func (e *HTTPError) Error() string {
	return http.StatusText(e.StatusCode)
}

// shouldRetryErr returns true for transient network/timeout errors.
func shouldRetryErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		if ue.Timeout() {
			return true
		}
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	// Treat any other non-context error as transient: io.EOF, reset, DNS.
	return true
}

func sleepJitter(ctx context.Context, attempt int) error {
	base := time.Duration(250*(1<<uint(attempt-1))) * time.Millisecond
	if base > 2*time.Second {
		base = 2 * time.Second
	}
	jitter := time.Duration(rand.Int63n(int64(base / 2)))
	d := base + jitter
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

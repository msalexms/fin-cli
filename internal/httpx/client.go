// Package httpx is the HTTP client shared by every provider: timeouts,
// bounded retries for idempotent requests, a fixed User-Agent, and helpers
// that turn transport failures and status codes into domain errors without
// leaking request URLs (which may carry API keys) into messages.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/version"
)

const maxAttempts = 3

// Client wraps http.Client with retry logic for GET/HEAD requests.
type Client struct {
	http *http.Client
	ua   string
}

// New returns a Client with a 10s overall timeout per attempt.
func New() *Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConnsPerHost:   4,
		ForceAttemptHTTP2:     true,
	}
	return &Client{
		http: &http.Client{Transport: tr, Timeout: 10 * time.Second},
		ua:   version.UserAgent(),
	}
}

// Do executes req. Idempotent requests are retried with jittered backoff on
// transport errors and 5xx responses. Transport errors are returned wrapped
// in domain.ErrNetwork with the URL stripped.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.ua)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	attempts := 1
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		attempts = maxAttempts
	}

	var lastErr error
	for i := range attempts {
		if i > 0 {
			if err := backoff(req.Context(), i); err != nil {
				return nil, err
			}
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if ctxErr := req.Context().Err(); ctxErr != nil {
				return nil, ctxErr
			}
			lastErr = fmt.Errorf("%w: %v", domain.ErrNetwork, stripURL(err))
			continue
		}
		if resp.StatusCode >= 500 && i < attempts-1 {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("%w: http %d", domain.ErrUnavailable, resp.StatusCode)
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

// GetJSON performs a GET and returns the body (capped at limit bytes) after
// mapping non-2xx statuses to domain errors prefixed with provider.
func (c *Client) GetJSON(ctx context.Context, provider, rawURL string, header http.Header, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", provider, stripURL(err))
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", provider, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("%s: %w: read body: %v", provider, domain.ErrNetwork, stripURL(err))
	}
	if err := StatusError(provider, resp.StatusCode); err != nil {
		return nil, err
	}
	return body, nil
}

// StatusError maps an HTTP status code to a domain error, or nil for 2xx.
func StatusError(provider string, status int) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return fmt.Errorf("%s: %w (http %d)", provider, domain.ErrUnauthorized, status)
	case status == http.StatusNotFound:
		return fmt.Errorf("%s: %w", provider, domain.ErrNotFound)
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%s: %w", provider, domain.ErrRateLimited)
	case status >= 500:
		return fmt.Errorf("%s: %w (http %d)", provider, domain.ErrUnavailable, status)
	default:
		return fmt.Errorf("%s: unexpected http %d", provider, status)
	}
}

// stripURL removes the request URL from *url.Error so query-string API keys
// never reach logs or the screen.
func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}

func backoff(ctx context.Context, attempt int) error {
	base := min(time.Duration(250<<(attempt-1))*time.Millisecond, 2*time.Second)
	d := base + rand.N(base/2)
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

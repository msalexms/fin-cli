package finnhub

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"fin-cli/internal/domain"
)

// classify inspects the SDK error (and optional *http.Response) and maps it to
// a domain sentinel so the UI can render actionable messages.
func classify(resp *http.Response, err error) error {
	if err == nil {
		return nil
	}
	if resp != nil {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("%w: finnhub rejected the api key", domain.ErrUnauthorized)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%w: finnhub quota exceeded", domain.ErrRateLimited)
		case http.StatusNotFound:
			return fmt.Errorf("%w: not found on finnhub", domain.ErrNotFound)
		}
		if resp.StatusCode >= 500 {
			return fmt.Errorf("%w: finnhub http %d", domain.ErrUnavailable, resp.StatusCode)
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("finnhub http %d: %v", resp.StatusCode, err)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: deadline exceeded", domain.ErrNetwork)
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	return fmt.Errorf("%w: %v", domain.ErrNetwork, err)
}

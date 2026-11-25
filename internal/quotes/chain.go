package quotes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/msalexms/fin-cli/internal/domain"
)

// ChainError reports that every provider in a chain failed. Error lists each
// provider's failure; Unwrap exposes the most relevant one so callers can
// still use errors.Is with the domain sentinels.
type ChainError struct {
	Primary error
	All     []error
}

func (e *ChainError) Error() string {
	msgs := make([]string, len(e.All))
	for i, err := range e.All {
		msgs[i] = err.Error()
	}
	return strings.Join(msgs, "; ")
}

func (e *ChainError) Unwrap() error { return e.Primary }

// runChain calls fn on each provider in order and returns the first success.
// Every failure moves on to the next provider: a symbol unknown to one source
// (e.g. a European ticker on Finnhub's free tier) is often known to the next.
func runChain[T any](ctx context.Context, providers []domain.Provider, fn func(domain.Provider) (T, error)) (T, error) {
	var zero T
	var errs []error
	for _, p := range providers {
		v, err := fn(p)
		if err == nil {
			return v, nil
		}
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		errs = append(errs, err)
	}
	return zero, chainError(errs)
}

// chainError picks the error that best explains the failure, in priority:
// transient (so stale cache can be served) > bad key > not found > no data >
// anything else > missing key.
func chainError(errs []error) error {
	var configured []error
	for _, err := range errs {
		if !errors.Is(err, domain.ErrNoAPIKey) {
			configured = append(configured, err)
		}
	}
	if len(configured) == 0 {
		return fmt.Errorf("%w: every provider in the chain needs a key; set one with `fin-cli config set` or add \"yahoo\"",
			domain.ErrNoAPIKey)
	}
	if len(configured) == 1 {
		return configured[0]
	}
	rank := func(err error) int {
		switch {
		case domain.IsTransient(err):
			return 0
		case errors.Is(err, domain.ErrUnauthorized):
			return 1
		case errors.Is(err, domain.ErrNotFound):
			return 2
		case errors.Is(err, domain.ErrNoData):
			return 3
		default:
			return 4
		}
	}
	primary := configured[0]
	for _, err := range configured[1:] {
		if rank(err) < rank(primary) {
			primary = err
		}
	}
	return &ChainError{Primary: primary, All: configured}
}

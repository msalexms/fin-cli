// Package watchlist persists the user's tracked tickers to a TOML file.
// Every mutation is a locked read-modify-write, so concurrent fin-cli
// processes (TUI + CLI) never lose each other's changes.
package watchlist

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/pelletier/go-toml/v2"

	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/fsutil"
)

const schemaVersion = 1

var (
	// ErrAlreadyPresent is returned by Add when the ticker is in the list.
	ErrAlreadyPresent = errors.New("already in watchlist")
	// ErrNotPresent is returned by Remove/Move when the ticker is absent.
	ErrNotPresent = errors.New("not in watchlist")
)

type file struct {
	SchemaVersion int      `toml:"schema_version"`
	Tickers       []string `toml:"tickers"`
}

// Store manages the watchlist file.
type Store struct {
	path string
}

// New returns a Store that reads and writes path.
func New(path string) *Store { return &Store{path: path} }

// Path returns the file location.
func (s *Store) Path() string { return s.path }

// Load returns the tickers in order. A missing file is an empty watchlist.
func (s *Store) Load() ([]domain.Ticker, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var f file
	if err := toml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.path, err)
	}
	out := make([]domain.Ticker, 0, len(f.Tickers))
	for _, raw := range f.Tickers {
		t, err := domain.ParseTicker(raw)
		if err != nil || slices.Contains(out, t) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// Add appends t. Returns ErrAlreadyPresent if it is already tracked.
func (s *Store) Add(t domain.Ticker) error {
	return s.update(func(cur []domain.Ticker) ([]domain.Ticker, error) {
		if slices.Contains(cur, t) {
			return nil, ErrAlreadyPresent
		}
		return append(cur, t), nil
	})
}

// Remove drops t. Returns ErrNotPresent if it is not tracked.
func (s *Store) Remove(t domain.Ticker) error {
	return s.update(func(cur []domain.Ticker) ([]domain.Ticker, error) {
		i := slices.Index(cur, t)
		if i < 0 {
			return nil, ErrNotPresent
		}
		return slices.Delete(cur, i, i+1), nil
	})
}

// Move shifts t by delta positions (negative = up), clamped to the list.
func (s *Store) Move(t domain.Ticker, delta int) error {
	return s.update(func(cur []domain.Ticker) ([]domain.Ticker, error) {
		i := slices.Index(cur, t)
		if i < 0 {
			return nil, ErrNotPresent
		}
		j := min(max(i+delta, 0), len(cur)-1)
		cur = slices.Delete(cur, i, i+1)
		return slices.Insert(cur, j, t), nil
	})
}

func (s *Store) update(fn func([]domain.Ticker) ([]domain.Ticker, error)) error {
	unlock, err := fsutil.Lock(s.path)
	if err != nil {
		return err
	}
	defer unlock()

	cur, err := s.Load()
	if err != nil {
		return err
	}
	next, err := fn(cur)
	if err != nil {
		return err
	}
	raw := make([]string, len(next))
	for i, t := range next {
		raw[i] = string(t)
	}
	b, err := toml.Marshal(file{SchemaVersion: schemaVersion, Tickers: raw})
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(s.path, b)
}

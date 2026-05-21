// Package watchlist persists the user's tracked tickers to a TOML file.
package watchlist

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/sys/unix"

	"fin-cli/internal/domain"
)

// CurrentSchemaVersion for watchlist.toml.
const CurrentSchemaVersion = 1

// file is the on-disk representation.
type file struct {
	SchemaVersion int      `toml:"schema_version"`
	Tickers       []string `toml:"tickers"`
}

// Store manages the watchlist file.
type Store struct {
	path string
}

// New returns a Store that reads/writes path.
func New(path string) *Store { return &Store{path: path} }

// Load returns the tickers, preserving order. An empty watchlist is valid.
func (s *Store) Load() ([]domain.Ticker, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var f file
	if err := toml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse watchlist: %w", err)
	}
	out := make([]domain.Ticker, 0, len(f.Tickers))
	for _, t := range f.Tickers {
		t = strings.TrimSpace(strings.ToUpper(t))
		if t != "" {
			out = append(out, domain.Ticker(t))
		}
	}
	return out, nil
}

// Save writes tickers atomically under flock.
func (s *Store) Save(tickers []domain.Ticker) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	unlock, err := flock(s.path)
	if err != nil {
		return err
	}
	defer unlock()

	raw := make([]string, 0, len(tickers))
	for _, t := range tickers {
		raw = append(raw, string(t))
	}
	f := file{SchemaVersion: CurrentSchemaVersion, Tickers: raw}
	b, err := toml.Marshal(f)
	if err != nil {
		return err
	}
	return atomicWrite(s.path, b)
}

// Add appends t if not present. Returns ErrAlreadyPresent if it is.
func (s *Store) Add(t domain.Ticker) error {
	t = domain.Ticker(strings.TrimSpace(strings.ToUpper(string(t))))
	if t == "" {
		return fmt.Errorf("%w: empty ticker", domain.ErrInvalidInput)
	}
	cur, err := s.Load()
	if err != nil {
		return err
	}
	for _, x := range cur {
		if x == t {
			return ErrAlreadyPresent
		}
	}
	cur = append(cur, t)
	return s.Save(cur)
}

// Remove drops t. Returns ErrNotPresent if it is not in the list.
func (s *Store) Remove(t domain.Ticker) error {
	t = domain.Ticker(strings.TrimSpace(strings.ToUpper(string(t))))
	cur, err := s.Load()
	if err != nil {
		return err
	}
	idx := -1
	for i, x := range cur {
		if x == t {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotPresent
	}
	cur = append(cur[:idx], cur[idx+1:]...)
	return s.Save(cur)
}

// ErrAlreadyPresent is returned by Add when the ticker is in the list.
var ErrAlreadyPresent = errors.New("ticker already in watchlist")

// ErrNotPresent is returned by Remove when the ticker is absent.
var ErrNotPresent = errors.New("ticker not in watchlist")

// --- internal helpers duplicated here to keep watchlist free of config deps ---

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() { _ = os.Remove(tmp) }

	if _, err := f.Write(data); err != nil {
		f.Close()
		cleanup()
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return err
	}
	return os.Rename(tmp, path)
}

func flock(path string) (func(), error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
		_ = os.Remove(path + ".lock")
	}, nil
}

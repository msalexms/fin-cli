// Package cache is a tiny file-per-key cache on disk with schema versioning.
//
// Each entry is stored as a JSON file whose mtime indicates freshness.
// Callers choose the TTL when reading, so the same cache can serve multiple
// consumers with different staleness tolerances.
package cache

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fin-cli/internal/domain"
)

// SchemaVersion is bumped when the on-disk entry format changes incompatibly.
const SchemaVersion = 1

// Entry is the on-disk wrapper.
type Entry[T any] struct {
	Schema    int       `json:"schema"`
	FetchedAt time.Time `json:"fetched_at"`
	Value     T         `json:"value"`
}

// Store is a file-per-key disk cache rooted at Dir.
type Store struct {
	Dir string
}

// New returns a Store; it does not create Dir (caller handles EnsureDirs).
func New(dir string) *Store { return &Store{Dir: dir} }

// Get reads key. Returns domain.ErrCacheMiss if the file does not exist or
// the schema version is unknown. It does not enforce TTL; callers inspect
// FetchedAt and decide.
func Get[T any](s *Store, key string) (Entry[T], error) {
	var zero Entry[T]
	p := s.pathFor(key)
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return zero, domain.ErrCacheMiss
		}
		return zero, err
	}
	var e Entry[T]
	if err := json.Unmarshal(b, &e); err != nil {
		// Corrupt entry: treat as miss and remove.
		_ = os.Remove(p)
		return zero, domain.ErrCacheMiss
	}
	if e.Schema != SchemaVersion {
		_ = os.Remove(p)
		return zero, domain.ErrCacheMiss
	}
	return e, nil
}

// Set writes value atomically (tmp + rename) with 0o600 perms.
func Set[T any](s *Store, key string, value T) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	e := Entry[T]{Schema: SchemaVersion, FetchedAt: time.Now().UTC(), Value: value}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	final := s.pathFor(key)
	f, err := os.CreateTemp(s.Dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, final)
}

// Delete removes a single key; missing keys are not an error.
func (s *Store) Delete(key string) error {
	err := os.Remove(s.pathFor(key))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Purge removes all entries under Dir (but preserves the directory itself).
func (s *Store) Purge() error {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		_ = os.Remove(filepath.Join(s.Dir, e.Name()))
	}
	return nil
}

// sanitizeKey keeps file names safe across filesystems.
func sanitizeKey(k string) string {
	k = strings.ToUpper(k)
	var b strings.Builder
	b.Grow(len(k))
	for _, r := range k {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func (s *Store) pathFor(key string) string {
	return filepath.Join(s.Dir, sanitizeKey(key)+".json")
}

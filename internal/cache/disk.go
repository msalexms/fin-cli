// Package cache is a small file-per-key JSON cache on disk. Entries record
// when they were written; callers decide how old is too old.
package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/msalexms/fin-cli/internal/fsutil"
)

// schemaVersion is bumped when the entry format changes incompatibly;
// entries with another version are discarded.
const schemaVersion = 2

// ErrMiss is returned when a key is absent, corrupt or from another schema.
var ErrMiss = errors.New("cache miss")

// Entry is the on-disk wrapper.
type Entry[T any] struct {
	Schema    int       `json:"schema"`
	FetchedAt time.Time `json:"fetched_at"`
	Value     T         `json:"value"`
}

// Store is a file-per-key cache rooted at Dir.
type Store struct {
	Dir string
}

// New returns a Store rooted at dir. The directory is created on first write.
func New(dir string) *Store { return &Store{Dir: dir} }

// Get reads key. Unreadable entries are removed and reported as ErrMiss.
func Get[T any](s *Store, key string) (Entry[T], error) {
	var e Entry[T]
	p := s.path(key)
	b, err := os.ReadFile(p)
	if err != nil {
		return e, ErrMiss
	}
	if err := json.Unmarshal(b, &e); err != nil || e.Schema != schemaVersion {
		_ = os.Remove(p)
		return Entry[T]{}, ErrMiss
	}
	return e, nil
}

// Set writes value atomically.
func Set[T any](s *Store, key string, value T) error {
	b, err := json.Marshal(Entry[T]{Schema: schemaVersion, FetchedAt: time.Now().UTC(), Value: value})
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(s.path(key), b)
}

// Purge removes every entry but keeps the directory.
func (s *Store) Purge() error {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var errs []error
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(s.Dir, e.Name())); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// path maps key to a file name that is safe on every file system.
func (s *Store) path(key string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(key) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return filepath.Join(s.Dir, b.String()+".json")
}

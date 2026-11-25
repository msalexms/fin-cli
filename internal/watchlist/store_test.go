package watchlist

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/msalexms/fin-cli/internal/domain"
)

func TestStore_AddRemoveMove(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "watchlist.toml"))

	if ts, err := s.Load(); err != nil || len(ts) != 0 {
		t.Fatalf("empty load = %v, %v", ts, err)
	}
	for _, tk := range []domain.Ticker{"AAPL", "MSFT", "SAN.MC"} {
		if err := s.Add(tk); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Add("AAPL"); !errors.Is(err, ErrAlreadyPresent) {
		t.Fatalf("duplicate add err = %v", err)
	}
	if err := s.Move("SAN.MC", -5); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("MSFT"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("MSFT"); !errors.Is(err, ErrNotPresent) {
		t.Fatalf("second remove err = %v", err)
	}
	got, _ := s.Load()
	if want := []domain.Ticker{"SAN.MC", "AAPL"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

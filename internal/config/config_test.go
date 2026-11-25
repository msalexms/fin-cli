package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLoad_FirstRunWritesDocumentedDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PollingInterval.Std() != DefaultPolling || !slices.Equal(cfg.Providers, Default().Providers) {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "# How often the TUI refreshes") || !strings.Contains(string(b), `polling_interval = "5m"`) {
		t.Fatalf("template not written:\n%s", b)
	}
}

func TestSaveLoad_RoundTripKeepsComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := Default()
	for key, val := range map[string]string{
		"finnhub.api_key":   "abc123",
		"polling_interval":  "90s",
		"providers":         "yahoo, finnhub, yahoo",
		"history_providers": "twelvedata",
		"ui.sort_mode":      "alpha",
	} {
		f, _ := FieldByKey(key)
		if err := f.Set(&cfg, val); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Finnhub.APIKey != "abc123" || got.PollingInterval.Std() != 90*time.Second ||
		!slices.Equal(got.Providers, []string{"yahoo", "finnhub"}) ||
		!slices.Equal(got.HistoryProviders, []string{"twelvedata"}) || got.UI.SortMode != "alpha" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "# Quote providers") {
		t.Fatal("comments lost on save")
	}
}

func TestFieldValidation(t *testing.T) {
	cfg := Default()
	bad := map[string]string{
		"polling_interval":  "5s",
		"providers":         "yahoo,bloomberg",
		"history_providers": "finnhub",
		"ui.sort_mode":      "random",
		"finnhub.api_key":   "has space",
	}
	for key, val := range bad {
		f, ok := FieldByKey(key)
		if !ok {
			t.Fatalf("missing field %s", key)
		}
		if err := f.Set(&cfg, val); err == nil {
			t.Errorf("%s=%q: expected error", key, val)
		}
	}
	if _, ok := FieldByKey("nope"); ok {
		t.Error("unknown key found")
	}
}

func TestLoad_NormalizesBadValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	raw := "schema_version = 1\npolling_interval = \"1s\"\nproviders = [\"nope\"]\n[ui]\nsort_mode = \"%desc\"\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PollingInterval.Std() != DefaultPolling || !slices.Equal(cfg.Providers, Default().Providers) ||
		cfg.UI.SortMode != SortChangeDesc || cfg.SchemaVersion != SchemaVersion {
		t.Fatalf("not normalized: %+v", cfg)
	}
}

func TestDurationString(t *testing.T) {
	for d, want := range map[time.Duration]string{
		5 * time.Minute: "5m", time.Hour: "1h", 90 * time.Second: "1m30s", 30 * time.Second: "30s",
	} {
		if got := Duration(d).String(); got != want {
			t.Errorf("%v → %q, want %q", d, got, want)
		}
	}
}

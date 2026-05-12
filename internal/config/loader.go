package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/sys/unix"
)

// CurrentSchemaVersion is the latest config schema fin-cli understands.
const CurrentSchemaVersion = 1

// Config is the root on-disk configuration.
// Keep it flat and TOML-friendly: snake_case field names, no maps of structs.
type Config struct {
	SchemaVersion  int            `toml:"schema_version"`
	PollingInterval Duration      `toml:"polling_interval"`
	Finnhub        FinnhubConfig  `toml:"finnhub"`
	OpenFIGI       OpenFIGIConfig `toml:"openfigi"`
	UI             UIConfig       `toml:"ui"`
}

// FinnhubConfig holds provider-specific options.
type FinnhubConfig struct {
	APIKey string `toml:"api_key"`
}

// OpenFIGIConfig holds ISIN resolver options. APIKey is optional.
type OpenFIGIConfig struct {
	APIKey string `toml:"api_key"`
}

// UIConfig gathers presentation options.
type UIConfig struct {
	// Reserved for future overrides (theme, polling, etc.).
}

// Default returns the default configuration used on first run.
func Default() Config {
	return Config{
		SchemaVersion:   CurrentSchemaVersion,
		PollingInterval: Duration(5 * time.Minute),
	}
}

// Duration wraps time.Duration to marshal/unmarshal as a TOML string (e.g. "5m").
type Duration time.Duration

// UnmarshalText parses a duration string.
func (d *Duration) UnmarshalText(text []byte) error {
	s := string(text)
	if s == "" {
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(v)
	return nil
}

// MarshalText serializes as "5m" / "1h30m".
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// Std returns the underlying time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// Load reads config from path. If the file does not exist, a default
// config is written (with placeholders) and returned.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			cfg := Default()
			if werr := writeWithPlaceholders(path); werr != nil {
				return cfg, werr
			}
			return cfg, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}

	migrated, err := migrate(cfg)
	if err != nil {
		return Config{}, err
	}
	if migrated.SchemaVersion != cfg.SchemaVersion {
		// Write back the migrated version, keeping a backup.
		_ = backup(path)
		if werr := Save(path, migrated); werr != nil {
			return Config{}, werr
		}
	}
	return migrated, nil
}

// Save writes cfg atomically with 0o600 perms, using flock on the target.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	// Advisory lock: prevent two fin-cli processes from interleaving writes.
	unlock, err := flock(path)
	if err != nil {
		return err
	}
	defer unlock()

	b, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return atomicWrite(path, b)
}

// writeWithPlaceholders creates a commented config.toml on first run.
func writeWithPlaceholders(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	const tpl = `# fin-cli configuration
# Edit with: fin-cli config edit
# Values can be overridden by environment variables:
#   FIN_CLI_FINNHUB_KEY
#   FIN_CLI_OPENFIGI_KEY

schema_version = 1
# polling_interval controls the TUI auto-refresh cadence.
polling_interval = "5m"

[finnhub]
# Get a free key at https://finnhub.io
api_key = ""

[openfigi]
# Optional (25 req/min without key, 250 req/min with one).
# https://www.openfigi.com/api
api_key = ""

[ui]
`
	return atomicWrite(path, []byte(tpl))
}

// atomicWrite writes data to path via tmp file + rename, with 0o600 perms.
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
	if err := os.Rename(tmp, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// flock acquires an advisory exclusive lock on path. It creates the file if
// missing. Returns an unlock function that must be called.
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

// backup renames path to path + ".bak.<unix>" as a best-effort snapshot.
func backup(path string) error {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	ts := time.Now().Unix()
	dst := fmt.Sprintf("%s.bak.%d", path, ts)
	return os.Rename(path, dst)
}

// Package config exposes XDG Base Directory aware paths for fin-cli.
package config

import (
	"os"
	"path/filepath"
)

const appName = "fin-cli"

// Paths holds all directories and files fin-cli reads/writes.
type Paths struct {
	ConfigDir    string
	CacheDir     string
	DataDir      string
	ConfigFile   string // <ConfigDir>/config.toml
	WatchlistDir string // <ConfigDir>
	Watchlist    string // <ConfigDir>/watchlist.toml
	QuoteCache   string // <CacheDir>/quotes
	ISINCache    string // <CacheDir>/isin
	LogFile      string // <DataDir>/fin-cli.log
}

// DefaultPaths returns paths derived from XDG_* environment variables
// with fallbacks to ~/.config, ~/.cache, ~/.local/share.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	cfgBase := envOr("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	cacheBase := envOr("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	dataBase := envOr("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	p := Paths{
		ConfigDir:    filepath.Join(cfgBase, appName),
		CacheDir:     filepath.Join(cacheBase, appName),
		DataDir:      filepath.Join(dataBase, appName),
	}
	p.ConfigFile = filepath.Join(p.ConfigDir, "config.toml")
	p.WatchlistDir = p.ConfigDir
	p.Watchlist = filepath.Join(p.ConfigDir, "watchlist.toml")
	p.QuoteCache = filepath.Join(p.CacheDir, "quotes")
	p.ISINCache = filepath.Join(p.CacheDir, "isin")
	p.LogFile = filepath.Join(p.DataDir, "fin-cli.log")
	return p, nil
}

// EnsureDirs creates the needed directories with 0700 perms (idempotent).
func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.ConfigDir, p.CacheDir, p.DataDir, p.QuoteCache, p.ISINCache} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

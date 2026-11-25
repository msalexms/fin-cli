package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const appName = "fin-cli"

// Paths holds every file and directory fin-cli reads or writes.
type Paths struct {
	ConfigFile string
	Watchlist  string
	QuoteCache string
	ISINCache  string
	LogFile    string
}

// DefaultPaths follows the XDG Base Directory spec ($XDG_CONFIG_HOME,
// $XDG_CACHE_HOME, $XDG_STATE_HOME) on Linux and BSD, and the native
// per-user directories elsewhere unless XDG variables are set.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	cfgBase := filepath.Join(home, ".config")
	cacheBase := filepath.Join(home, ".cache")
	stateBase := filepath.Join(home, ".local", "state")
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		if d, err := os.UserConfigDir(); err == nil {
			cfgBase, stateBase = d, d
		}
		if d, err := os.UserCacheDir(); err == nil {
			cacheBase = d
		}
	}
	cfgDir := filepath.Join(envOr("XDG_CONFIG_HOME", cfgBase), appName)
	cacheDir := filepath.Join(envOr("XDG_CACHE_HOME", cacheBase), appName)
	stateDir := filepath.Join(envOr("XDG_STATE_HOME", stateBase), appName)

	return Paths{
		ConfigFile: filepath.Join(cfgDir, "config.toml"),
		Watchlist:  filepath.Join(cfgDir, "watchlist.toml"),
		QuoteCache: filepath.Join(cacheDir, "quotes"),
		ISINCache:  filepath.Join(cacheDir, "isin"),
		LogFile:    filepath.Join(stateDir, "fin-cli.log"),
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

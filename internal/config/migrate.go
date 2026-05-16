package config

import (
	"fmt"
	"time"
)

// migrate upgrades cfg to CurrentSchemaVersion, filling defaults for missing fields.
// It returns an error only for unsupported (future) schemas.
func migrate(cfg Config) (Config, error) {
	if cfg.SchemaVersion > CurrentSchemaVersion {
		return cfg, fmt.Errorf("config schema %d is newer than supported %d; upgrade fin-cli",
			cfg.SchemaVersion, CurrentSchemaVersion)
	}

	// v0 → v1 (legacy, handle zero-value schema)
	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = 1
	}

	// v1 → v2: add provider chain fields with safe defaults.
	if cfg.SchemaVersion == 1 {
		if len(cfg.Providers) == 0 {
			cfg.Providers = []string{"finnhub"}
			// If Yahoo was previously only used for history, add it as a quote
			// fallback too for global coverage.
			cfg.Providers = append(cfg.Providers, "yahoo")
		}
		if len(cfg.HistoryProviders) == 0 {
			cfg.HistoryProviders = []string{"yahoo"}
		}
		cfg.SchemaVersion = 2
	}

	// Ensure polling interval has a sensible value.
	if cfg.PollingInterval.Std() == 0 {
		cfg.PollingInterval = Duration(5 * time.Minute)
	}

	return cfg, nil
}

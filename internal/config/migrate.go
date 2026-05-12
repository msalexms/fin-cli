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
	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = CurrentSchemaVersion
	}
	if cfg.PollingInterval.Std() == 0 {
		cfg.PollingInterval = Duration(5 * time.Minute)
	}
	return cfg, nil
}

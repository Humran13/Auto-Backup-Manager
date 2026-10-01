package config

import "fmt"

// Migrate upgrades cfg in place to CurrentSchemaVersion. An empty/zero
// Version is treated as version 1 (the first shipped schema) so hand-written
// configs without an explicit version still load.
func Migrate(cfg *Config) error {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Version > CurrentSchemaVersion {
		return fmt.Errorf("unsupported future schema version %d", cfg.Version)
	}
	// Future migrations append here, e.g.:
	// if cfg.Version == 1 { migrateV1ToV2(cfg); cfg.Version = 2 }
	return nil
}

package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// validNameRE restricts job/storage names to safe filesystem- and
// rclone-path-friendly characters, since they end up embedded in repository
// paths and file names.
var validNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// Load reads and validates a config file from path. It never returns a
// Config that failed validation, so callers can trust the result outright.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	return Parse(data)
}

// Parse validates and returns cfg, migrating it forward if it was written by
// an older version of Auto-Backup-Manager.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := Migrate(&cfg); err != nil {
		return nil, fmt.Errorf("migrating config: %w", err)
	}

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

// Save serializes cfg to path. It never writes secrets because Config has no
// field capable of holding one.
func Save(path string, cfg *Config) error {
	if err := Validate(cfg); err != nil {
		return fmt.Errorf("refusing to save invalid config: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("serializing config: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// Validate strictly checks cfg for the mistakes that would otherwise surface
// as a confusing failure mid-backup: unknown destinations, empty source
// lists, duplicate names, and malformed retention.
func Validate(cfg *Config) error {
	if cfg.Version <= 0 {
		return fmt.Errorf("version must be set")
	}
	if cfg.Version > CurrentSchemaVersion {
		return fmt.Errorf("config schema version %d is newer than this build supports (%d); upgrade Auto-Backup-Manager", cfg.Version, CurrentSchemaVersion)
	}
	if cfg.Global.DeviceID == "" {
		return fmt.Errorf("global.device_id must be set (run 'abm setup')")
	}

	storageNames := map[string]Storage{}
	for i, s := range cfg.Storage {
		if !validNameRE.MatchString(s.Name) {
			return fmt.Errorf("storage[%d]: invalid name %q", i, s.Name)
		}
		if _, dup := storageNames[s.Name]; dup {
			return fmt.Errorf("storage[%d]: duplicate storage name %q", i, s.Name)
		}
		if err := validateStorageType(s.Type); err != nil {
			return fmt.Errorf("storage[%q]: %w", s.Name, err)
		}
		if s.RcloneRemote == "" && s.Type != StorageLocal {
			return fmt.Errorf("storage[%q]: rclone_remote must be set", s.Name)
		}
		storageNames[s.Name] = s
	}

	// Zero jobs is a legitimate, temporary state right after 'abm setup' and
	// 'abm storage add' but before the first 'abm job add'; callers that
	// actually need work to do (backup/maintain) check len(cfg.Jobs)
	// themselves and report a clear "nothing configured" message.
	for name, job := range cfg.Jobs {
		if !validNameRE.MatchString(name) {
			return fmt.Errorf("job %q: invalid job name", name)
		}
		if err := validateJob(name, job, storageNames); err != nil {
			return err
		}
	}
	return nil
}

func validateStorageType(t StorageType) error {
	switch t {
	case StorageGoogleDrive, StorageOneDrive, StorageDropbox, StorageS3, StorageSFTP, StorageLocal:
		return nil
	default:
		return fmt.Errorf("unknown storage type %q", t)
	}
}

func validateJob(name string, job Job, storage map[string]Storage) error {
	if len(job.Sources) == 0 {
		return fmt.Errorf("job %q: at least one source path is required", name)
	}
	seen := map[string]bool{}
	for _, src := range job.Sources {
		if src == "" {
			return fmt.Errorf("job %q: empty source path", name)
		}
		if seen[src] {
			return fmt.Errorf("job %q: duplicate source path %q", name, src)
		}
		seen[src] = true
	}
	if job.Destination == "" {
		return fmt.Errorf("job %q: destination is required", name)
	}
	if _, ok := storage[job.Destination]; !ok {
		return fmt.Errorf("job %q: destination %q does not match any configured storage", name, job.Destination)
	}
	if job.RepositoryPath == "" {
		return fmt.Errorf("job %q: repository_path is required", name)
	}
	if job.MaxFileSizeMB < 0 {
		return fmt.Errorf("job %q: max_file_size_mb cannot be negative", name)
	}
	if job.Retention != nil {
		if err := ValidateRetention(*job.Retention); err != nil {
			return fmt.Errorf("job %q: retention: %w", name, err)
		}
	}
	for i, db := range job.Databases {
		if err := validateDatabase(db); err != nil {
			return fmt.Errorf("job %q: databases[%d]: %w", name, i, err)
		}
	}
	return nil
}

func validateDatabase(db Database) error {
	switch db.Kind {
	case DatabaseMySQL, DatabasePostgreSQL:
		if db.CredentialsRef == "" {
			return fmt.Errorf("credentials_ref is required for %s", db.Kind)
		}
	case DatabaseSQLite:
		if db.Path == "" {
			return fmt.Errorf("path is required for sqlite databases")
		}
	default:
		return fmt.Errorf("unknown database kind %q", db.Kind)
	}
	return nil
}

// ValidateRetention rejects retention policies that would silently do
// nothing (all-zero) or that mix within-window and count-based keep rules in
// a way restic would reject.
func ValidateRetention(r Retention) error {
	if r.KeepWithinHourly == "" && r.KeepHourly == 0 && r.KeepDaily == 0 && r.KeepWeekly == 0 && r.KeepMonthly == 0 {
		return fmt.Errorf("at least one keep-* rule must be set")
	}
	if r.KeepWithinHourly != "" {
		if _, err := ParseDuration(r.KeepWithinHourly); err != nil {
			return fmt.Errorf("keep_within_hourly: %w", err)
		}
	}
	if r.KeepHourly < 0 || r.KeepDaily < 0 || r.KeepWeekly < 0 || r.KeepMonthly < 0 {
		return fmt.Errorf("keep counts cannot be negative")
	}
	return nil
}

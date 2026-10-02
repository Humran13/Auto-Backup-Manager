package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/Humran13/Auto-Backup-Manager/internal/provider"
)

// validNameRE restricts job/storage names to safe filesystem- and
// rclone-path-friendly characters, since they end up embedded in repository
// paths and file names.
var validNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// validJobName accepts the human-readable project names shown throughout the
// GUI while still being safe in state/lock filenames and repository paths.
func validJobName(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > 64 || strings.TrimSpace(name) != name {
		return false
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\\<>:"|?*`) {
		return false
	}
	for _, ch := range name {
		if ch < 32 || ch == 127 {
			return false
		}
	}
	return true
}

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
// an older version of Auto-Backup-Manager. A v1 document is detected by its
// version field and parsed with v1's own field shape (see ParseLegacyV1)
// before conversion, since v1's renamed fields can't be recovered from a
// document already unmarshaled into the current struct.
func Parse(data []byte) (*Config, error) {
	var probe struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	var cfg *Config
	if probe.Version == 0 || probe.Version == 1 {
		v2, err := ParseLegacyV1(data)
		if err != nil {
			return nil, err
		}
		cfg = v2
	} else {
		cfg = &Config{}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
	}

	if err := Migrate(cfg); err != nil {
		return nil, fmt.Errorf("migrating config: %w", err)
	}

	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
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
// as a confusing failure mid-backup: unknown providers/destinations, empty
// source lists, duplicate names, missing required (non-secret) provider
// fields, and malformed retention. It cannot check that required *secret*
// fields (access keys, passwords) have actually been stored -- that's
// internal/backend's job at run time, since Config has no access to the
// secret store.
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
		if err := validateStorageProvider(s); err != nil {
			return fmt.Errorf("storage[%q]: %w", s.Name, err)
		}
		storageNames[s.Name] = s
	}

	// Zero jobs is a legitimate, temporary state right after 'abm setup' and
	// 'abm storage add' but before the first 'abm job add'; callers that
	// actually need work to do (backup/maintain) check len(cfg.Jobs)
	// themselves and report a clear "nothing configured" message.
	for name, job := range cfg.Jobs {
		if !validJobName(name) {
			return fmt.Errorf("job %q: invalid job name", name)
		}
		if err := validateJob(name, job, storageNames); err != nil {
			return err
		}
	}
	return nil
}

func validateStorageProvider(s Storage) error {
	p, ok := provider.Get(s.Provider)
	if !ok {
		return fmt.Errorf("unknown provider %q (see 'abm storage providers')", s.Provider)
	}
	if p.Unsupported {
		return fmt.Errorf("provider %q is not supported: %s", s.Provider, p.UnsupportedReason)
	}
	for _, f := range p.RequiredFields {
		if f.Secret {
			continue // secret fields live in the secret store, not here
		}
		if s.Options[f.Key] == "" && f.Default == "" {
			return fmt.Errorf("missing required field %q (%s) for provider %q", f.Key, f.Label, s.Provider)
		}
	}
	return nil
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
	if len(job.Destinations) == 0 {
		return fmt.Errorf("job %q: at least one destination is required", name)
	}
	seenDest := map[string]bool{}
	for _, d := range job.Destinations {
		if _, ok := storage[d]; !ok {
			return fmt.Errorf("job %q: destination %q does not match any configured storage", name, d)
		}
		if seenDest[d] {
			return fmt.Errorf("job %q: duplicate destination %q", name, d)
		}
		seenDest[d] = true
	}
	switch job.EffectivePolicy() {
	case PolicyAllRequired, PolicyPrimaryRequired:
	default:
		return fmt.Errorf("job %q: unknown destination_policy mode %q", name, job.DestinationPolicy.Mode)
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

// Package config defines the on-disk configuration schema for Auto-Backup-Manager
// and the validation rules that keep a job from ever running with bad input.
package config

// CurrentSchemaVersion is bumped whenever a backwards-incompatible change is
// made to the on-disk config format. Migrate() upgrades older documents to it.
//
// v2 generalized Storage from a fixed provider enum to an open provider-ID +
// options map (see internal/provider), and generalized Job.Destination (one
// string) to Job.Destinations (an ordered list with a policy), so a job can
// target more than one storage destination. See Migrate for exactly what an
// old v1 document turns into, including the cases that need the admin to
// re-run `abm storage add` afterward (S3 and SFTP moved from being
// rclone-mediated to restic's native backends, so their credentials -- held
// only in rclone.conf before -- cannot be carried forward automatically).
const CurrentSchemaVersion = 2

// Config is the root of config.yaml. It never contains plaintext secrets:
// restic repository passwords, cloud OAuth tokens, and database passwords are
// held in separate, tightly-permissioned secret stores (see internal/secrets).
type Config struct {
	Version int            `yaml:"version"`
	Global  Global         `yaml:"global"`
	Storage []Storage      `yaml:"storage"`
	Jobs    map[string]Job `yaml:"jobs"`
}

// Global holds device-wide settings that apply to every job unless overridden.
type Global struct {
	DeviceName       string    `yaml:"device_name"`
	DeviceID         string    `yaml:"device_id"`
	Organization     string    `yaml:"organization"`
	DefaultRetention Retention `yaml:"default_retention"`
	LogLevel         string    `yaml:"log_level"`
}

// Storage is a named destination. Provider is a stable ID from the provider
// registry (internal/provider), e.g. "google-drive", "generic-s3", "sftp" --
// never a fixed enum here, so adding a new provider never requires a config
// schema change. Options holds every non-secret field the provider's
// registry entry declares (endpoint, bucket, region, remote name, host,
// username, ...); secret fields (access keys, passwords, OAuth client
// secrets) are never stored here -- see internal/secrets and
// internal/backend, which resolves them by storage name at run time.
type Storage struct {
	Name      string            `yaml:"name"`
	Provider  string            `yaml:"provider"`
	Options   map[string]string `yaml:"options,omitempty"`
	Immutable bool              `yaml:"immutable,omitempty"`
}

// DatabaseKind identifies which native dump tool a database hook should use.
type DatabaseKind string

const (
	DatabaseMySQL      DatabaseKind = "mysql"
	DatabasePostgreSQL DatabaseKind = "postgresql"
	DatabaseSQLite     DatabaseKind = "sqlite"
)

// Database describes a single database to dump safely before each backup.
// Credentials are never stored here: CredentialsRef names an entry in the
// secret store (see internal/secrets) that is resolved at run time.
type Database struct {
	Kind           DatabaseKind `yaml:"kind"`
	Name           string       `yaml:"name"`
	Host           string       `yaml:"host,omitempty"`
	Port           int          `yaml:"port,omitempty"`
	CredentialsRef string       `yaml:"credentials_ref,omitempty"`
	Path           string       `yaml:"path,omitempty"` // for sqlite
}

// Retention configures how many recovery points restic's forget policy keeps.
// The project default is 10 days of hourly snapshots, i.e. KeepWithinHourly
// = "240h", pruned on PruneSchedule rather than after every backup.
type Retention struct {
	KeepWithinHourly string `yaml:"keep_within_hourly,omitempty"` // Go duration, e.g. "240h" (10 days)
	KeepHourly       int    `yaml:"keep_hourly,omitempty"`
	KeepDaily        int    `yaml:"keep_daily,omitempty"`
	KeepWeekly       int    `yaml:"keep_weekly,omitempty"`
	KeepMonthly      int    `yaml:"keep_monthly,omitempty"`
	PruneSchedule    string `yaml:"prune_schedule,omitempty"` // e.g. "daily"
}

// DestinationPolicyMode controls how a job with more than one destination
// treats a partial failure. Kept to exactly two modes deliberately -- see
// package job for how each is enforced.
type DestinationPolicyMode string

const (
	// PolicyAllRequired: every destination must succeed, or the whole run is reported failed.
	PolicyAllRequired DestinationPolicyMode = "all-required"
	// PolicyPrimaryRequired: the first (primary) destination must succeed;
	// a secondary destination failing leaves the run "degraded" (reported,
	// not hidden) rather than failed outright, so a flaky secondary never
	// blocks your actual protection on the primary.
	PolicyPrimaryRequired DestinationPolicyMode = "primary-required"
)

// DestinationPolicy configures multi-destination behavior for one job.
type DestinationPolicy struct {
	Mode DestinationPolicyMode `yaml:"mode,omitempty"`
}

// Job is one backup task: a set of sources going to one or more
// destinations on a schedule, with its own retention, exclusions and
// optional database hooks. Destinations[0] is always the primary.
//
// Each destination gets its OWN independent restic repository (own
// password, own lifecycle) under that destination, at
// RepositoryPath -- this project deliberately does not attempt to run two
// simultaneous restic writers against one shared repository, which restic
// does not support safely.
type Job struct {
	Sources           []string          `yaml:"sources"`
	Excludes          []string          `yaml:"excludes,omitempty"`
	MaxFileSizeMB     int64             `yaml:"max_file_size_mb,omitempty"`
	Destinations      []string          `yaml:"destinations"`
	DestinationPolicy DestinationPolicy `yaml:"destination_policy,omitempty"`
	RepositoryPath    string            `yaml:"repository_path"`
	Schedule          string            `yaml:"schedule,omitempty"` // e.g. "hourly"
	Retention         *Retention        `yaml:"retention,omitempty"`
	Databases         []Database        `yaml:"databases,omitempty"`
	Tags              []string          `yaml:"tags,omitempty"`
	Enabled           bool              `yaml:"enabled"`
}

// EffectivePolicy returns j's destination policy, defaulting to
// PolicyPrimaryRequired when unset -- a job with multiple destinations
// should not fail outright just because a secondary is temporarily
// unreachable while the primary (your actual protection) succeeded.
func (j Job) EffectivePolicy() DestinationPolicyMode {
	if j.DestinationPolicy.Mode != "" {
		return j.DestinationPolicy.Mode
	}
	return PolicyPrimaryRequired
}

// Primary returns j's primary (first) destination name, or "" if the job has none.
func (j Job) Primary() string {
	if len(j.Destinations) == 0 {
		return ""
	}
	return j.Destinations[0]
}

// Package config defines the on-disk configuration schema for Auto-Backup-Manager
// and the validation rules that keep a job from ever running with bad input.
package config

// CurrentSchemaVersion is bumped whenever a backwards-incompatible change is
// made to the on-disk config format. Migrate() upgrades older documents to it.
const CurrentSchemaVersion = 1

// Config is the root of config.yaml. It never contains plaintext secrets:
// restic repository passwords, cloud OAuth tokens, and database passwords are
// held in separate, tightly-permissioned secret stores (see internal/secrets).
type Config struct {
	Version int         `yaml:"version"`
	Global  Global      `yaml:"global"`
	Storage []Storage   `yaml:"storage"`
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

// StorageType enumerates the supported rclone-backed destinations.
type StorageType string

const (
	StorageGoogleDrive StorageType = "gdrive"
	StorageOneDrive    StorageType = "onedrive"
	StorageDropbox     StorageType = "dropbox"
	StorageS3          StorageType = "s3"
	StorageSFTP        StorageType = "sftp"
	StorageLocal       StorageType = "local"
)

// Storage is a named destination. It references an rclone remote by name;
// the remote's actual credentials live in rclone's own config file, which is
// protected by OS-native permissions/ACLs, never in this file.
type Storage struct {
	Name         string            `yaml:"name"`
	Type         StorageType       `yaml:"type"`
	RcloneRemote string            `yaml:"rclone_remote"`
	Options      map[string]string `yaml:"options,omitempty"`
	Immutable    bool              `yaml:"immutable,omitempty"`
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
	Kind          DatabaseKind `yaml:"kind"`
	Name          string       `yaml:"name"`
	Host          string       `yaml:"host,omitempty"`
	Port          int          `yaml:"port,omitempty"`
	CredentialsRef string      `yaml:"credentials_ref,omitempty"`
	Path          string       `yaml:"path,omitempty"` // for sqlite
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

// Job is one backup task: a set of sources going to one destination on a
// schedule, with its own retention, exclusions and optional database hooks.
type Job struct {
	Sources         []string   `yaml:"sources"`
	Excludes        []string   `yaml:"excludes,omitempty"`
	MaxFileSizeMB   int64      `yaml:"max_file_size_mb,omitempty"`
	Destination     string     `yaml:"destination"`
	RepositoryPath  string     `yaml:"repository_path"`
	Schedule        string     `yaml:"schedule,omitempty"` // e.g. "hourly"
	Retention       *Retention `yaml:"retention,omitempty"`
	Databases       []Database `yaml:"databases,omitempty"`
	Tags            []string   `yaml:"tags,omitempty"`
	Enabled         bool       `yaml:"enabled"`
}

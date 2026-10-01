//go:build !windows

// Package paths centralizes the standard filesystem locations Auto-Backup-Manager
// uses on each OS, so no other package hardcodes an installation layout.
package paths

import "os"

// ABM_HOME overrides the standard FHS layout for local development and
// integration tests, so the test suite never has to write into real system
// directories like /etc or /var/lib.
func envRoot(suffix, fallback string) string {
	if v := os.Getenv("ABM_HOME"); v != "" {
		return v + suffix
	}
	return fallback
}

// Linux layout follows the FHS: binaries under /usr/local/bin, config under
// /etc, mutable state under /var/lib, logs under /var/log.
var (
	ConfigDir  = envRoot("/etc/auto-backup-manager", "/etc/auto-backup-manager")
	SecretsDir = envRoot("/etc/auto-backup-manager/secrets", "/etc/auto-backup-manager/secrets")
	StateDir   = envRoot("/var/lib/auto-backup-manager", "/var/lib/auto-backup-manager")
	LockDir    = envRoot("/var/lib/auto-backup-manager/locks", "/var/lib/auto-backup-manager/locks")
	DumpDir    = envRoot("/var/lib/auto-backup-manager/dumps", "/var/lib/auto-backup-manager/dumps")
	LogDir     = envRoot("/var/log/auto-backup-manager", "/var/log/auto-backup-manager")
	BinDir     = "/usr/local/bin"
)

func ConfigFile() string       { return ConfigDir + "/config.yaml" }
func DeviceIDFile() string     { return ConfigDir + "/device-id" }
func RcloneConfigFile() string { return ConfigDir + "/rclone.conf" }

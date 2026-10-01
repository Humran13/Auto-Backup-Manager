//go:build windows

package paths

import "os"

func root() string {
	// ABM_HOME overrides the standard ProgramData layout for local
	// development and integration tests, so the test suite never has to
	// write into a real machine's ProgramData.
	if v := os.Getenv("ABM_HOME"); v != "" {
		return v
	}
	if v := os.Getenv("ProgramData"); v != "" {
		return v + `\Auto-Backup-Manager`
	}
	return `C:\ProgramData\Auto-Backup-Manager`
}

// Windows layout: configuration/state/secrets under ProgramData (matching
// how services store shared machine state), binaries under Program Files.
var (
	ConfigDir  = root()
	SecretsDir = root() + `\secrets`
	StateDir   = root() + `\state`
	LockDir    = root() + `\locks`
	DumpDir    = root() + `\dumps`
	LogDir     = root() + `\logs`
	BinDir     = `C:\Program Files\Auto-Backup-Manager`
)

func ConfigFile() string       { return ConfigDir + `\config.yaml` }
func DeviceIDFile() string     { return ConfigDir + `\device-id` }
func RcloneConfigFile() string { return ConfigDir + `\rclone.conf` }

// Command abm is the Auto-Backup-Manager CLI: the single cross-platform
// binary used for setup, scheduled/manual backups, restores, and diagnostics
// on both Linux and Windows.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/logging"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/secrets"
)

// version is set via -ldflags "-X main.version=..." at release build time.
var version = "dev"

// app bundles the resources most commands need: loaded config (nil if none
// exists yet, e.g. before `abm setup`), a secret store, and a logger.
type app struct {
	cfg       *config.Config
	cfgErr    error
	secrets   secrets.Store
	dbSecrets secrets.Store
	log       *slog.Logger
}

func newApp() *app {
	a := &app{}
	a.cfg, a.cfgErr = config.Load(paths.ConfigFile())
	a.secrets = newSecretStore(paths.SecretsDir)
	a.dbSecrets = newSecretStore(paths.SecretsDir + string(os.PathSeparator) + "databases")

	level := "info"
	if a.cfg != nil && a.cfg.Global.LogLevel != "" {
		level = a.cfg.Global.LogLevel
	}
	logger, err := logging.New(paths.LogDir, level)
	if err != nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
		logger.Warn("falling back to stderr-only logging", "error", err)
	}
	a.log = logger
	return a
}

// requireConfig exits with a clear message when a command needs a config
// that doesn't exist or failed to validate, instead of a confusing panic.
func (a *app) requireConfig() *config.Config {
	if a.cfg == nil {
		fmt.Fprintln(os.Stderr, "error: no valid configuration found. Run 'abm setup' first.")
		if a.cfgErr != nil {
			fmt.Fprintln(os.Stderr, "detail:", a.cfgErr)
		}
		os.Exit(1)
	}
	return a.cfg
}

func main() {
	a := newApp()

	root := &cobra.Command{
		Use:     "abm",
		Short:   "Auto-Backup-Manager: hourly encrypted backups for Linux and Windows",
		Version: version,
	}

	root.AddCommand(
		newSetupCmd(a),
		newStatusCmd(a),
		newDoctorCmd(a),
		newJobCmd(a),
		newBackupCmd(a),
		newMaintainCmd(a),
		newSnapshotsCmd(a),
		newRestoreCmd(a),
		newCheckCmd(a),
		newStorageCmd(a),
		newScheduleCmd(a),
		newLogsCmd(a),
		newUninstallCmd(a),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

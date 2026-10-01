// Command abm is the Auto-Backup-Manager CLI: the single cross-platform
// binary used for setup, scheduled/manual backups, restores, and diagnostics
// on both Linux and Windows.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/logging"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/secrets"
)

// version is set via -ldflags "-X main.version=..." at release build time.
var version = "dev"

// app bundles the resources most commands need: loaded config (nil if none
// exists yet, e.g. before `abm setup`), a secret store, and a logger. The CLI
// uses it directly; the GUI (cmd/abm/gui_*.go) uses the exact same app and
// the exact same internal/job, internal/config, internal/provider,
// internal/backend, internal/doctor, and internal/scheduler calls every CLI
// command uses -- there is no separate "GUI business logic", only HTTP
// handlers thin enough to just parse a request and call the same functions.
//
// mu guards cfg/cfgErr: the CLI only ever reads them once per process run,
// but the GUI is long-lived and serves concurrent requests, some of which
// mutate config.yaml -- reloadConfig() re-reads it under the write lock so
// every subsequent request sees the change.
type app struct {
	mu        sync.RWMutex
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
// CLI-only: the GUI must never os.Exit mid-request, so its handlers use
// currentConfig/reloadConfig instead.
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

// currentConfig returns the most recently loaded config (nil if none/invalid
// exists), safe for concurrent GUI request handling.
func (a *app) currentConfig() (*config.Config, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg, a.cfgErr
}

// reloadConfig re-reads config.yaml from disk, updating what currentConfig
// returns. Called after every GUI mutation (setup, storage add, job add,
// ...) so the next request sees it -- the CLI never needs this since each
// invocation is a fresh process.
func (a *app) reloadConfig() (*config.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg, a.cfgErr = config.Load(paths.ConfigFile())
	return a.cfg, a.cfgErr
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
		newGUICmd(a),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

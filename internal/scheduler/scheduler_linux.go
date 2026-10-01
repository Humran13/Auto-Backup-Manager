//go:build linux

// Package scheduler installs/removes the OS-native scheduled trigger that
// runs "abm backup now" every hour: a systemd timer on Linux, a Task
// Scheduler task on Windows. Neither platform requires a long-running abm
// daemon.
package scheduler

import (
	"fmt"
	"os"
	"os/exec"
)

const (
	serviceUnitPath = "/etc/systemd/system/auto-backup-manager.service"
	timerUnitPath   = "/etc/systemd/system/auto-backup-manager.timer"
)

// serviceUnit runs "abm backup now" for every enabled job in sequence.
// Type=oneshot means systemd considers the unit "done" after it exits, which
// is what lets the timer re-trigger it hourly without a resident process.
const serviceUnit = `[Unit]
Description=Auto-Backup-Manager hourly backup run
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/abm backup now --all
# Locking (internal/lock) already prevents overlap per job; this timeout is
# a last-resort safety net against a genuinely hung transfer.
TimeoutStartSec=6h
Nice=10
IOSchedulingClass=best-effort
IOSchedulingPriority=7

[Install]
WantedBy=multi-user.target
`

// timerUnit fires hourly. Persistent=true is what satisfies "a VPS that was
// powered off catches up on missed backups": systemd runs the unit once at
// boot if the last scheduled hourly trigger was missed while it was down.
const timerUnit = `[Unit]
Description=Run Auto-Backup-Manager hourly

[Timer]
OnCalendar=hourly
Persistent=true
RandomizedDelaySec=120

[Install]
WantedBy=timers.target
`

// maintenanceServiceUnit runs retention (forget --prune) once a day rather
// than after every hourly backup, avoiding unnecessary cloud load.
const maintenanceServiceUnit = `[Unit]
Description=Auto-Backup-Manager daily retention/prune

[Service]
Type=oneshot
ExecStart=/usr/local/bin/abm maintain --all --prune
TimeoutStartSec=6h
Nice=15

[Install]
WantedBy=multi-user.target
`

const maintenanceTimerUnit = `[Unit]
Description=Run Auto-Backup-Manager retention/prune daily

[Timer]
OnCalendar=daily
Persistent=true
RandomizedDelaySec=600

[Install]
WantedBy=timers.target
`

// Install writes the systemd units and enables + starts the timers.
func Install() error {
	files := map[string]string{
		serviceUnitPath: serviceUnit,
		timerUnitPath:   timerUnit,
		"/etc/systemd/system/auto-backup-manager-maintain.service": maintenanceServiceUnit,
		"/etc/systemd/system/auto-backup-manager-maintain.timer":   maintenanceTimerUnit,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	if err := run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := run("systemctl", "enable", "--now", "auto-backup-manager.timer"); err != nil {
		return err
	}
	return run("systemctl", "enable", "--now", "auto-backup-manager-maintain.timer")
}

// Uninstall disables and removes the systemd units. It never touches backup
// repositories or configuration -- only the scheduling itself.
func Uninstall() error {
	for _, unit := range []string{"auto-backup-manager.timer", "auto-backup-manager-maintain.timer"} {
		_ = run("systemctl", "disable", "--now", unit)
	}
	for _, path := range []string{
		serviceUnitPath, timerUnitPath,
		"/etc/systemd/system/auto-backup-manager-maintain.service",
		"/etc/systemd/system/auto-backup-manager-maintain.timer",
	} {
		_ = os.Remove(path)
	}
	return run("systemctl", "daemon-reload")
}

// Status reports whether the timer is active and enabled.
func Status() (string, error) {
	cmd := exec.Command("systemctl", "status", "auto-backup-manager.timer", "--no-pager")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, out)
	}
	return nil
}

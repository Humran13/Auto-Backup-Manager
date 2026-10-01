//go:build windows

package job

import "golang.org/x/sys/windows"

// canUseFSSnapshot reports whether this process holds the elevated token VSS
// requires. The scheduled Task Scheduler task always runs as SYSTEM (see
// internal/scheduler), so this is true for scheduled hourly runs; an
// unelevated interactive "abm backup now" instead falls back to a plain
// backup with a clearly reported warning, rather than failing outright.
func canUseFSSnapshot() bool {
	token := windows.GetCurrentProcessToken()
	return token.IsElevated()
}

//go:build !windows

package job

// canUseFSSnapshot is always false on non-Windows: restic's
// --use-fs-snapshot (VSS) is a Windows-only feature.
func canUseFSSnapshot() bool { return false }

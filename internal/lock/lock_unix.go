//go:build !windows

package lock

import "syscall"

// processAlive sends signal 0, which performs no action but still fails
// with ESRCH if the process does not exist -- the standard Unix liveness
// check that avoids the races of parsing /proc.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}

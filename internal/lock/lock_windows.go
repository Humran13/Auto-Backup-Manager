//go:build windows

package lock

import "golang.org/x/sys/windows"

// processAlive opens the process with the minimal query right and checks its
// exit code; os.FindProcess alone always succeeds on Windows and cannot be
// used to detect liveness.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	const stillActive = 259
	return exitCode == stillActive
}

//go:build !windows

package main

import "os/exec"

// openBrowser best-effort opens a browser on Linux via xdg-open, if
// available -- many VPS/server environments have no desktop/browser at all,
// so this silently does nothing rather than erroring when it's missing; the
// URL is always printed to stdout regardless (see runGUIServer).
func openBrowser(url string) {
	if _, err := exec.LookPath("xdg-open"); err != nil {
		return
	}
	_ = exec.Command("xdg-open", url).Start()
}

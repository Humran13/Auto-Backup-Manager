//go:build windows

package main

import "os/exec"

// openBrowser opens the user's default browser on Windows via the shell's
// own URL association (rundll32 url.dll,FileProtocolHandler), which is the
// standard way to do this without a third-party dependency.
func openBrowser(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

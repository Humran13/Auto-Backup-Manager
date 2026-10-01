package main

import (
	"net/http"

	"github.com/Humran13/Auto-Backup-Manager/internal/scheduler"
)

func (g *guiServer) handleScheduleShow(w http.ResponseWriter, r *http.Request) {
	out, err := scheduler.Status()
	resp := map[string]any{"status": out}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleScheduleSet installs the OS-native hourly schedule (systemd
// timer/Task Scheduler), the exact same internal/scheduler.Install call
// `abm schedule set` makes -- including the same elevation requirement on
// Windows, which surfaces here as a normal JSON error rather than a CLI
// exit code.
func (g *guiServer) handleScheduleSet(w http.ResponseWriter, r *http.Request) {
	if err := installScheduler(); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

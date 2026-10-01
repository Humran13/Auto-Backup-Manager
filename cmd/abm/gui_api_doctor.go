package main

import (
	"context"
	"net/http"

	"github.com/Humran13/Auto-Backup-Manager/internal/doctor"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/scheduler"
)

type checkView struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "healthy" | "warning" | "error"
	Detail string `json:"detail"`
}

// handleDoctor runs the exact same internal/doctor.Run checks as `abm
// doctor`, just rendered as a list of healthy/warning/error rows instead of
// terminal lines.
func (g *guiServer) handleDoctor(w http.ResponseWriter, r *http.Request) {
	cfg, _ := g.app.currentConfig()
	report := doctor.Run(context.Background(), doctor.Options{
		Config:          cfg,
		StateDir:        paths.StateDir,
		SchedulerStatus: scheduler.Status,
	})
	views := make([]checkView, 0, len(report.Checks))
	for _, c := range report.Checks {
		status := "healthy"
		if !c.OK {
			status = "error"
		}
		views = append(views, checkView{Name: c.Name, Status: status, Detail: c.Detail})
	}
	writeJSON(w, http.StatusOK, views)
}

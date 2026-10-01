package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Humran13/Auto-Backup-Manager/internal/job"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/restic"
)

type snapshotView struct {
	ID       string   `json:"id"`
	ShortID  string   `json:"shortId"`
	Time     string   `json:"time"`
	Hostname string   `json:"hostname"`
	Paths    []string `json:"paths"`
	Tags     []string `json:"tags"`
	IsLatest bool     `json:"isLatest"`
}

// handleSnapshots lists recoverable snapshots for a job's destination,
// identical to `abm snapshots` -- same restic.Runner resolution
// (job.ResticRunner), same Latest() marking.
func (g *guiServer) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	jobName := r.URL.Query().Get("job")
	destName := r.URL.Query().Get("destination")
	if jobName == "" {
		writeErr(w, http.StatusBadRequest, "job is required")
		return
	}
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "not configured")
		return
	}
	if _, ok := cfg.Jobs[jobName]; !ok {
		writeErr(w, http.StatusNotFound, "no such job")
		return
	}

	rr, _, err := job.ResticRunner(g.app.jobDepsFor(cfg), jobName, destName)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	snaps, err := rr.Snapshots(r.Context(), nil)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	latest, _ := restic.Latest(snaps)

	views := make([]snapshotView, 0, len(snaps))
	for _, s := range snaps {
		views = append(views, snapshotView{
			ID: s.ID, ShortID: s.ShortID, Time: s.Time.Format(timeFormat),
			Hostname: s.Hostname, Paths: s.Paths, Tags: s.Tags,
			IsLatest: latest != nil && s.ID == latest.ID,
		})
	}
	writeJSON(w, http.StatusOK, views)
}

type restoreRequest struct {
	Job         string   `json:"job"`
	Destination string   `json:"destination"`
	SnapshotID  string   `json:"snapshotId"` // "latest" or a specific ID
	Target      string   `json:"target"`     // empty -> auto-generated safe default
	Include     []string `json:"include"`
	InPlace     bool     `json:"inPlace"`
	Confirm     bool     `json:"confirm"` // required when InPlace is true
}

// handleRestore starts a real restic restore (internal/restic.Restore, the
// exact call `abm restore` makes) in the background. The default target is
// always a new, separate directory (C:\ABM-Restores\<job>\<timestamp> or
// /var/lib/auto-backup-manager/restores/<job>/<timestamp>), never the
// original source path, unless InPlace is explicitly set and Confirm is
// true -- mirroring the CLI's --in-place/--yes safety gate exactly.
func (g *guiServer) handleRestore(w http.ResponseWriter, r *http.Request) {
	var req restoreRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Job == "" || req.SnapshotID == "" {
		writeErr(w, http.StatusBadRequest, "job and snapshotId are required")
		return
	}
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "not configured")
		return
	}
	j, ok := cfg.Jobs[req.Job]
	if !ok {
		writeErr(w, http.StatusNotFound, "no such job")
		return
	}

	target := req.Target
	if req.InPlace {
		if !req.Confirm {
			writeErr(w, http.StatusBadRequest, "in-place restore requires confirm=true")
			return
		}
		if target == "" {
			if len(j.Sources) != 1 {
				writeErr(w, http.StatusBadRequest, "in-place restore requires an explicit target when a job has more than one source")
				return
			}
			target = j.Sources[0]
		}
	} else if target == "" {
		target = defaultRestoreTarget(req.Job)
	}

	rr, _, err := job.ResticRunner(g.app.jobDepsFor(cfg), req.Job, req.Destination)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	runID, rs := g.newRun("restore", req.Job)
	rs.Stage = "Restoring"

	go func() {
		err := rr.Restore(context.Background(), restic.RestoreOptions{
			SnapshotID: req.SnapshotID, Target: target, Include: req.Include,
		})
		g.mu.Lock()
		defer g.mu.Unlock()
		rs.Done = true
		if err != nil {
			rs.Error = err.Error()
			rs.Stage = "Failed"
		} else {
			rs.Stage = "Completed"
			rs.Result = map[string]string{"target": target}
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"runId": runID, "target": target})
}

// defaultRestoreTarget builds a safe, timestamped, job-scoped restore
// directory under the platform's standard ABM state location, so repeated
// restores never collide and never touch the original source by accident.
// Uses os.MkdirTemp (not a second-granularity timestamp alone) for the final
// path component: two restores started within the same second -- easy to
// trigger, found while testing this feature -- would otherwise resolve to
// the identical directory, and since restic's restore never deletes stale
// files left over from a previous restore into the same target, a second
// restore's "latest" snapshot could appear to still contain files that only
// a different, earlier snapshot actually had.
func defaultRestoreTarget(jobName string) string {
	day := time.Now().Format("20060102")
	parent := filepath.Join(paths.StateDir, "restores", jobName, day)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		// Fall back to a nanosecond-precision path rather than failing the
		// restore outright over a cosmetic directory-naming problem.
		return filepath.Join(parent, time.Now().Format("150405.000000000"))
	}
	dir, err := os.MkdirTemp(parent, time.Now().Format("150405-")+"*")
	if err != nil {
		return filepath.Join(parent, time.Now().Format("150405.000000000"))
	}
	return dir
}

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strings"
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

type recoveryManifestView struct {
	Job          string         `json:"job"`
	Organization string         `json:"organization"`
	Device       string         `json:"device"`
	Sources      []string       `json:"sources"`
	Databases    []databaseView `json:"databases"`
}

type recoveryPointContentsResponse struct {
	Manifest recoveryManifestView `json:"manifest"`
	Files    []restic.FileInfo    `json:"files"`
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

// handleRecoveryPointContents combines the immutable restic tree with the
// human-readable source manifest retained in the job configuration.
func (g *guiServer) handleRecoveryPointContents(w http.ResponseWriter, r *http.Request) {
	jobName := r.URL.Query().Get("job")
	snapshotID := r.URL.Query().Get("snapshot")
	destName := r.URL.Query().Get("destination")
	if jobName == "" || snapshotID == "" {
		writeErr(w, http.StatusBadRequest, "job and snapshot are required")
		return
	}
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "not configured")
		return
	}
	j, ok := cfg.Jobs[jobName]
	if !ok {
		writeErr(w, http.StatusNotFound, "no such job")
		return
	}
	rr, _, err := job.ResticRunner(g.app.jobDepsFor(cfg), jobName, destName)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	files, err := rr.ListFiles(r.Context(), snapshotID)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	manifest := recoveryManifestView{Job: jobName, Organization: cfg.Global.Organization, Device: cfg.Global.DeviceName, Sources: append([]string{}, j.Sources...)}
	for _, db := range j.Databases {
		manifest.Databases = append(manifest.Databases, databaseView{Kind: string(db.Kind), Name: db.Name, Host: db.Host, Port: db.Port, Path: db.Path})
	}
	writeJSON(w, http.StatusOK, recoveryPointContentsResponse{Manifest: manifest, Files: files})
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
	responseTarget := target
	originalInPlace := false
	if req.InPlace {
		if !req.Confirm {
			writeErr(w, http.StatusBadRequest, "in-place restore requires confirm=true")
			return
		}
		if target != "" {
			writeErr(w, http.StatusBadRequest, "in-place restore target is derived from the recovery manifest and must not be supplied")
			return
		}
		if err := validateOriginalSources(runtime.GOOS, j.Sources); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		// Restore into a verified staging tree first, then copy only the
		// configured source subtrees to their exact original paths. Besides
		// avoiding C:\\C\\... on Windows, staging prevents Restic from changing
		// unrelated ancestor metadata when the Unix reconstruction root is /.
		target = defaultRestoreTarget(req.Job + "-original")
		originalInPlace = true
		responseTarget = "configured original locations"
	} else if target == "" {
		target = defaultRestoreTarget(req.Job)
		responseTarget = target
	} else if err := validateSafeRestoreTarget(runtime.GOOS, target, j.Sources); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := os.MkdirAll(target, 0o750); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "restore destination cannot be created: "+err.Error())
		return
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
		if err == nil && originalInPlace {
			err = restoreOriginalSources(runtime.GOOS, target, j.Sources)
			if err == nil {
				_ = os.RemoveAll(target)
			}
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		rs.Done = true
		if err != nil {
			rs.Error = err.Error()
			rs.Stage = "Failed"
		} else {
			rs.Stage = "Completed"
			rs.Result = map[string]string{"target": responseTarget}
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"runId": runID, "target": responseTarget})
}

// validateOriginalSources rejects malformed or non-canonical manifest paths
// before an operation is allowed to write to live locations. It is kept
// platform-parametric so both path formats remain regression-tested on CI.
func validateOriginalSources(goos string, sources []string) error {
	if len(sources) == 0 {
		return fmt.Errorf("the recovery manifest has no original source paths")
	}
	for _, source := range sources {
		if hasControlCharacter(source) {
			return fmt.Errorf("the recovery manifest contains an invalid original source path")
		}
		if goos == "windows" {
			if _, _, err := splitWindowsAbsolutePath(source); err != nil {
				return fmt.Errorf("invalid original Windows source %q: %w", source, err)
			}
			continue
		}
		if !pathpkg.IsAbs(source) || pathpkg.Clean(source) != source {
			return fmt.Errorf("invalid original Unix source %q: path must be absolute and canonical", source)
		}
	}
	return nil
}

func validateSafeRestoreTarget(goos, target string, sources []string) error {
	if err := validateAbsoluteCanonicalPath(goos, target); err != nil {
		return fmt.Errorf("invalid restore destination: %w", err)
	}
	for _, source := range sources {
		if pathContains(goos, source, target) || pathContains(goos, target, source) {
			return fmt.Errorf("safe restore destination must be separate from every original source")
		}
	}
	return nil
}

func validateAbsoluteCanonicalPath(goos, value string) error {
	if hasControlCharacter(value) {
		return fmt.Errorf("path contains a control character")
	}
	if goos == "windows" {
		_, _, err := splitWindowsAbsolutePath(value)
		return err
	}
	if !pathpkg.IsAbs(value) || pathpkg.Clean(value) != value {
		return fmt.Errorf("path must be absolute and canonical")
	}
	return nil
}

func splitWindowsAbsolutePath(value string) (string, string, error) {
	normalized := strings.ReplaceAll(value, `\`, "/")
	if len(normalized) < 3 || normalized[1] != ':' || normalized[2] != '/' ||
		!((normalized[0] >= 'A' && normalized[0] <= 'Z') || (normalized[0] >= 'a' && normalized[0] <= 'z')) {
		return "", "", fmt.Errorf("path must start with an absolute drive root")
	}
	if strings.Contains(normalized[2:], ":") || pathpkg.Clean(normalized[2:]) != normalized[2:] {
		return "", "", fmt.Errorf("path must be canonical and must not contain traversal")
	}
	return strings.ToUpper(normalized[:1]), strings.TrimPrefix(normalized[2:], "/"), nil
}

func hasControlCharacter(value string) bool {
	for _, char := range value {
		if char < 32 || char == 127 {
			return true
		}
	}
	return false
}

func pathContains(goos, parent, child string) bool {
	if goos == "windows" {
		parent = strings.ToLower(strings.TrimRight(strings.ReplaceAll(parent, `\`, "/"), "/"))
		child = strings.ToLower(strings.TrimRight(strings.ReplaceAll(child, `\`, "/"), "/"))
	} else {
		parent = strings.TrimRight(parent, "/")
		child = strings.TrimRight(child, "/")
	}
	return child == parent || strings.HasPrefix(child, parent+"/")
}

// restoreOriginalSources materializes only configured source trees from
// Restic's absolute-path staging layout. On Windows this also avoids the
// incorrect C:\\C\\... nesting produced by restoring directly to C:\\.
func restoreOriginalSources(goos, staging string, sources []string) error {
	for _, destination := range sources {
		var staged string
		if goos == "windows" {
			drive, tail, err := splitWindowsAbsolutePath(destination)
			if err != nil {
				return err
			}
			staged = filepath.Join(staging, drive, filepath.FromSlash(tail))
		} else {
			staged = filepath.Join(staging, filepath.FromSlash(strings.TrimPrefix(destination, "/")))
		}
		if _, err := os.Lstat(staged); os.IsNotExist(err) {
			// A path-restricted restore may intentionally omit every file from
			// this configured source. Do not turn that into a failed restore.
			continue
		} else if err != nil {
			return fmt.Errorf("inspecting staged original source %q: %w", staged, err)
		}
		if err := copyRestoredPath(staged, destination); err != nil {
			return fmt.Errorf("restoring original source %q from retained staging directory %q: %w", destination, staging, err)
		}
	}
	return nil
}

func copyRestoredPath(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyRestoredFile(source, destination, info)
	}
	return filepath.WalkDir(source, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, current)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("restored path escaped its source tree")
		}
		dest := filepath.Join(destination, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(dest, info.Mode().Perm())
		}
		return copyRestoredFile(current, dest, info)
	})
}

func copyRestoredFile(source, destination string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
			return err
		}
		_ = os.Remove(destination)
		return os.Symlink(link, destination)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported restored file type %s", info.Mode().String())
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chtimes(destination, info.ModTime(), info.ModTime())
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

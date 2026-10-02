package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/job"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/retention"
)

type jobView struct {
	Name           string         `json:"name"`
	Enabled        bool           `json:"enabled"`
	Sources        []string       `json:"sources"`
	Excludes       []string       `json:"excludes"`
	Destinations   []string       `json:"destinations"`
	Policy         string         `json:"policy"`
	RepositoryPath string         `json:"repositoryPath"`
	KeepWithin     string         `json:"keepWithinHourly"`
	MaxFileSizeMB  int64          `json:"maxFileSizeMb"`
	Tags           []string       `json:"tags"`
	Databases      []databaseView `json:"databases"`
}

type databaseView struct {
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	Host           string `json:"host,omitempty"`
	Port           int    `json:"port,omitempty"`
	CredentialsRef string `json:"credentialsRef,omitempty"`
	Path           string `json:"path,omitempty"`
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
}

func toJobView(name string, j config.Job) jobView {
	v := jobView{
		Name: name, Enabled: j.Enabled, Sources: j.Sources, Excludes: j.Excludes,
		Destinations: j.Destinations, Policy: string(j.EffectivePolicy()),
		RepositoryPath: j.RepositoryPath, MaxFileSizeMB: j.MaxFileSizeMB, Tags: j.Tags,
	}
	if j.Retention != nil {
		v.KeepWithin = j.Retention.KeepWithinHourly
	}
	for _, db := range j.Databases {
		v.Databases = append(v.Databases, databaseView{
			Kind: string(db.Kind), Name: db.Name, Host: db.Host, Port: db.Port,
			CredentialsRef: db.CredentialsRef, Path: db.Path,
		})
	}
	return v
}

func (g *guiServer) handleJobsList(w http.ResponseWriter, r *http.Request) {
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeJSON(w, http.StatusOK, []jobView{})
		return
	}
	names := make([]string, 0, len(cfg.Jobs))
	for name := range cfg.Jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	views := make([]jobView, 0, len(names))
	for _, name := range names {
		views = append(views, toJobView(name, cfg.Jobs[name]))
	}
	writeJSON(w, http.StatusOK, views)
}

type jobUpsertRequest struct {
	Name           string         `json:"name"`
	Sources        []string       `json:"sources"`
	Excludes       []string       `json:"excludes"`
	Destinations   []string       `json:"destinations"`
	Policy         string         `json:"policy"`
	RepositoryPath string         `json:"repositoryPath"`
	KeepWithin     string         `json:"keepWithinHourly"`
	MaxFileSizeMB  int64          `json:"maxFileSizeMb"`
	Tags           []string       `json:"tags"`
	Databases      []databaseView `json:"databases"`
	Enabled        *bool          `json:"enabled"`
}

// handleJobUpsert both creates a new job and edits an existing one (upsert
// by name), mirroring `abm job add`'s validation and defaults exactly --
// repository_path defaults to org/device-id/job-name, retention defaults to
// the project's 10-day hourly window, same as the CLI.
func (g *guiServer) handleJobUpsert(w http.ResponseWriter, r *http.Request) {
	var req jobUpsertRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Sources) == 0 {
		writeErr(w, http.StatusBadRequest, "at least one source is required")
		return
	}
	if len(req.Destinations) == 0 {
		writeErr(w, http.StatusBadRequest, "at least one destination is required")
		return
	}

	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusConflict, "run setup first")
		return
	}

	_, existed := cfg.Jobs[req.Name]
	repoPath := req.RepositoryPath
	if repoPath == "" {
		if existed {
			repoPath = cfg.Jobs[req.Name].RepositoryPath
		} else {
			repoPath = cfg.Global.Organization + "/" + cfg.Global.DeviceID + "/" + req.Name
		}
	}
	keepWithin := req.KeepWithin
	if keepWithin == "" {
		keepWithin = retention.DefaultKeepWithinHourly
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	var dbs []config.Database
	for i, d := range req.Databases {
		credentialsRef := d.CredentialsRef
		if d.Kind != string(config.DatabaseSQLite) {
			credentialsRef = fmt.Sprintf("%s-db-%d", req.Name, i+1)
			if d.Username == "" || d.Password == "" {
				writeErr(w, http.StatusBadRequest, "database username and password are required")
				return
			}
			if err := g.app.dbSecrets.Set(credentialsRef, d.Username+":"+d.Password); err != nil {
				writeErr(w, http.StatusInternalServerError, "storing database credential: "+err.Error())
				return
			}
		}
		dbs = append(dbs, config.Database{
			Kind: config.DatabaseKind(d.Kind), Name: d.Name, Host: d.Host, Port: d.Port,
			CredentialsRef: credentialsRef, Path: d.Path,
		})
	}

	newJob := config.Job{
		Sources:           req.Sources,
		Excludes:          req.Excludes,
		MaxFileSizeMB:     req.MaxFileSizeMB,
		Destinations:      req.Destinations,
		DestinationPolicy: config.DestinationPolicy{Mode: config.DestinationPolicyMode(req.Policy)},
		RepositoryPath:    repoPath,
		Schedule:          "hourly",
		Retention:         &config.Retention{KeepWithinHourly: keepWithin, PruneSchedule: "daily"},
		Tags:              req.Tags,
		Databases:         dbs,
		Enabled:           enabled,
	}

	if cfg.Jobs == nil {
		cfg.Jobs = map[string]config.Job{}
	}
	cfg.Jobs[req.Name] = newJob
	if err := config.Save(paths.ConfigFile(), cfg); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if !existed {
		for _, dest := range req.Destinations {
			password, err := randomPassword()
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if err := g.app.secrets.Set(job.ResticPasswordKey(req.Name, dest), password); err != nil {
				writeErr(w, http.StatusInternalServerError, "storing repository password: "+err.Error())
				return
			}
		}
	}

	g.app.reloadConfig()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (g *guiServer) handleJobRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "not configured")
		return
	}
	if _, ok := cfg.Jobs[name]; !ok {
		writeErr(w, http.StatusNotFound, "no such job")
		return
	}
	delete(cfg.Jobs, name)
	if err := config.Save(paths.ConfigFile(), cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	g.app.reloadConfig()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type enableRequest struct {
	Enabled bool `json:"enabled"`
}

func (g *guiServer) handleJobEnable(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req enableRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "not configured")
		return
	}
	j, ok := cfg.Jobs[name]
	if !ok {
		writeErr(w, http.StatusNotFound, "no such job")
		return
	}
	j.Enabled = req.Enabled
	cfg.Jobs[name] = j
	if err := config.Save(paths.ConfigFile(), cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	g.app.reloadConfig()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleJobRun starts a real backup (internal/job.Run, the exact function
// `abm backup now` calls) in the background and returns a run ID the UI
// polls via GET /api/runs/{id}. Stage transitions reflect real log events
// from that run (see stageHandler), never a fabricated percentage restic
// doesn't report.
func (g *guiServer) handleJobRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "not configured")
		return
	}
	if _, ok := cfg.Jobs[name]; !ok {
		writeErr(w, http.StatusNotFound, "no such job")
		return
	}

	runID, rs := g.newRun("backup", name)
	deps := g.app.jobDepsFor(cfg)
	deps.Logger = newStageLogger(rs)

	go func() {
		// context.Background(), not r.Context(): this backup must keep
		// running after the HTTP response below is sent -- r.Context() is
		// canceled by net/http as soon as this handler returns, which would
		// otherwise kill the backup the instant the UI got its "started"
		// response back.
		st, err := job.Run(context.Background(), deps, name)
		g.mu.Lock()
		defer g.mu.Unlock()
		rs.Done = true
		if err != nil {
			rs.Error = err.Error()
			rs.Stage = "Failed"
		} else {
			rs.Stage = "Completed"
		}
		rs.Result = st
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"runId": runID})
}

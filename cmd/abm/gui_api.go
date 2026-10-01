package main

import (
	"net/http"
	"os"
	"sort"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/deviceid"
	"github.com/Humran13/Auto-Backup-Manager/internal/job"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/provider"
	"gopkg.in/yaml.v3"
)

type jobStatusView struct {
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	Destinations    []string `json:"destinations"`
	Policy          string   `json:"policy"`
	LastAttempt     string   `json:"lastAttempt,omitempty"`
	LastSuccess     string   `json:"lastSuccess,omitempty"`
	LastError       string   `json:"lastError,omitempty"`
	LastWarning     string   `json:"lastWarning,omitempty"`
	Degraded        bool     `json:"degraded"`
	PrimarySnapshot string   `json:"primarySnapshot,omitempty"`
	FilesNew        int      `json:"filesNew"`
	FilesChanged    int      `json:"filesChanged"`
	FilesUnmodified int      `json:"filesUnmodified"`
}

type statusResponse struct {
	HasConfig    bool            `json:"hasConfig"`
	DeviceName   string          `json:"deviceName"`
	Organization string          `json:"organization"`
	DeviceID     string          `json:"deviceId"`
	StorageCount int             `json:"storageCount"`
	Jobs         []jobStatusView `json:"jobs"`
}

func (g *guiServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeJSON(w, http.StatusOK, statusResponse{HasConfig: false})
		return
	}

	resp := statusResponse{
		HasConfig:    true,
		DeviceName:   cfg.Global.DeviceName,
		Organization: cfg.Global.Organization,
		DeviceID:     cfg.Global.DeviceID,
		StorageCount: len(cfg.Storage),
	}
	names := make([]string, 0, len(cfg.Jobs))
	for name := range cfg.Jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		j := cfg.Jobs[name]
		view := jobStatusView{Name: name, Enabled: j.Enabled, Destinations: j.Destinations, Policy: string(j.EffectivePolicy())}
		st, err := job.LoadStatus(paths.StateDir, name)
		if err == nil {
			if !st.LastAttempt.IsZero() {
				view.LastAttempt = st.LastAttempt.Format(timeFormat)
			}
			if !st.LastSuccess.IsZero() {
				view.LastSuccess = st.LastSuccess.Format(timeFormat)
			}
			view.LastError = st.LastError
			view.LastWarning = st.LastWarning
			view.Degraded = st.Degraded
			if p := st.Primary(); p != nil {
				view.PrimarySnapshot = p.LastSnapshotID
				view.FilesNew = p.FilesNew
				view.FilesChanged = p.FilesChanged
				view.FilesUnmodified = p.FilesUnmodified
			}
		}
		resp.Jobs = append(resp.Jobs, view)
	}
	writeJSON(w, http.StatusOK, resp)
}

const timeFormat = "2006-01-02 15:04:05 MST"

// providerView mirrors provider.Provider for JSON, filtering out nothing --
// the GUI renders directly from the same registry the CLI's
// `abm storage providers` reads, so a new provider never needs a GUI change.
type providerView struct {
	ID                  string                `json:"id"`
	DisplayName         string                `json:"displayName"`
	Family              string                `json:"family"`
	Backend             string                `json:"backend"`
	Maturity            string                `json:"maturity"`
	Auth                string                `json:"auth"`
	Headless            string                `json:"headless"`
	Experimental        bool                  `json:"experimental"`
	Unsupported         bool                  `json:"unsupported"`
	UnsupportedReason   string                `json:"unsupportedReason,omitempty"`
	RequiresOwnOAuthApp bool                  `json:"requiresOwnOAuthApp"`
	RequiredFields      []credentialFieldView `json:"requiredFields"`
	OptionalFields      []credentialFieldView `json:"optionalFields"`
	KnownLimitations    []string              `json:"knownLimitations"`
	DocPath             string                `json:"docPath"`
}

type credentialFieldView struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Secret      bool   `json:"secret"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

func toProviderView(p provider.Provider) providerView {
	conv := func(fs []provider.CredentialField) []credentialFieldView {
		out := make([]credentialFieldView, 0, len(fs))
		for _, f := range fs {
			out = append(out, credentialFieldView{Key: f.Key, Label: f.Label, Secret: f.Secret, Required: f.Required, Default: f.Default, Placeholder: f.Placeholder})
		}
		return out
	}
	return providerView{
		ID: p.ID, DisplayName: p.DisplayName, Family: string(p.Family), Backend: string(p.Backend),
		Maturity: string(p.Maturity), Auth: string(p.Auth), Headless: string(p.Headless),
		Experimental: p.Experimental, Unsupported: p.Unsupported, UnsupportedReason: p.UnsupportedReason,
		RequiresOwnOAuthApp: p.RequiresOwnOAuthApp,
		RequiredFields:      conv(p.RequiredFields),
		OptionalFields:      conv(p.OptionalFields),
		KnownLimitations:    p.KnownLimitations,
		DocPath:             p.DocPath,
	}
}

func (g *guiServer) handleProviders(w http.ResponseWriter, r *http.Request) {
	views := make([]providerView, 0, len(provider.Registry))
	for _, p := range provider.Registry {
		views = append(views, toProviderView(p))
	}
	sort.Slice(views, func(i, j int) bool { return views[i].ID < views[j].ID })
	writeJSON(w, http.StatusOK, views)
}

type settingsResponse struct {
	DeviceName       string `json:"deviceName"`
	Organization     string `json:"organization"`
	LogLevel         string `json:"logLevel"`
	DefaultRetention string `json:"defaultRetention"`
}

func (g *guiServer) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "not configured yet")
		return
	}
	writeJSON(w, http.StatusOK, settingsResponse{
		DeviceName:       cfg.Global.DeviceName,
		Organization:     cfg.Global.Organization,
		LogLevel:         cfg.Global.LogLevel,
		DefaultRetention: cfg.Global.DefaultRetention.KeepWithinHourly,
	})
}

type settingsRequest struct {
	DeviceName   string `json:"deviceName"`
	Organization string `json:"organization"`
	LogLevel     string `json:"logLevel"`
}

func (g *guiServer) handleSettingsSet(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "not configured yet")
		return
	}
	if req.DeviceName != "" {
		cfg.Global.DeviceName = req.DeviceName
	}
	if req.Organization != "" {
		cfg.Global.Organization = req.Organization
	}
	if req.LogLevel != "" {
		cfg.Global.LogLevel = req.LogLevel
	}
	if err := config.Save(paths.ConfigFile(), cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	g.app.reloadConfig()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type setupRequest struct {
	DeviceName   string `json:"deviceName"`
	Organization string `json:"organization"`
}

// handleSetup bootstraps device identity/config exactly like `abm setup
// --non-interactive`: same directories, same deviceid.LoadOrCreate call, so
// a config created via the GUI is byte-for-byte what the CLI would have
// produced.
func (g *guiServer) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DeviceName == "" {
		writeErr(w, http.StatusBadRequest, "deviceName is required")
		return
	}
	if req.Organization == "" {
		req.Organization = "default-org"
	}

	for _, dir := range []string{paths.ConfigDir, paths.StateDir, paths.LockDir, paths.DumpDir, paths.LogDir, paths.SecretsDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			writeErr(w, http.StatusInternalServerError, "creating "+dir+": "+err.Error())
			return
		}
	}

	id, err := deviceid.LoadOrCreate(paths.DeviceIDFile())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "device id: "+err.Error())
		return
	}

	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		cfg = &config.Config{Version: config.CurrentSchemaVersion, Jobs: map[string]config.Job{}}
	}
	cfg.Global.DeviceName = req.DeviceName
	cfg.Global.DeviceID = id
	cfg.Global.Organization = req.Organization
	if cfg.Global.LogLevel == "" {
		cfg.Global.LogLevel = "info"
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.WriteFile(paths.ConfigFile(), data, 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	g.app.reloadConfig()
	writeJSON(w, http.StatusOK, map[string]string{"deviceId": id})
}

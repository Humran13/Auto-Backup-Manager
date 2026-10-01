package main

import (
	"context"
	"net/http"
	"sort"

	"github.com/Humran13/Auto-Backup-Manager/internal/backend"
	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
	"github.com/Humran13/Auto-Backup-Manager/internal/provider"
)

type storageView struct {
	Name        string            `json:"name"`
	Provider    string            `json:"provider"`
	DisplayName string            `json:"displayName"`
	Family      string            `json:"family"`
	Backend     string            `json:"backend"`
	Maturity    string            `json:"maturity"`
	Immutable   bool              `json:"immutable"`
	Options     map[string]string `json:"options"` // never contains secret fields by construction
}

func (g *guiServer) handleStorageList(w http.ResponseWriter, r *http.Request) {
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeJSON(w, http.StatusOK, []storageView{})
		return
	}
	views := make([]storageView, 0, len(cfg.Storage))
	for _, s := range cfg.Storage {
		p, _ := provider.Get(s.Provider)
		views = append(views, storageView{
			Name: s.Name, Provider: s.Provider, DisplayName: p.DisplayName,
			Family: string(p.Family), Backend: string(p.Backend), Maturity: string(p.Maturity),
			Immutable: s.Immutable, Options: s.Options,
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	writeJSON(w, http.StatusOK, views)
}

type storageAddRequest struct {
	Name      string            `json:"name"`
	Provider  string            `json:"provider"`
	Options   map[string]string `json:"options"`
	Secrets   map[string]string `json:"secrets"`
	Immutable bool              `json:"immutable"`
	SkipProbe bool              `json:"skipProbe"`
}

// handleStorageAdd mirrors `abm storage add` exactly: same field validation
// against the provider registry, same capability probe before accepting the
// destination, same secret-store key scheme (backend.SecretKey) -- a
// storage destination added via the GUI is indistinguishable from one added
// via the CLI.
func (g *guiServer) handleStorageAdd(w http.ResponseWriter, r *http.Request) {
	var req storageAddRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	p, ok := provider.Get(req.Provider)
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown provider")
		return
	}
	if p.Unsupported {
		writeErr(w, http.StatusBadRequest, "provider is not supported: "+p.UnsupportedReason)
		return
	}

	options := req.Options
	if options == nil {
		options = map[string]string{}
	}
	secretVals := req.Secrets
	if secretVals == nil {
		secretVals = map[string]string{}
	}
	for _, f := range append(append([]provider.CredentialField{}, p.RequiredFields...), p.OptionalFields...) {
		var v string
		if f.Secret {
			v = secretVals[f.Key]
		} else {
			v = options[f.Key]
		}
		if v == "" {
			v = f.Default
		}
		if v == "" && f.Required {
			writeErr(w, http.StatusBadRequest, "missing required field: "+f.Label)
			return
		}
	}

	storage := config.Storage{Name: req.Name, Provider: req.Provider, Options: options, Immutable: req.Immutable}

	if !req.SkipProbe {
		probeStore := &inMemoryOverlayStore{base: g.app.secrets, overlay: map[string]string{}}
		for k, v := range secretVals {
			probeStore.overlay[backend.SecretKey(req.Name, k)] = v
		}
		if err := backend.Probe(context.Background(), "restic", storage, probeStore, paths.RcloneConfigFile()); err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "capability test failed: "+err.Error())
			return
		}
	}

	for k, v := range secretVals {
		if err := g.app.secrets.Set(backend.SecretKey(req.Name, k), v); err != nil {
			writeErr(w, http.StatusInternalServerError, "storing credential: "+err.Error())
			return
		}
	}

	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusConflict, "run setup first")
		return
	}
	cfg.Storage = append(cfg.Storage, storage)
	if err := config.Save(paths.ConfigFile(), cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	g.app.reloadConfig()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type storageNameRequest struct {
	Name string `json:"name"`
}

func (g *guiServer) handleStorageTest(w http.ResponseWriter, r *http.Request) {
	var req storageNameRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "not configured")
		return
	}
	s := findStorageByName(cfg, req.Name)
	if s == nil {
		writeErr(w, http.StatusNotFound, "no such storage")
		return
	}
	if err := backend.Probe(context.Background(), "restic", *s, g.app.secrets, paths.RcloneConfigFile()); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (g *guiServer) handleStorageReconnect(w http.ResponseWriter, r *http.Request) {
	var req storageNameRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "not configured")
		return
	}
	s := findStorageByName(cfg, req.Name)
	if s == nil {
		writeErr(w, http.StatusNotFound, "no such storage")
		return
	}
	p, ok := provider.Get(s.Provider)
	if !ok || p.Backend != provider.BackendRclone {
		writeErr(w, http.StatusBadRequest, "this storage is not rclone-backed; its credentials don't expire the way OAuth tokens do")
		return
	}
	remote := s.Options["remote"]
	if remote == "" {
		writeErr(w, http.StatusBadRequest, "storage has no rclone remote configured")
		return
	}
	if err := rcloneRunner().Reconnect(r.Context(), remote); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "reconnect failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (g *guiServer) handleStorageRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cfg, _ := g.app.currentConfig()
	if cfg == nil {
		writeErr(w, http.StatusNotFound, "not configured")
		return
	}
	if findStorageByName(cfg, name) == nil {
		writeErr(w, http.StatusNotFound, "no such storage")
		return
	}
	var kept []config.Storage
	for _, s := range cfg.Storage {
		if s.Name != name {
			kept = append(kept, s)
		}
	}
	cfg.Storage = kept
	if err := config.Save(paths.ConfigFile(), cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	g.app.reloadConfig()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

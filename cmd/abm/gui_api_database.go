package main

import (
	"net/http"
	"os"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/database"
	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
)

type databaseTestRequest struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Path     string `json:"path"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleDatabaseTest performs the same consistent logical dump a real job
// will use, then securely removes it. Credentials exist only in this request
// and the child process; they are never logged or returned to the browser.
func (g *guiServer) handleDatabaseTest(w http.ResponseWriter, r *http.Request) {
	var req databaseTestRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Kind == "" || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "database type and name are required")
		return
	}
	if err := os.MkdirAll(paths.DumpDir, 0o700); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create the protected test directory")
		return
	}
	tmp, err := os.MkdirTemp(paths.DumpDir, "connection-test-")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create the protected test directory")
		return
	}
	defer os.RemoveAll(tmp)
	db := config.Database{Kind: config.DatabaseKind(req.Kind), Name: req.Name, Host: req.Host, Port: req.Port, Path: req.Path}
	_, cleanup, err := database.Dump(r.Context(), db, database.Credentials{Username: req.Username, Password: req.Password}, tmp)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "database test failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

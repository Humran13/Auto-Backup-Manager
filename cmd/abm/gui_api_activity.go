package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
)

type activityEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Job     string `json:"job,omitempty"`
	Raw     string `json:"raw,omitempty"` // present only if the line wasn't valid JSON
}

// handleActivity reads the same structured log file `abm logs` reads
// (internal/logging writes it, already redacting secrets via
// internal/secrets.Redact before anything touches disk), parsing each JSON
// line into a friendlier shape for the Activity page; a line that isn't
// valid JSON is shown as-is rather than dropped.
func (g *guiServer) handleActivity(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}

	path := filepath.Join(paths.LogDir, "abm.log")
	f, err := os.Open(path)
	if err != nil {
		writeJSON(w, http.StatusOK, []activityEntry{})
		return
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > limit {
			lines = lines[1:]
		}
	}

	entries := make([]activityEntry, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- { // newest first
		line := lines[i]
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			entries = append(entries, activityEntry{Raw: line})
			continue
		}
		e := activityEntry{}
		if v, ok := raw["time"].(string); ok {
			e.Time = v
		}
		if v, ok := raw["level"].(string); ok {
			e.Level = v
		}
		if v, ok := raw["msg"].(string); ok {
			e.Message = v
		}
		if v, ok := raw["job"].(string); ok {
			e.Job = v
		}
		entries = append(entries, e)
	}
	writeJSON(w, http.StatusOK, entries)
}

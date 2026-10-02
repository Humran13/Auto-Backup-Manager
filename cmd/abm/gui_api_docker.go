package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type dockerSuggestion struct {
	Path    string `json:"path,omitempty"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// handleDockerInspect reads Compose metadata only; it never silently adds
// /var/lib/docker or image/cache layers. Bind mounts, compose configuration,
// and .env are suggested. Named volumes are reported for review because a
// live database volume must be handled through the database-dump workflow.
func (g *guiServer) handleDockerInspect(w http.ResponseWriter, r *http.Request) {
	root := filepath.Clean(r.URL.Query().Get("path"))
	if !filepath.IsAbs(root) {
		writeErr(w, http.StatusBadRequest, "an absolute application folder is required")
		return
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		writeErr(w, http.StatusBadRequest, "application folder is not available")
		return
	}
	var composePath string
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		candidate := filepath.Join(root, name)
		if _, err := os.Stat(candidate); err == nil {
			composePath = candidate
			break
		}
	}
	if composePath == "" {
		writeErr(w, http.StatusNotFound, "no Compose file was found in that folder")
		return
	}
	data, err := os.ReadFile(composePath)
	if err != nil {
		writeErr(w, http.StatusForbidden, "the Compose file cannot be read")
		return
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "the Compose file is not valid YAML")
		return
	}
	suggestions := []dockerSuggestion{{Path: composePath, Kind: "compose", Message: "Compose definition"}}
	if envPath := filepath.Join(root, ".env"); fileExists(envPath) {
		suggestions = append(suggestions, dockerSuggestion{Path: envPath, Kind: "config", Message: ".env configuration (may contain secrets; protected by repository encryption)"})
	}
	services, _ := document["services"].(map[string]any)
	seen := map[string]bool{composePath: true}
	for _, rawService := range services {
		service, _ := rawService.(map[string]any)
		volumes, _ := service["volumes"].([]any)
		for _, rawVolume := range volumes {
			value, ok := rawVolume.(string)
			if !ok {
				continue // long syntax is reported by Compose itself; never guess
			}
			source := strings.SplitN(value, ":", 2)[0]
			if strings.HasPrefix(source, ".") {
				source = filepath.Clean(filepath.Join(root, source))
			}
			if filepath.IsAbs(source) && !seen[source] {
				seen[source] = true
				suggestions = append(suggestions, dockerSuggestion{Path: source, Kind: "bind-mount", Message: "Persistent bind-mounted application data"})
			} else if source != "" && !strings.ContainsAny(source, `/\\`) {
				suggestions = append(suggestions, dockerSuggestion{Kind: "named-volume", Message: "Named volume " + source + " requires review; use a database dump for live database data"})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"composeFile": composePath, "suggestions": suggestions})
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

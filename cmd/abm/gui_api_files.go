package main

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Humran13/Auto-Backup-Manager/internal/paths"
)

type fileBrowserEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"isDir"`
	Size     int64  `json:"size,omitempty"`
	Modified string `json:"modified,omitempty"`
}

type fileBrowserResponse struct {
	Path    string             `json:"path"`
	Parent  string             `json:"parent,omitempty"`
	Roots   []string           `json:"roots"`
	Entries []fileBrowserEntry `json:"entries"`
}

// handleFilesBrowse always reads the filesystem of the machine running ABM.
// It deliberately does not use an HTML file input (which would browse the
// administrator's laptop when managing a VPS). Only absolute, cleaned paths
// are accepted; NUL/control characters are rejected before touching disk.
func (g *guiServer) handleFilesBrowse(w http.ResponseWriter, r *http.Request) {
	requested := r.URL.Query().Get("path")
	if requested == "" {
		if runtime.GOOS == "windows" {
			requested = firstWindowsRoot()
		} else {
			requested = string(os.PathSeparator)
		}
	}
	if strings.IndexFunc(requested, func(ch rune) bool { return ch == 0 || ch < 32 }) >= 0 || !filepath.IsAbs(requested) {
		writeErr(w, http.StatusBadRequest, "path must be an absolute server path")
		return
	}
	clean := filepath.Clean(requested)
	info, err := os.Stat(clean)
	if err != nil {
		writeErr(w, http.StatusNotFound, "path is not available on the server")
		return
	}
	if !info.IsDir() {
		writeErr(w, http.StatusBadRequest, "path is not a directory")
		return
	}

	dirEntries, err := os.ReadDir(clean)
	if err != nil {
		writeErr(w, http.StatusForbidden, "directory cannot be read by the ABM service")
		return
	}
	entries := make([]fileBrowserEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		item := fileBrowserEntry{Name: entry.Name(), Path: filepath.Join(clean, entry.Name()), IsDir: entry.IsDir()}
		if fi, statErr := entry.Info(); statErr == nil {
			item.Size = fi.Size()
			item.Modified = fi.ModTime().Format(timeFormat)
		}
		entries = append(entries, item)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	parent := filepath.Dir(clean)
	if parent == clean {
		parent = ""
	}
	writeJSON(w, http.StatusOK, fileBrowserResponse{Path: clean, Parent: parent, Roots: filesystemRoots(), Entries: entries})
}

func filesystemRoots() []string {
	if runtime.GOOS != "windows" {
		return []string{string(os.PathSeparator)}
	}
	var roots []string
	for letter := 'A'; letter <= 'Z'; letter++ {
		root := string(letter) + `:\`
		if _, err := os.Stat(root); err == nil {
			roots = append(roots, root)
		}
	}
	return roots
}

func firstWindowsRoot() string {
	roots := filesystemRoots()
	if len(roots) > 0 {
		return roots[0]
	}
	volume := filepath.VolumeName(paths.ConfigDir)
	if volume != "" {
		return volume + `\`
	}
	return `C:\`
}

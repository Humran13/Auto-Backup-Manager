// Package deviceid generates and persists the stable identifier that scopes
// each machine's backups within a shared repository, so unrelated computers
// never get combined under one uncontrolled path.
package deviceid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Generate returns a new random device ID: 16 random bytes as hex, prefixed
// with a short tag so it is recognizable in paths and logs at a glance.
func Generate() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating device id: %w", err)
	}
	return "dev-" + hex.EncodeToString(buf), nil
}

// LoadOrCreate reads the device ID stored at path, or generates and persists
// a new one if none exists yet. The ID must remain stable for the life of
// the machine: it is embedded in every backup repository path, and changing
// it would orphan existing snapshots from the "latest" resolution.
func LoadOrCreate(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("reading device id file %s: %w", path, err)
	}

	id, err := Generate()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", fmt.Errorf("creating device id directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o640); err != nil {
		return "", fmt.Errorf("writing device id file %s: %w", path, err)
	}
	return id, nil
}

//go:build !windows

package secrets

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileStore keeps each secret as the sole content of its own file under Dir,
// e.g. /etc/auto-backup-manager/secrets/<key>, created 0600 and owned by
// root. This is the "carefully permissioned root-only files" approach called
// for when systemd credentials aren't in play; it's also what
// RESTIC_PASSWORD_FILE needs directly, so Path just returns the file itself.
type FileStore struct {
	Dir string
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{Dir: dir}
}

func (s *FileStore) pathFor(key string) string {
	return filepath.Join(s.Dir, key)
}

func (s *FileStore) Get(key string) (string, error) {
	data, err := os.ReadFile(s.pathFor(key))
	if os.IsNotExist(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *FileStore) Set(key, value string) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("creating secrets directory: %w", err)
	}
	path := s.pathFor(key)
	// Write to a temp file first and rename, so a concurrent reader never
	// observes a partially written secret.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(value), 0o600); err != nil {
		return fmt.Errorf("writing secret %s: %w", key, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("finalizing secret %s: %w", key, err)
	}
	return os.Chmod(path, 0o600)
}

func (s *FileStore) Path(key string) (string, error) {
	path := s.pathFor(key)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	return path, nil
}

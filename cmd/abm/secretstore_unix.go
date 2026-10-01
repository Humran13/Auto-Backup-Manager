//go:build !windows

package main

import "github.com/Humran13/Auto-Backup-Manager/internal/secrets"

func newSecretStore(dir string) secrets.Store {
	return secrets.NewFileStore(dir)
}

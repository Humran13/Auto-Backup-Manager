package secrets

import "fmt"

// Store persists a single secret value (e.g. a restic repository password or
// a database credential) using whatever protection the host OS offers.
type Store interface {
	// Get returns the plaintext secret named key.
	Get(key string) (string, error)
	// Set writes/overwrites the secret named key.
	Set(key, value string) error
	// Path returns a filesystem path suitable for RESTIC_PASSWORD_FILE, i.e.
	// a location holding exactly this secret's plaintext and nothing else,
	// protected by OS permissions. Some backends materialize this on demand.
	Path(key string) (string, error)
}

// ErrNotFound is returned by Get when key has never been set.
var ErrNotFound = fmt.Errorf("secret not found")

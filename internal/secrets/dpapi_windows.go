//go:build windows

package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// DPAPIStore encrypts each secret with Windows Data Protection API before
// writing it under Dir (e.g. C:\ProgramData\Auto-Backup-Manager\secrets),
// using CRYPTPROTECT_LOCAL_MACHINE so any admin process on this machine can
// decrypt it -- matching a service that must run unattended, with no user
// logged in, the same way Task Scheduler runs the backup job itself.
type DPAPIStore struct {
	Dir string
}

func NewDPAPIStore(dir string) *DPAPIStore {
	return &DPAPIStore{Dir: dir}
}

var (
	crypt32                = syscall.NewLazyDLL("crypt32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree          = kernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

const cryptProtectLocalMachine = 0x4

func newBlob(data []byte) dataBlob {
	if len(data) == 0 {
		return dataBlob{}
	}
	return dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
}

func blobBytes(b dataBlob) []byte {
	if b.cbData == 0 {
		return nil
	}
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	return out
}

func protect(plaintext []byte) ([]byte, error) {
	in := newBlob(plaintext)
	var out dataBlob
	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0,
		uintptr(cryptProtectLocalMachine),
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return blobBytes(out), nil
}

func unprotect(ciphertext []byte) ([]byte, error) {
	in := newBlob(ciphertext)
	var out dataBlob
	r, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return blobBytes(out), nil
}

func (s *DPAPIStore) pathFor(key string) string {
	return filepath.Join(s.Dir, key+".dpapi")
}

func (s *DPAPIStore) Get(key string) (string, error) {
	data, err := os.ReadFile(s.pathFor(key))
	if os.IsNotExist(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	plain, err := unprotect(data)
	if err != nil {
		return "", fmt.Errorf("decrypting secret %s: %w", key, err)
	}
	return string(plain), nil
}

func (s *DPAPIStore) Set(key, value string) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("creating secrets directory: %w", err)
	}
	enc, err := protect([]byte(value))
	if err != nil {
		return fmt.Errorf("encrypting secret %s: %w", key, err)
	}
	path := s.pathFor(key)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, enc, 0o600); err != nil {
		return fmt.Errorf("writing secret %s: %w", key, err)
	}
	return os.Rename(tmp, path)
}

// Path decrypts the secret into a private per-process temp file for tools
// (like restic's RESTIC_PASSWORD_FILE) that need a plaintext file rather
// than an in-memory value. Callers must remove the returned path once done;
// it is never the persistent on-disk secret, which always stays encrypted.
func (s *DPAPIStore) Path(key string) (string, error) {
	plain, err := s.Get(key)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "abm-secret-*")
	if err != nil {
		return "", fmt.Errorf("creating temp secret file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(plain); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

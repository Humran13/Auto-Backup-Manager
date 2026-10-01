// Package backend is the one place in Auto-Backup-Manager that turns a
// provider + storage configuration into an actual restic repository target
// (the -r spec and any credential environment variables restic needs).
// Nothing else in the codebase should construct a repository spec by hand:
// backup, retention, scheduling and restore logic all call Build and never
// need to know which provider -- or even which transport family -- a
// destination uses. Adding a new provider that reuses an existing Backend
// (another S3-compatible preset, another rclone remote) requires no change
// here at all; adding a genuinely new transport is one new case.
package backend

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Humran13/Auto-Backup-Manager/internal/config"
	"github.com/Humran13/Auto-Backup-Manager/internal/provider"
	"github.com/Humran13/Auto-Backup-Manager/internal/secrets"
)

// Target is what restic needs to reach one repository.
type Target struct {
	Spec string   // the restic -r value
	Env  []string // extra environment variables (credentials) for the restic subprocess
	// RcloneConfig is non-empty only for BackendRclone targets, naming the
	// rclone.conf path restic must be pointed at via RCLONE_CONFIG.
	RcloneConfig string
	// ExtraArgs are additional restic global flags (e.g. "-o",
	// "sftp.command=...") inserted right after "-r <repo>". Used by SFTP
	// when a non-default port or a specific key file is needed: restic's
	// plain "sftp:user@host:/path" form has no syntax for either, since it
	// otherwise just shells out to the system ssh using its own config/agent.
	ExtraArgs []string
}

// SecretKey returns the secret-store key for one (storage, credential field)
// pair. Used both when registering a storage (cmd/abm/storage.go) and when
// resolving it here.
func SecretKey(storageName, fieldKey string) string {
	return "storage-" + storageName + "-" + fieldKey
}

// Build resolves storage (whose repository lives at repoPath within it) into
// a restic Target, dispatching purely on the provider's declared Backend.
func Build(storage config.Storage, repoPath string, secretStore secrets.Store, rcloneConfigPath string) (Target, error) {
	p, ok := provider.Get(storage.Provider)
	if !ok {
		return Target{}, fmt.Errorf("unknown provider %q for storage %q", storage.Provider, storage.Name)
	}

	secretVals, err := resolveSecrets(p, storage.Name, secretStore)
	if err != nil {
		return Target{}, err
	}
	opt := func(key string) string { return storage.Options[key] }

	switch p.Backend {
	case provider.BackendLocal:
		return buildLocal(opt, repoPath)
	case provider.BackendSFTP:
		return buildSFTP(opt, repoPath)
	case provider.BackendS3:
		return buildS3(opt, secretVals, repoPath)
	case provider.BackendAzure:
		return buildAzure(opt, secretVals, repoPath)
	case provider.BackendGS:
		return buildGS(opt, repoPath)
	case provider.BackendSwift:
		return buildSwift(opt, secretVals, repoPath)
	case provider.BackendRclone:
		return buildRclone(opt, repoPath, rcloneConfigPath)
	default:
		return Target{}, fmt.Errorf("provider %q has unhandled backend %q", storage.Provider, p.Backend)
	}
}

// resolveSecrets fetches every Secret credential field a provider declares
// from the secret store, keyed by field Key. A missing *required* secret
// produces an actionable error naming the exact command to fix it, rather
// than a generic "not found" the admin has to decode.
func resolveSecrets(p provider.Provider, storageName string, store secrets.Store) (map[string]string, error) {
	resolved := map[string]string{}
	all := append(append([]provider.CredentialField{}, p.RequiredFields...), p.OptionalFields...)
	for _, f := range all {
		if !f.Secret {
			continue
		}
		v, err := store.Get(SecretKey(storageName, f.Key))
		if err != nil {
			if f.Required {
				return nil, fmt.Errorf("missing required credential %q for storage %q: run 'abm storage add --provider %s --name %s ...' again to supply it",
					f.Key, storageName, p.ID, storageName)
			}
			continue
		}
		resolved[f.Key] = v
	}
	return resolved, nil
}

func buildLocal(opt func(string) string, repoPath string) (Target, error) {
	path := opt("path")
	if path == "" {
		return Target{}, fmt.Errorf("local storage has no path configured")
	}
	// Deliberately not joinPath: path is an absolute filesystem path (Unix
	// "/mnt/backup" or Windows "C:\backup") whose leading slash/drive must
	// be preserved, unlike the relative sub-paths joinPath normalizes.
	return Target{Spec: strings.TrimRight(path, "/\\") + "/" + strings.Trim(repoPath, "/")}, nil
}

func buildSFTP(opt func(string) string, repoPath string) (Target, error) {
	host := opt("host")
	user := opt("username")
	if host == "" || user == "" {
		return Target{}, fmt.Errorf("sftp storage is missing host/username")
	}
	path := joinPath(opt("path_prefix"), repoPath)
	spec := fmt.Sprintf("sftp:%s@%s:%s", user, host, path)

	port := opt("port")
	keyFile := opt("key_file")
	if (port == "" || port == "22") && keyFile == "" {
		// Default port, default identity/agent: restic's plain form is
		// sufficient, no need to override how it invokes ssh.
		return Target{Spec: spec}, nil
	}

	// A non-default port and/or a specific key file require restic's
	// sftp.command override: its plain "sftp:user@host:/path" syntax has no
	// way to express either, since restic otherwise just shells out to the
	// system ssh using its own config/agent for everything connection-related.
	sshArgs := []string{"ssh",
		// BatchMode disables any interactive prompt (an unknown host key, a
		// passphrase, a password fallback): ssh fails immediately with a
		// clear error instead of hanging indefinitely waiting for input
		// that will never come when restic runs this unattended. Found
		// during this project's own testing: without it, a backup against
		// a host whose key isn't yet trusted hangs for minutes before
		// eventually timing out, which would hang a scheduled hourly job.
		"-o", "BatchMode=yes",
	}
	if port != "" && port != "22" {
		sshArgs = append(sshArgs, "-p", port)
	}
	if keyFile != "" {
		// sftp.command is parsed with POSIX shell word-splitting, where a
		// backslash is an escape character -- a raw Windows path like
		// C:\keys\id_ed25519 gets silently mangled (found during this
		// project's own testing against a real SFTP server). Forward
		// slashes are accepted by Windows OpenSSH just as well and survive
		// that parsing intact.
		sshArgs = append(sshArgs, "-i", filepath.ToSlash(keyFile))
	}
	sshArgs = append(sshArgs, user+"@"+host, "-s", "sftp")
	return Target{
		Spec:      spec,
		ExtraArgs: []string{"-o", "sftp.command=" + strings.Join(sshArgs, " ")},
	}, nil
}

func buildS3(opt func(string) string, secretVals map[string]string, repoPath string) (Target, error) {
	bucket := opt("bucket")
	if bucket == "" {
		return Target{}, fmt.Errorf("s3 storage has no bucket configured")
	}
	accessKey, secretKey := secretVals["access_key"], secretVals["secret_key"]
	if accessKey == "" || secretKey == "" {
		return Target{}, fmt.Errorf("s3 storage is missing access_key/secret_key credentials")
	}

	path := joinPath(bucket, opt("path_prefix"), repoPath)
	endpoint := opt("endpoint")
	if endpoint == "" {
		endpoint = "s3.amazonaws.com" // Amazon S3's own default endpoint
	}
	// Real S3-compatible providers always use TLS, so that's the default;
	// but respect an explicit http:// scheme (e.g. an on-prem/self-hosted
	// S3-compatible server, or a local test double, running without TLS --
	// found during this project's own testing against a plain-HTTP local
	// test server, which the previous hardcoded "https://" silently broke).
	scheme := "https://"
	if strings.HasPrefix(endpoint, "http://") {
		scheme = "http://"
		endpoint = strings.TrimPrefix(endpoint, "http://")
	} else {
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}

	env := []string{
		"AWS_ACCESS_KEY_ID=" + accessKey,
		"AWS_SECRET_ACCESS_KEY=" + secretKey,
	}
	if region := opt("region"); region != "" {
		env = append(env, "AWS_DEFAULT_REGION="+region)
	}
	return Target{Spec: "s3:" + scheme + endpoint + "/" + path, Env: env}, nil
}

func buildAzure(opt func(string) string, secretVals map[string]string, repoPath string) (Target, error) {
	container := opt("container")
	accountName := opt("account_name")
	accountKey := secretVals["account_key"]
	if container == "" || accountName == "" || accountKey == "" {
		return Target{}, fmt.Errorf("azure storage is missing container/account_name/account_key")
	}
	return Target{
		Spec: fmt.Sprintf("azure:%s:/%s", container, repoPath),
		Env: []string{
			"AZURE_ACCOUNT_NAME=" + accountName,
			"AZURE_ACCOUNT_KEY=" + accountKey,
		},
	}, nil
}

func buildGS(opt func(string) string, repoPath string) (Target, error) {
	bucket := opt("bucket")
	projectID := opt("project_id")
	credsFile := opt("credentials_file")
	if bucket == "" || projectID == "" || credsFile == "" {
		return Target{}, fmt.Errorf("google cloud storage is missing bucket/project_id/credentials_file")
	}
	return Target{
		Spec: fmt.Sprintf("gs:%s:/%s", bucket, repoPath),
		Env: []string{
			"GOOGLE_PROJECT_ID=" + projectID,
			"GOOGLE_APPLICATION_CREDENTIALS=" + credsFile,
		},
	}, nil
}

func buildSwift(opt func(string) string, secretVals map[string]string, repoPath string) (Target, error) {
	container := opt("container")
	authURL := opt("auth_url")
	username := opt("username")
	password := secretVals["password"]
	if container == "" || authURL == "" || username == "" || password == "" {
		return Target{}, fmt.Errorf("swift storage is missing container/auth_url/username/password")
	}
	env := []string{
		"OS_AUTH_URL=" + authURL,
		"OS_USERNAME=" + username,
		"OS_PASSWORD=" + password,
	}
	if t := opt("tenant"); t != "" {
		env = append(env, "OS_TENANT_NAME="+t)
	}
	if r := opt("region"); r != "" {
		env = append(env, "OS_REGION_NAME="+r)
	}
	return Target{Spec: fmt.Sprintf("swift:%s:/%s", container, repoPath), Env: env}, nil
}

func buildRclone(opt func(string) string, repoPath, rcloneConfigPath string) (Target, error) {
	remote := opt("remote")
	if remote == "" {
		return Target{}, fmt.Errorf("rclone-backed storage has no remote configured")
	}
	return Target{Spec: "rclone:" + remote + ":" + repoPath, RcloneConfig: rcloneConfigPath}, nil
}

func joinPath(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "/")
}

// Package provider is Auto-Backup-Manager's storage-provider catalog: a
// single registry describing every supported destination (what it is, how
// to authenticate to it, how mature that support is, and what restic/rclone
// backend actually moves the bytes). Nothing outside this package and
// internal/backend should know provider-specific details -- adding a new
// provider later means adding one entry here (plus, for a new underlying
// transport, a small backend adapter), not touching backup, retention,
// scheduling, or restore logic anywhere else.
package provider

// Family groups providers that share a connection/auth shape, used mainly
// for the setup wizard's category menu.
type Family string

const (
	FamilyCloudDrive    Family = "cloud-drive"    // consumer/business file-sync services via rclone OAuth
	FamilyObjectStorage Family = "object-storage" // S3-compatible or another native object backend
	FamilySFTP          Family = "sftp"
	FamilyLocal         Family = "local"
	FamilyGenericRclone Family = "generic-rclone" // escape hatch: any rclone remote the user already configured
)

// Maturity is an evidence-based claim, never a marketing one: it reflects
// what has actually been validated, not what "should" work. See
// docs/TESTING.md and the provider matrix in the project status report for
// what backs each level.
type Maturity string

const (
	// MaturityStable: ABM has validated the integration architecture, the
	// upstream backend is mature, and either a real-account round-trip or an
	// equivalent protocol-level integration test (e.g. MinIO for S3,
	// a disposable SFTP server) has passed.
	MaturityStable Maturity = "stable"
	// MaturitySupported: implemented and documented, expected to work based
	// on the upstream rclone/restic backend's own maturity, but ABM has not
	// run a real-account validation (typically because that requires a
	// human OAuth approval or a paid account this project doesn't have).
	MaturitySupported Maturity = "supported"
	// MaturityExperimental: the upstream backend, auth model, or protocol
	// itself carries meaningful limitations (non-public APIs, mandatory
	// periodic reauthentication, 2FA session quirks) independent of ABM.
	MaturityExperimental Maturity = "experimental"
	// MaturityUnavailable: no safe, supported integration currently exists.
	MaturityUnavailable Maturity = "unavailable"
)

// AuthMethod describes how a user authenticates a provider.
type AuthMethod string

const (
	AuthOAuth    AuthMethod = "oauth"     // standard rclone OAuth (browser or authorize-token)
	AuthOAuthJWT AuthMethod = "oauth-jwt" // Box-style: browser OAuth or an app JWT service-account config
	AuthAPIKey   AuthMethod = "api-key"   // access key / secret key (object storage)
	AuthSSHKey   AuthMethod = "ssh-key"   // SFTP
	AuthSession  AuthMethod = "session"   // username/password producing a session/trust token (iCloud, Jottacloud legacy)
	AuthNone     AuthMethod = "none"      // local disk
	AuthExisting AuthMethod = "existing"  // generic rclone passthrough: reuses a remote the user already authenticated
)

// HeadlessSupport describes whether a provider can be authenticated on a
// server with no browser of its own. There is no single universal answer:
// each provider's own OAuth/auth implementation decides this, so ABM never
// presents one generic flow for all of them.
type HeadlessSupport string

const (
	// HeadlessDirect: the auth flow needs no browser at all (API keys, SSH keys, local).
	HeadlessDirect HeadlessSupport = "direct"
	// HeadlessTokenTransfer: run `rclone authorize` (or equivalent) on any
	// machine with a browser, then paste the resulting token on the server.
	HeadlessTokenTransfer HeadlessSupport = "token-transfer"
	// HeadlessDifficult: technically possible but meaningfully painful
	// (manual 2FA/session-token extraction, no `rclone authorize` support).
	HeadlessDifficult HeadlessSupport = "difficult"
	// HeadlessUnsupported: no known safe way to authenticate without a
	// browser/session on the server itself.
	HeadlessUnsupported HeadlessSupport = "unsupported"
)

// Backend identifies which transport actually moves bytes for a provider.
// Multiple providers can share one Backend (every object-storage preset
// shares BackendS3); the provider entry only adds the credential schema,
// documentation, and maturity label on top.
type Backend string

const (
	BackendLocal  Backend = "local"  // restic's local backend directly, no rclone involved
	BackendSFTP   Backend = "sftp"   // restic's native sftp backend directly, no rclone involved
	BackendS3     Backend = "s3"     // restic's native s3 backend (covers every S3-compatible provider)
	BackendAzure  Backend = "azure"  // restic's native azure backend
	BackendGS     Backend = "gs"     // restic's native Google Cloud Storage backend
	BackendSwift  Backend = "swift"  // restic's native OpenStack Swift backend
	BackendRclone Backend = "rclone" // restic's rclone backend -- required for file-sync cloud drives, which have no restic-native equivalent
)

// CredentialField describes one piece of input a provider's setup needs.
// Secret fields are never echoed back and are stored via internal/secrets;
// non-secret fields (endpoint, region, bucket) are stored in config.yaml.
type CredentialField struct {
	Key         string // storage.Options key (non-secret) or secret-store suffix (secret)
	Label       string
	Secret      bool
	Required    bool
	Default     string
	Placeholder string
}

// Provider is one catalog entry. See package doc for the scaling intent:
// everything provider-specific lives here, not scattered through switch
// statements elsewhere.
type Provider struct {
	ID          string // stable identifier, used in config.yaml and on the CLI (--provider <id>); never renamed once released
	DisplayName string
	Family      Family
	Backend     Backend
	// RcloneBackend is the rclone backend type string (e.g. "drive",
	// "onedrive", "dropbox") when Backend == BackendRclone; empty otherwise.
	RcloneBackend string
	Maturity      Maturity
	Auth          AuthMethod
	Headless      HeadlessSupport
	Experimental  bool
	// RequiresOwnOAuthApp is true for providers where ABM must not rely on
	// rclone's shared/default OAuth client (currently Google Drive): the
	// user is guided to create their own Client ID/Secret.
	RequiresOwnOAuthApp bool
	RequiredFields      []CredentialField
	OptionalFields      []CredentialField
	DocPath             string // path under docs/providers/, relative to repo root
	KnownLimitations    []string
	ReauthRequired      bool // periodic reauthentication is a normal part of using this provider, not a failure
	ImmutabilityCapable bool // the backend itself can be configured for WORM/append-only/object-lock storage
	Unsupported         bool // true only for catalog entries that exist purely to explain why they're not offered (e.g. Sync.com)
	UnsupportedReason   string
}

package provider

// AuthUserPass covers providers (MEGA, legacy Jottacloud) whose rclone
// backend takes a plain username/password rather than OAuth or an API key.
const AuthUserPass AuthMethod = "username-password"

// rcloneRemoteField is the one field every rclone-backed provider that's
// authenticated via a separately-run `rclone config`/`rclone authorize`
// needs at the `abm storage add` step: which already-configured remote to
// register. Google Drive additionally requires its own OAuth app
// credentials (see below); every other OAuth cloud-drive provider's only
// required field at this step is the remote name.
func rcloneRemoteField() CredentialField {
	return CredentialField{Key: "remote", Label: "rclone remote name (create it first with 'rclone config')", Required: true}
}

// s3CredentialFields is the single shared field schema for every
// S3-compatible preset: one engine, many presets, per the project's
// "do not create independent implementations for AWS, Wasabi, IDrive, etc."
// requirement. endpointDefault is pre-filled (and, for single-endpoint
// providers, effectively fixed); leave it empty for providers like AWS S3
// where the endpoint varies by region and bucket.
func s3CredentialFields(endpointDefault string, endpointRequired bool) ([]CredentialField, []CredentialField) {
	required := []CredentialField{
		{Key: "bucket", Label: "Bucket name", Required: true},
		{Key: "access_key", Label: "Access key ID", Secret: true, Required: true},
		{Key: "secret_key", Label: "Secret access key", Secret: true, Required: true},
	}
	if endpointRequired {
		required = append([]CredentialField{{Key: "endpoint", Label: "Endpoint", Required: true, Default: endpointDefault}}, required...)
	}
	optional := []CredentialField{
		{Key: "region", Label: "Region (if applicable)"},
		{Key: "path_prefix", Label: "Path prefix within the bucket"},
	}
	if !endpointRequired {
		optional = append([]CredentialField{{Key: "endpoint", Label: "Endpoint override (optional)", Default: endpointDefault}}, optional...)
	}
	return required, optional
}

// Registry is the full provider catalog. Order here only affects display
// order in `abm storage providers`; lookups are by ID via Get().
var Registry = buildRegistry()

func buildRegistry() []Provider {
	var reg []Provider

	// ---- Cloud drives (rclone OAuth backends) ----

	reg = append(reg, Provider{
		ID: "google-drive", DisplayName: "Google Drive", Family: FamilyCloudDrive,
		Backend: BackendRclone, RcloneBackend: "drive",
		Maturity: MaturityStable, Auth: AuthOAuth, Headless: HeadlessTokenTransfer,
		RequiresOwnOAuthApp: true,
		// Only "remote" is required at the abm storage add step: the OAuth
		// Client ID/Secret are needed once, during the separate `rclone
		// config`/`rclone authorize` step that creates the remote (see
		// docs/providers/GOOGLE-DRIVE.md) -- ABM doesn't need to know or
		// store them afterward, since the remote already holds the
		// resulting token.
		RequiredFields: []CredentialField{rcloneRemoteField()},
		DocPath:        "docs/providers/GOOGLE-DRIVE.md",
		KnownLimitations: []string{
			"Do not use rclone's shared/default Google OAuth client; Google has been restricting it and it is not suitable for unattended production use. Create your own OAuth Client ID/Secret (see doc).",
			"Not immutable/WORM storage: a compromised OAuth token can delete backups.",
		},
	})

	reg = append(reg, Provider{
		ID: "onedrive", DisplayName: "Microsoft OneDrive", Family: FamilyCloudDrive,
		Backend: BackendRclone, RcloneBackend: "onedrive",
		Maturity: MaturityStable, Auth: AuthOAuth, Headless: HeadlessTokenTransfer,
		RequiredFields: []CredentialField{rcloneRemoteField()},
		DocPath:        "docs/providers/ONEDRIVE.md",
		KnownLimitations: []string{
			"Business/SharePoint-backed drives under restrictive tenant policies may require a custom Azure AD app registration instead of rclone's default client.",
			"Not immutable/WORM storage.",
		},
	})

	reg = append(reg, Provider{
		ID: "dropbox", DisplayName: "Dropbox", Family: FamilyCloudDrive,
		Backend: BackendRclone, RcloneBackend: "dropbox",
		Maturity: MaturityStable, Auth: AuthOAuth, Headless: HeadlessTokenTransfer,
		RequiredFields:   []CredentialField{rcloneRemoteField()},
		DocPath:          "docs/providers/DROPBOX.md",
		KnownLimitations: []string{"Not immutable/WORM storage."},
	})

	reg = append(reg, Provider{
		ID: "box", DisplayName: "Box", Family: FamilyCloudDrive,
		Backend: BackendRclone, RcloneBackend: "box",
		Maturity: MaturitySupported, Auth: AuthOAuthJWT, Headless: HeadlessTokenTransfer,
		RequiredFields: []CredentialField{rcloneRemoteField()},
		DocPath:        "docs/providers/BOX.md",
		KnownLimitations: []string{
			"Business/admin-managed Box accounts may require an admin-approved JWT app configuration instead of interactive OAuth.",
			"Not immutable/WORM storage.",
		},
	})

	reg = append(reg, Provider{
		ID: "pcloud", DisplayName: "pCloud", Family: FamilyCloudDrive,
		Backend: BackendRclone, RcloneBackend: "pcloud",
		Maturity: MaturitySupported, Auth: AuthOAuth, Headless: HeadlessTokenTransfer,
		RequiredFields: []CredentialField{rcloneRemoteField()},
		DocPath:        "docs/providers/PCLOUD.md",
		KnownLimitations: []string{
			"EU-region accounts must use pCloud's EU hostname; verify this during `rclone config` setup.",
			"Not immutable/WORM storage.",
		},
	})

	reg = append(reg, Provider{
		ID: "mega", DisplayName: "MEGA", Family: FamilyCloudDrive,
		Backend: BackendRclone, RcloneBackend: "mega",
		Maturity: MaturitySupported, Auth: AuthUserPass, Headless: HeadlessDirect,
		RequiredFields: []CredentialField{
			rcloneRemoteField(),
			{Key: "username", Label: "MEGA account email", Required: true},
			{Key: "password", Label: "MEGA account password", Secret: true, Required: true},
		},
		DocPath: "docs/providers/MEGA.md",
		KnownLimitations: []string{
			"Uses a direct username/password login (no OAuth); rotate the password if it is ever exposed.",
			"MEGA's rclone backend has historically been more fragile under heavy parallel transfer than mature OAuth backends; keep concurrency modest.",
			"Not immutable/WORM storage. Distinct from MEGA S4 object storage (see that entry) -- consumer MEGA and MEGA S4 are different products with different backends.",
		},
	})

	reg = append(reg, Provider{
		ID: "jottacloud", DisplayName: "Jottacloud", Family: FamilyCloudDrive,
		Backend: BackendRclone, RcloneBackend: "jottacloud",
		Maturity: MaturitySupported, Auth: AuthSession, Headless: HeadlessDifficult,
		RequiredFields: []CredentialField{rcloneRemoteField()},
		DocPath:        "docs/providers/JOTTACLOUD.md",
		KnownLimitations: []string{
			"No `rclone authorize` support: you must generate a personal login token from the Jottacloud web UI's security settings and paste it into `rclone config` yourself, even on a headless server.",
			"Not immutable/WORM storage.",
		},
	})

	reg = append(reg, Provider{
		ID: "icloud-drive", DisplayName: "Apple iCloud Drive", Family: FamilyCloudDrive,
		Backend: BackendRclone, RcloneBackend: "iclouddrive",
		Maturity: MaturityExperimental, Experimental: true,
		Auth: AuthSession, Headless: HeadlessDifficult, ReauthRequired: true,
		RequiredFields: []CredentialField{rcloneRemoteField()},
		DocPath:        "docs/providers/ICLOUD.md",
		KnownLimitations: []string{
			"EXPERIMENTAL upstream rclone backend as of the pinned rclone version.",
			"Requires Apple ID authentication and 2FA; app-specific passwords are not a reliable substitute.",
			"The resulting trust/session token can expire and require periodic manual reauthentication -- this is not a bug, it is how Apple's session model works. Unattended indefinite operation is not guaranteed.",
			"Headless VPS setup is meaningfully harder than OAuth providers; expect to complete initial auth from a machine where you can respond to 2FA.",
			"Not immutable/WORM storage.",
		},
	})

	reg = append(reg, Provider{
		ID: "proton-drive", DisplayName: "Proton Drive", Family: FamilyCloudDrive,
		Backend: BackendRclone, RcloneBackend: "protondrive",
		Maturity: MaturityExperimental, Experimental: true,
		Auth: AuthSession, Headless: HeadlessDifficult, ReauthRequired: true,
		RequiredFields: []CredentialField{rcloneRemoteField()},
		DocPath:        "docs/providers/PROTON-DRIVE.md",
		KnownLimitations: []string{
			"EXPERIMENTAL: rclone's Proton Drive support relies on a reverse-engineered, non-public API, not an official Proton SDK/API.",
			"Treat any upstream rclone bump touching this backend as higher-risk; verify changelogs before upgrading the pinned rclone version.",
			"Session/auth may require periodic reauthentication.",
			"Not immutable/WORM storage.",
		},
	})

	reg = append(reg, Provider{
		ID: "sync-com", DisplayName: "Sync.com", Family: FamilyCloudDrive,
		Unsupported: true, Maturity: MaturityUnavailable,
		UnsupportedReason: "As of the versions of rclone this project pins, rclone has no Sync.com backend, and Sync.com does not publish a public API, S3-compatible, or WebDAV interface suitable for an unattended backup client. Implementing this via browser automation or reverse-engineered credentials would be fragile and is explicitly out of scope. This can become a real provider entry if/when a supported backend exists upstream -- no other code changes would be needed.",
		DocPath:           "docs/providers/UNSUPPORTED.md",
	})

	// ---- Object storage (restic's native S3 backend, one engine) ----

	reg = append(reg, s3Preset("aws-s3", "Amazon S3", "", false, MaturityStable, "docs/providers/S3.md"))
	reg = append(reg, s3Preset("backblaze-b2", "Backblaze B2 (S3-compatible API)", "s3.us-west-002.backblazeb2.com", true, MaturityStable, "docs/providers/BACKBLAZE-B2.md"))
	reg = append(reg, s3Preset("wasabi", "Wasabi", "s3.wasabisys.com", true, MaturityStable, "docs/providers/WASABI.md"))
	reg = append(reg, s3Preset("idrive-e2", "IDrive e2", "", true, MaturitySupported, "docs/providers/IDRIVE-E2.md"))
	reg = append(reg, s3Preset("cloudflare-r2", "Cloudflare R2", "", true, MaturitySupported, "docs/providers/CLOUDFLARE-R2.md"))
	reg = append(reg, s3Preset("hetzner-object-storage", "Hetzner Object Storage", "", true, MaturitySupported, "docs/providers/HETZNER-OBJECT-STORAGE.md"))
	reg = append(reg, s3Preset("digitalocean-spaces", "DigitalOcean Spaces", "", true, MaturitySupported, "docs/providers/DIGITALOCEAN-SPACES.md"))
	reg = append(reg, s3Preset("storj", "Storj (S3 gateway)", "gateway.storjshare.io", true, MaturitySupported, "docs/providers/STORJ.md"))
	reg = append(reg, s3Preset("mega-s4", "MEGA S4 (object storage)", "", true, MaturitySupported, "docs/providers/MEGA-S4.md"))
	reg = append(reg, s3Preset("minio", "MinIO (self-hosted)", "", true, MaturityStable, "docs/providers/S3.md"))
	reg = append(reg, s3Preset("generic-s3", "Generic S3-compatible", "", true, MaturityStable, "docs/providers/S3.md"))

	// IDrive e2 is specifically its object-storage product, not the
	// consumer IDrive backup app (which has no supported integration path).
	for i := range reg {
		if reg[i].ID == "idrive-e2" {
			reg[i].KnownLimitations = append(reg[i].KnownLimitations,
				"This targets IDrive e2 object storage specifically, not the ordinary consumer IDrive backup service, which has no supported ABM integration.")
		}
		if reg[i].ID == "mega-s4" {
			reg[i].KnownLimitations = append(reg[i].KnownLimitations,
				"MEGA S4 is a distinct object-storage product from consumer MEGA file storage (see the \"mega\" provider) -- different backend, different architecture, not interchangeable.")
		}
	}

	reg = append(reg, Provider{
		ID: "azure-blob", DisplayName: "Microsoft Azure Blob Storage", Family: FamilyObjectStorage,
		Backend: BackendAzure, Maturity: MaturitySupported, Auth: AuthAPIKey, Headless: HeadlessDirect,
		RequiredFields: []CredentialField{
			{Key: "account_name", Label: "Storage account name", Required: true},
			{Key: "account_key", Label: "Storage account key", Secret: true, Required: true},
			{Key: "container", Label: "Container name", Required: true},
		},
		DocPath:          "docs/providers/AZURE-BLOB.md",
		KnownLimitations: []string{"Uses restic's native azure backend directly (no rclone involved)."},
	})

	reg = append(reg, Provider{
		ID: "google-cloud-storage", DisplayName: "Google Cloud Storage", Family: FamilyObjectStorage,
		Backend: BackendGS, Maturity: MaturitySupported, Auth: AuthAPIKey, Headless: HeadlessDirect,
		RequiredFields: []CredentialField{
			{Key: "project_id", Label: "GCP project ID", Required: true},
			{Key: "bucket", Label: "Bucket name", Required: true},
			{Key: "credentials_file", Label: "Path to a service-account JSON key file", Required: true},
		},
		DocPath:          "docs/providers/GOOGLE-CLOUD-STORAGE.md",
		KnownLimitations: []string{"Uses restic's native gs backend directly (no rclone involved). The service-account JSON key file itself must be protected with the same care as any other credential."},
	})

	reg = append(reg, Provider{
		ID: "openstack-swift", DisplayName: "OpenStack Swift", Family: FamilyObjectStorage,
		Backend: BackendSwift, Maturity: MaturitySupported, Auth: AuthAPIKey, Headless: HeadlessDirect,
		RequiredFields: []CredentialField{
			{Key: "auth_url", Label: "Identity (Keystone) auth URL", Required: true},
			{Key: "username", Label: "Username", Required: true},
			{Key: "password", Label: "Password/API key", Secret: true, Required: true},
			{Key: "container", Label: "Container name", Required: true},
		},
		OptionalFields:   []CredentialField{{Key: "tenant", Label: "Tenant/project name"}, {Key: "region", Label: "Region"}},
		DocPath:          "docs/providers/SWIFT.md",
		KnownLimitations: []string{"Uses restic's native swift backend directly (no rclone involved). Field names vary across OpenStack/Oracle-compatible deployments; see the doc for the exact environment variables restic maps these to."},
	})

	// ---- SFTP (restic's native backend, not rclone) ----

	reg = append(reg, Provider{
		ID: "sftp", DisplayName: "SFTP / Storage Server", Family: FamilySFTP,
		Backend: BackendSFTP, Maturity: MaturityStable, Auth: AuthSSHKey, Headless: HeadlessDirect,
		RequiredFields: []CredentialField{
			{Key: "host", Label: "Hostname", Required: true},
			{Key: "username", Label: "Username", Required: true},
		},
		OptionalFields: []CredentialField{
			{Key: "port", Label: "Port", Default: "22"},
			{Key: "key_file", Label: "SSH private key file (preferred)"},
			{Key: "password", Label: "Password (discouraged; prefer a key)", Secret: true},
			{Key: "path_prefix", Label: "Remote path prefix"},
		},
		DocPath: "docs/providers/SFTP.md",
		KnownLimitations: []string{
			"Uses restic's native sftp backend directly (via the system `ssh`/`sftp` client), not rclone -- simpler and one less moving part for a backend restic has supported natively since its earliest releases.",
			"Host key verification is never silently disabled; the host must already be a known host for the account running abm (or configured via SSH config), exactly as plain `sftp`/`ssh` would require.",
		},
	})

	// ---- Local / external disk ----

	reg = append(reg, Provider{
		ID: "local", DisplayName: "Local / External Disk", Family: FamilyLocal,
		Backend: BackendLocal, Maturity: MaturityStable, Auth: AuthNone, Headless: HeadlessDirect,
		RequiredFields: []CredentialField{{Key: "path", Label: "Destination directory", Required: true}},
		DocPath:        "docs/providers/LOCAL.md",
		KnownLimitations: []string{
			"A permanently attached local or external disk is not sufficient as the only protection against ransomware, fire, theft, or total server loss -- pair it with an off-site destination.",
			"ABM detects and fails loudly (never reports false success) if the path is unavailable at backup time, e.g. an external drive left unplugged.",
		},
	})

	// ---- Generic rclone passthrough ----

	reg = append(reg, Provider{
		ID: "rclone-existing", DisplayName: "Use an Existing rclone Remote", Family: FamilyGenericRclone,
		Backend: BackendRclone, Maturity: MaturitySupported, Auth: AuthExisting, Headless: HeadlessDirect,
		RequiredFields: []CredentialField{{Key: "remote", Label: "Name of an already-configured rclone remote", Required: true}},
		DocPath:        "docs/providers/GENERIC-RCLONE.md",
		KnownLimitations: []string{
			"ABM does not know this remote's backend maturity; a capability/round-trip smoke test (internal/backend) is run before it is accepted, but ABM cannot promise every rclone backend behaves well as a restic repository target.",
			"This is the escape hatch that lets ABM support any current or future rclone backend without a code change: add the remote with `rclone config`, then point a job at it here.",
		},
	})

	return reg
}

func s3Preset(id, displayName, endpointDefault string, endpointRequired bool, maturity Maturity, docPath string) Provider {
	required, optional := s3CredentialFields(endpointDefault, endpointRequired)
	return Provider{
		ID: id, DisplayName: displayName, Family: FamilyObjectStorage,
		Backend: BackendS3, Maturity: maturity, Auth: AuthAPIKey, Headless: HeadlessDirect,
		RequiredFields: required, OptionalFields: optional,
		DocPath:             docPath,
		ImmutabilityCapable: true,
		KnownLimitations: []string{
			"Uses restic's native s3 backend directly (no rclone involved) for a simpler, single transport path shared by every S3-compatible preset.",
			"Object Lock/immutability (where the provider supports it) must be enabled on the bucket itself before use; see docs/IMMUTABILITY.md -- ABM does not configure it for you, and it changes how `restic prune` can behave.",
		},
	}
}

// Get returns the provider with the given ID, or false if none matches.
func Get(id string) (Provider, bool) {
	for _, p := range Registry {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

// ByFamily groups the catalog for the setup wizard's category menu, in
// registry order within each family, skipping unsupported entries (those
// are surfaced separately via Unsupported()).
func ByFamily(f Family) []Provider {
	var out []Provider
	for _, p := range Registry {
		if p.Family == f && !p.Unsupported {
			out = append(out, p)
		}
	}
	return out
}

// Unsupported returns catalog entries that exist only to document why a
// commonly-requested service isn't offered.
func Unsupported() []Provider {
	var out []Provider
	for _, p := range Registry {
		if p.Unsupported {
			out = append(out, p)
		}
	}
	return out
}

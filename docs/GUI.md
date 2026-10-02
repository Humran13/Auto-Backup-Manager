# Web GUI

The web GUI is Auto-Backup-Manager's normal setup and recovery workflow. It
calls the same typed Go services used by the advanced CLI; it never shells out
to `abm` commands.

## First run

The one-command installer starts the GUI. Windows opens
`http://127.0.0.1:8765` automatically. Linux installs a localhost-only
systemd service and prints an SSH-tunnel command for authenticated VPS access.

The wizard walks through:

1. organization and device name;
2. a meaningful backup/project name;
3. one or more source folders selected in the server filesystem browser;
4. a local/mounted destination (other providers remain available on Storage);
5. retention review;
6. a real destination write/read/delete probe;
7. the first encrypted backup and recovery-point verification.

The file picker always browses the machine running ABM. It never uses an HTML
file input, so a remote VPS session cannot accidentally browse the
administrator's laptop. Windows drive roots and Linux `/` navigation are
provided, with optional absolute-path entry under Advanced.

## Selective backups

A backup is a project or application and can include multiple folders/files.
Human-readable names such as `Motion Ventures Website` are supported. Docker
Compose inspection suggests the compose file, `.env`, and bind-mounted data;
it never silently includes `/var/lib/docker` or replaceable image/cache data.
Named volumes are reported for review, and live databases should use the
database-dump form.

MySQL/MariaDB/Percona, PostgreSQL, and SQLite are configured within the job
form. Test Connection performs the same consistent logical dump used by a
backup. Database passwords are write-only and stored in the platform secret
store, never returned by an API.

## Storage

Provider forms are generated from the shared provider registry. S3-compatible,
SFTP, Azure, GCS, Swift, local storage, and MEGA are configured directly in
the GUI. Secret inputs are password masked and API responses contain only
non-secret options.

Google Drive, OneDrive, and Dropbox expose a graphical Connect flow. ABM runs
rclone's OAuth authorization internally, shows the temporary authorization
link/progress, stores the resulting token in ABM's protected rclone config,
and never displays the token. Google requires the user's own Desktop OAuth
Client ID and Client Secret. The browser UI and state boundary are automated;
real provider login is optional and requires credentials that are not present
in public CI.

On desktop Windows, rclone can open the provider authorization page locally.
On a headless VPS, OAuth callback behavior is limited by the provider/rclone
localhost callback model; see the provider-specific documentation. No fake
successful login is presented.

## Recovery points and restore

The GUI calls versions “Recovery Points.” Opening one shows its restic tree
and a recovery manifest containing organization, device, job, original source
paths, and database metadata. Users can restore the whole project or selected
files/folders.

The default target is a new timestamped directory under ABM state. “Restore to
Original Locations” is an advanced option that lists the original paths and
requires both a warning confirmation and typing `RESTORE ORIGINAL`. Restores
use restic content verification. On Windows, a known restic timestamp-only
error for synthetic multi-source ancestor directories is treated as metadata
warning only after content verification; file/verification errors remain
fatal.

## Security model

- Listener and middleware both enforce loopback-only access.
- Mutations require an unpredictable per-process CSRF token.
- Windows secrets use DPAPI; Linux secrets use root-only files.
- Saved secrets/tokens are never returned, logged, or embedded in URLs.
- VPS access uses the existing authenticated SSH channel; ABM is not exposed
  unauthenticated to the network.
- The GUI service runs at boot. Backup schedules remain OS-native systemd
  timers/Windows Scheduled Tasks.

## Browser verification

`npm run test:browser` runs Playwright against a real ABM process and real
restic repository. Any browser console error or uncaught exception fails the
test. See [TESTING.md](TESTING.md) for coverage and cloud-auth limitations.

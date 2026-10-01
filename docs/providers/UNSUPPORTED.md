# Unsupported Providers

These are services people commonly ask for that Auto-Backup-Manager
deliberately does not offer an integration for, and why. This list exists so
"why isn't X here" has a documented, specific answer instead of silence —
and so each entry can become a real provider the moment a safe path exists,
by adding a registry entry (see `internal/provider`), not by redesigning
anything.

## Sync.com

No officially supported path exists as of the rclone/restic versions this
project pins: rclone has no Sync.com backend, and Sync.com does not publish
a public API, S3-compatible endpoint, or WebDAV interface suitable for an
unattended backup client.

**What ABM will not do instead:** screen scraping, browser automation, or
reverse-engineered credential handling to force a connection anyway. Those
approaches are fragile, break silently on any UI change, and fall outside
this project's security posture for credential handling.

**How this could change:** if rclone adds official Sync.com support, or
Sync.com publishes a supported API, add a `provider.Provider` entry
pointing at it — the architecture doesn't need to change.

## Ordinary IDrive consumer backup accounts

IDrive's **consumer backup service** account is a different product from
**IDrive e2** (object storage), which *is* supported — see
[IDRIVE-E2.md](IDRIVE-E2.md). The consumer backup service doesn't expose an
API suited to being a restic repository target; e2 does, via its
S3-compatible interface.

## Anything requiring a GUI/desktop client as the only access method

A provider whose only supported access method is its own proprietary desktop
sync client (no API, no S3/SFTP/WebDAV compatibility, no rclone backend)
cannot be safely integrated as an unattended backup destination and will not
be added by automating that client's UI.

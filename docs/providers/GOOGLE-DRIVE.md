# Google Drive

## What you need

A Google account, and **your own OAuth Client ID and Client Secret** — do
not rely on rclone's shared/default client ID, which Google has been
retiring access for and which is rate-limited across every rclone user
globally. Create your own:

1. Go to the [Google Cloud Console](https://console.cloud.google.com/), create a project.
2. Enable the **Google Drive API** for that project.
3. Configure the OAuth consent screen (Internal if using Google Workspace, External + "Testing" otherwise — Testing mode's refresh tokens are enough for a single backup account).
4. Create an **OAuth client ID** of type "Desktop app".
5. Note the Client ID and Client Secret.

## Authenticating

**With a browser on this machine:**

```bash
rclone config --config /etc/auto-backup-manager/rclone.conf
# n) New remote -> name it (e.g. gdrive-primary) -> type "drive"
# paste your Client ID / Client Secret when prompted
# follow the browser OAuth flow
```

**Headless VPS (no browser on the server):** run `rclone authorize` with your
client ID/secret on *any* machine with a browser (your laptop), then paste
the resulting token on the VPS:

```bash
# on your laptop:
rclone authorize "drive" "<client-id>" "<client-secret>"
# complete the browser flow; copy the printed token JSON

# on the VPS:
rclone config --config /etc/auto-backup-manager/rclone.conf
# n) New remote -> type "drive" -> same client ID/secret -> when asked for
# the token, paste the JSON from the laptop step instead of using "y" to
# auto-authorize
```

`internal/rclone.Runner.AuthorizeOAuth`/`CreateOAuthRemote` wrap this same
flow programmatically; a fully scripted `abm storage add --provider google-drive
--headless` is not implemented yet — use `rclone config` directly as above,
then register it:

```bash
abm storage add --provider google-drive --name gdrive-primary --remote gdrive-primary
```

## Testing

```bash
abm storage test gdrive-primary
```

## Security limitations

Google Drive via OAuth is **not immutable/WORM storage**. Anyone with the
stored OAuth token (or your Google account credentials) can delete or
encrypt files in that Drive, including your backups. It's a convenient way
to get started, not a defense against ransomware or a compromised account —
see [THREAT-MODEL.md](THREAT-MODEL.md). For stronger resilience, pair it
with (or replace it with) an Object-Lock-capable S3-compatible destination;
see [S3.md](S3.md).

## Disconnecting / revoking access

Revoke the app's access at
[myaccount.google.com/permissions](https://myaccount.google.com/permissions),
and delete the corresponding remote from
`/etc/auto-backup-manager/rclone.conf` (or
`C:\ProgramData\Auto-Backup-Manager\rclone.conf` on Windows).

# Microsoft OneDrive

## What you need

A Microsoft account (Personal) or a Microsoft 365/Business account where an
admin allows app registrations (Business). rclone's built-in OneDrive OAuth
client works for most accounts without creating your own app registration,
but Business/Sharepoint-backed OneDrive accounts with restrictive tenant
policies may require a custom Azure AD app registration — see rclone's own
OneDrive documentation if the default client is blocked by your tenant.

## Authenticating

**With a browser on this machine:**

```bash
rclone config --config /etc/auto-backup-manager/rclone.conf
# n) New remote -> name it (e.g. onedrive-primary) -> type "onedrive"
# follow the browser OAuth flow; choose Personal or Business when prompted
```

**Headless VPS:** same pattern as [Google Drive](GOOGLE-DRIVE.md#authenticating):
run `rclone authorize "onedrive"` on a machine with a browser, paste the
resulting token JSON into `rclone config` on the VPS.

Then register it:

```bash
abm storage add --type onedrive --name onedrive-primary --remote onedrive-primary
```

## Testing

```bash
abm storage test onedrive-primary
```

## Security limitations

Same caveat as Google Drive: standard OAuth access is not immutable/WORM
storage. See [THREAT-MODEL.md](THREAT-MODEL.md).

## Disconnecting / revoking access

Revoke the app's access from your Microsoft account's
[app permissions page](https://account.live.com/consent/Manage), and delete
the corresponding remote from `rclone.conf`.

# Dropbox

## What you need

A Dropbox account. rclone's built-in Dropbox OAuth client is sufficient for
normal use.

## Authenticating

```bash
rclone config --config /etc/auto-backup-manager/rclone.conf
# n) New remote -> name it (e.g. dropbox-primary) -> type "dropbox"
# follow the browser OAuth flow
```

**Headless VPS:** same pattern as [Google Drive](GOOGLE-DRIVE.md#authenticating) —
`rclone authorize "dropbox"` on a machine with a browser, paste the token
into `rclone config` on the VPS.

Then register it:

```bash
abm storage add --provider dropbox --name dropbox-primary --remote dropbox-primary
```

## Testing

```bash
abm storage test dropbox-primary
```

## Security limitations

Same caveat as the other OAuth providers: not immutable/WORM storage. See
[THREAT-MODEL.md](THREAT-MODEL.md).

## Disconnecting / revoking access

Revoke the app's access from Dropbox's
[connected apps settings](https://www.dropbox.com/account/connected_apps),
and delete the corresponding remote from `rclone.conf`.

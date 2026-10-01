# MEGA

| | |
|---|---|
| **Provider ID** | `mega` |
| **Maturity** | Supported (implemented, not yet validated against a real MEGA account by this project) |
| **Backend** | rclone `mega` |
| **Auth** | Username + password (no OAuth) |
| **Headless** | Direct |

**Distinct from MEGA S4** (object storage — see [MEGA-S4.md](MEGA-S4.md)).
This entry is consumer MEGA file storage.

## Setup

```bash
abm storage add --provider mega --name mega --username you@example.com
# prompts for the password on stdin, never a command-line argument
```

## Security considerations

Unlike every OAuth-based provider in this project, MEGA's rclone backend
authenticates with your actual account password (obscured, not encrypted, in
`rclone.conf`). Treat it with the same care as a plaintext credential, and
rotate it if `rclone.conf` is ever exposed. Consider a dedicated MEGA account
used only for backups rather than your primary one.

## Known limitations

MEGA's rclone backend has historically been reported as less robust under
heavy parallel transfer than mature OAuth-based backends; this project has
not independently benchmarked it. Not immutable/WORM storage.

## Testing

```bash
abm storage test mega
```

## Reconnect

Update the stored password with `abm storage add --provider mega --name mega
--username ...` again if it changes; there is no OAuth token to refresh.

## Restore test

```bash
abm restore <job> latest --target /tmp/restore-test
```

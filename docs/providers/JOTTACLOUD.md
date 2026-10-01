# Jottacloud

| | |
|---|---|
| **Provider ID** | `jottacloud` |
| **Maturity** | Supported (implemented, not yet validated against a real Jottacloud account by this project) |
| **Backend** | rclone `jottacloud` |
| **Auth** | Personal login token from the Jottacloud web UI |
| **Headless** | Difficult — no `rclone authorize` support |

## Important: no generic OAuth flow

Unlike Google Drive/OneDrive/Dropbox, Jottacloud's rclone backend does **not**
support `rclone authorize`. You must log into the Jottacloud website
yourself, generate a personal login token from its security settings, and
paste that token into `rclone config` — including on a headless server,
where you'll need to do this step from a browser elsewhere and transfer just
the token string.

## Setup

```bash
rclone config --config /etc/auto-backup-manager/rclone.conf
# n) New remote -> name it (e.g. jotta-primary) -> type "jottacloud"
# choose the "standard" auth type when prompted, and paste the login token
# generated from the Jottacloud website's security settings

abm storage add --provider jottacloud --name jotta-primary --remote jotta-primary
```

## Testing

```bash
abm storage test jotta-primary
```

## Security considerations

Not immutable/WORM storage.

## Reconnect

The login token can expire or be revoked; repeat the setup steps above to
generate a fresh one when `abm storage test` starts failing with an auth
error.

## Restore test

```bash
abm restore <job> latest --target /tmp/restore-test
```

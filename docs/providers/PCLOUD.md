# pCloud

| | |
|---|---|
| **Provider ID** | `pcloud` |
| **Maturity** | Supported (implemented, not yet validated against a real pCloud account by this project) |
| **Backend** | rclone `pcloud` |
| **Auth** | Browser OAuth |
| **Headless** | Token transfer |

## Setup

```bash
rclone config --config /etc/auto-backup-manager/rclone.conf
# n) New remote -> name it (e.g. pcloud-primary) -> type "pcloud"
# follow the browser OAuth flow

abm storage add --provider pcloud --name pcloud-primary --remote pcloud-primary
```

**Headless VPS:** `rclone authorize "pcloud"` on a machine with a browser,
paste the token into `rclone config` on the server.

## Known limitations

EU-region pCloud accounts use a different API hostname than the default (US)
region; `rclone config`'s pcloud setup will prompt for this — answer
according to which region your account was created in.

## Testing

```bash
abm storage test pcloud-primary
```

## Security considerations

Not immutable/WORM storage.

## Reconnect

```bash
rclone config reconnect pcloud-primary: --config /etc/auto-backup-manager/rclone.conf
```

## Restore test

```bash
abm restore <job> latest --target /tmp/restore-test
```

# Box

| | |
|---|---|
| **Provider ID** | `box` |
| **Maturity** | Supported (implemented, not yet validated against a real Box account by this project) |
| **Backend** | rclone `box` |
| **Auth** | Browser OAuth, or an admin-configured JWT app for Business/admin-managed accounts |
| **Headless** | Token transfer (`rclone authorize` on another machine, paste the token) |

## Account type

Works with a personal Box account via browser OAuth. A Business/Enterprise
account under admin control may instead require an admin to set up a
JWT/service-account app in the Box Developer Console — Box documents this as
"Custom App" with "Server Authentication (with JWT)". If your organization
restricts third-party app authorization, you'll need admin involvement
regardless of what ABM does.

## Setup

```bash
rclone config --config /etc/auto-backup-manager/rclone.conf
# n) New remote -> name it (e.g. box-primary) -> type "box"
# follow the browser OAuth flow (or the JWT app flow for Business accounts)

abm storage add --provider box --name box-primary --remote box-primary
```

**Headless VPS:** `rclone authorize "box"` on a machine with a browser, paste
the resulting token into `rclone config` on the server.

## Testing

```bash
abm storage test box-primary
```

## Security considerations

Not immutable/WORM storage — standard OAuth access can be used to delete
files by anyone holding the token.

## Reconnect

```bash
rclone config reconnect box-primary: --config /etc/auto-backup-manager/rclone.conf
```

## Restore test

```bash
abm restore <job> latest --target /tmp/restore-test
```

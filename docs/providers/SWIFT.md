# OpenStack Swift

| | |
|---|---|
| **Provider ID** | `openstack-swift` |
| **Maturity** | Supported (implemented, not yet validated against a real Swift deployment by this project) |
| **Backend** | restic's native `swift` backend (not rclone) |
| **Auth** | Keystone username/password (+ tenant/region) |
| **Headless** | Direct |

Supports Swift-compatible deployments including some Oracle Cloud
configurations that expose a Swift-compatible endpoint, in addition to plain
OpenStack.

## What you need

- Your Keystone identity (auth) URL
- Username and password/API key
- A container name
- Tenant/project name and region, if your deployment requires them

## Setup

```bash
abm storage add --provider openstack-swift --name swift \
    --auth-url https://keystone.example.com/v3 \
    --username <user> --password <password> --container <container> \
    --tenant <tenant> --region <region>
```

## Security considerations

Use an application credential or scoped API key instead of a full account
password where your deployment supports it.

## Known limitations

Field names and required values vary meaningfully across OpenStack/Oracle
deployments; consult restic's current documentation for the exact `OS_*`
environment variables your deployment needs if the fields above aren't
sufficient.

# Microsoft Azure Blob Storage

| | |
|---|---|
| **Provider ID** | `azure-blob` |
| **Maturity** | Supported (implemented, not yet validated against a real Azure account by this project) |
| **Backend** | restic's native `azure` backend (not rclone) |
| **Auth** | Storage account name + account key |
| **Headless** | Direct |

## What you need

- A storage account name and a container within it
- A storage account key (or, per current restic documentation, a SAS token if preferred — check restic's pinned-version docs for exact support)

## Setup

```bash
abm storage add --provider azure-blob --name azure \
    --account-name <storage-account> --account-key <key> --container <container>
```

## Security considerations

Prefer a scoped SAS token over the full account key where restic's pinned
version supports it, limiting blast radius if the credential leaks.

## Reconnect / restore test

Account keys don't expire like OAuth tokens; rotate and re-run
`abm storage add` with the same `--name` if needed.

```bash
abm restore <job> latest --target /tmp/restore-test
```

## Known limitations

No Object Lock/immutability integration documented here yet; Azure does
support immutable blob storage policies, but this project has not evaluated
their interaction with `restic prune` — see [../IMMUTABILITY.md](../IMMUTABILITY.md)
for the general caution that applies to any provider's lock feature.

# Storj (S3 Gateway)

| | |
|---|---|
| **Provider ID** | `storj` |
| **Maturity** | Supported (implemented, not yet validated against a real Storj account by this project) |
| **Backend** | S3-compatible engine — see [S3.md](S3.md) |

Storj is a decentralized object-storage network; ABM connects through its
S3-compatible gateway rather than its native `uplink` protocol, so it works
through the same shared engine as every other object-storage preset.

## Setup

```bash
abm storage add --provider storj --name storj \
    --endpoint gateway.storjshare.io \
    --access-key <key> --secret-key <secret>
```

Generate S3-compatible gateway credentials from the Storj console; confirm
the current gateway hostname there, as Storj has offered both a shared
gateway and self-hosted gateway options.

## Security considerations

Scope the access grant to the specific bucket when creating gateway
credentials.

## Immutability

Storj's erasure-coded, distributed storage model is itself resistant to
single-point data loss, but this is different from Object Lock/WORM
semantics against a holder of valid credentials — see
[../IMMUTABILITY.md](../IMMUTABILITY.md) for the distinction.

## Reconnect / restore test

Same as [S3.md](S3.md).

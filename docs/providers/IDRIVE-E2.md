# IDrive e2

| | |
|---|---|
| **Provider ID** | `idrive-e2` |
| **Maturity** | Supported (implemented, not yet validated against a real IDrive e2 account by this project) |
| **Backend** | S3-compatible engine — see [S3.md](S3.md) |

**This targets IDrive e2 object storage specifically.** IDrive also sells an
unrelated consumer backup service ("IDrive"); that product has no supported
ABM integration and is not the same thing as e2.

## Setup

```bash
abm storage add --provider idrive-e2 --name idrivee2 \
    --endpoint <exact-endpoint-from-your-e2-console> \
    --access-key <key> --secret-key <secret>
```

IDrive e2 assigns a bucket-specific endpoint shown directly in its console
when you create the bucket — copy it exactly rather than guessing a pattern.

## Security considerations

Use a key scoped to the bucket if IDrive e2's console offers scoped keys.

## Object Lock

Check current IDrive e2 documentation for Object Lock support status at
setup time; see [../IMMUTABILITY.md](../IMMUTABILITY.md).

## Reconnect / restore test

Same as [S3.md](S3.md).

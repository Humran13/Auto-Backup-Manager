# Immutability / Ransomware Resilience

**An ordinary encrypted backup is not an immutable backup.** Restic encrypts
every repository, but encryption protects confidentiality, not the
repository's existence: anyone holding valid write credentials to the
destination — including ransomware or an attacker with root/admin on the
backup client itself — can delete or overwrite that encrypted data just as
easily as any other file they have write access to. This document is about
closing that specific gap, honestly, including where ABM does not (yet)
close it completely.

## The threat this addresses

> The original backup client may be compromised. If the VPS has credentials
> capable of deleting the entire backup repository, ransomware or a root
> compromise could destroy backup history.

See [THREAT-MODEL.md](THREAT-MODEL.md) for the full picture; this document
goes deep on this one scenario because it's the one where "we have backups"
can be quietly false by the time you need them.

## Two independent mechanisms, and how they interact

### 1. Object Lock / versioning on the destination (S3-compatible)

Backblaze B2, AWS S3, and some other S3-compatible providers support Object
Lock in **compliance** or **governance** mode: once an object is written and
locked, *nobody* — not even the account owner, not even someone holding
valid credentials — can delete or overwrite it until its retention period
expires.

**This is not a flip a switch and done feature for a restic repository.**
Understand the interaction before relying on it:

- restic's repository format is a set of immutable-by-convention pack files
  plus small index/snapshot/lock files that restic **does** rewrite and
  delete during normal operation (`forget`, and especially `prune`, which
  repacks and deletes pack files to reclaim space from removed snapshots).
- If every object in the bucket is under Object Lock, **`restic prune` will
  fail or be unable to actually reclaim space** until each locked object's
  retention period expires, because prune's whole job is deleting/rewriting
  objects that are supposed to be gone.
- Practical consequence: Object Lock and routine pruning are in tension.
  Either (a) accept that your bucket grows unboundedly within the lock
  window (no pruning until objects age out of lock), or (b) use a
  **non-destructive retention policy on that bucket** (`forget` without
  `prune`, so space isn't reclaimed but nothing contradicts the lock) and
  periodically start a fresh repository, or (c) lock only a secondary,
  less-frequently-pruned destination while a primary destination stays
  prunable normally.
- ABM does not configure Object Lock for you (it's a bucket-level setting
  via the provider's own console/API) and does not currently implement any
  special-cased pruning behavior for a locked destination. Enabling
  `--immutable` on a storage entry (`abm storage add --immutable`) is
  **advisory metadata only** — it marks the destination as lock-capable in
  `abm storage show`/`abm storage list`, it does not itself touch the bucket
  or change what `abm maintain` does.

### 2. Separate maintenance authority (restic's `rest-server --append-only`)

restic's own recommended answer to "the backup client shouldn't be able to
destroy history" is its [`rest-server`](https://github.com/restic/rest-server)
backend run in `--append-only` mode:

```
BACKUP CLIENT (this machine, possibly compromised later)
  - can create new snapshots (append)
  - CANNOT delete snapshots, forget, or prune
  - connects with a restricted credential

SEPARATE MAINTENANCE AUTHORITY (a different machine/account/credential)
  - connects to the same rest-server without --append-only restrictions
  - is the only thing that ever runs `restic forget --prune`
  - its credentials are never stored on the backup client
```

This is the architecturally cleanest answer to this exact threat: even if
the backup client is fully compromised, the attacker can add noise but
cannot destroy the history that's already there.

**Current ABM support: documented here, not implemented as a first-class
provider.** `rest-server` is reachable today only through the
[generic rclone remote](providers/GENERIC-RCLONE.md) path if rclone has a
compatible backend, or by pointing restic directly at a `rest:` repository
spec outside of ABM's provider registry (not currently wired into
`internal/backend`). Making `rest-server --append-only` a first-class ABM
provider — with the backup client's job using the restricted append-only
credential and a separate `abm maintain --destination ... --maintenance-credentials-ref ...`-style
path using an unrestricted one — is a natural next step for this
architecture but is **not implemented in this version**. Treat this section
as the documented design intent, not a current capability claim.

## What ABM does today

- Per-job, per-destination repository passwords (see [SECURITY.md](SECURITY.md)),
  so a leaked password never exposes other jobs' or destinations' history.
- Never silently reinitializes a repository that becomes unreachable (see
  [ARCHITECTURE.md](ARCHITECTURE.md)) — a destroyed-and-recreated repository
  would otherwise look deceptively like "just a new empty job."
- Multi-destination support (see the main README and `abm job add
  --destination`), so a primary destination's compromise doesn't
  necessarily take a secondary destination's independent history with it —
  **provided the secondary uses different credentials, ideally on a
  different account/provider, which ABM does not enforce for you.**

## What to actually do today for real ransomware resilience

1. Use an S3-compatible destination with Object Lock enabled on the bucket,
   understanding the prune tradeoff above, **or**
2. Run your own `restic rest-server --append-only` and point a job at it
   via the generic rclone/manual-`restic` path, keeping the unrestricted
   maintenance credential off the backup client entirely, **or**
3. At minimum, use a second destination (different provider, different
   credentials, ideally a different account) as described in the README's
   multi-destination section, so a single compromised credential can't reach
   every copy of your history.

**Do not consider plain OAuth-based cloud-drive storage (Google
Drive/OneDrive/Dropbox/Box/pCloud/MEGA/Jottacloud/iCloud/Proton) immutable
under any configuration** — none of those backends support Object Lock or
an append-only mode through ABM today.

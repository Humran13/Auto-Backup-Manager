# Web GUI

`abm gui` starts a local web interface as a user-friendly layer over the
exact same engine the CLI uses — `internal/job`, `internal/backend`,
`internal/provider`, `internal/doctor`, and `internal/scheduler` are called
directly by GUI request handlers, the same way CLI commands call them.
Nothing is implemented twice, and nothing the GUI does shells out to the
`abm` binary itself.

## Starting it

```bash
abm gui                 # http://127.0.0.1:8765, opens a browser where practical
abm gui --port 9000      # use a different port
abm gui --no-open        # print the URL only, don't launch a browser
```

Binds to `127.0.0.1` only, enforced twice: the listener itself only accepts
loopback connections, and every request is additionally checked against the
remote address as defense in depth. There is no built-in way to expose the
GUI on a LAN or public interface in this version — that is a deliberate
scope decision, not an oversight; see [SECURITY.md](SECURITY.md).

## Pages

| Page | What it does |
|---|---|
| Dashboard | Overall status, last success/failure per job, one-click "Back Up Now" |
| Storage | Provider registry browser (Cloud Drive / Object Storage / SFTP / Local / Other) and destination management |
| Backup Jobs | Create/edit/enable/disable/delete/run jobs |
| Snapshots | Per-job snapshot list with "latest" marked |
| Restore | Restore latest/specific snapshot, safe-by-default target directory |
| Schedule | Install/inspect the hourly systemd timer / Task Scheduler task |
| Databases | Dump-tool availability and which jobs have a database hook |
| Activity / Logs | Recent structured log entries (secrets redacted, same as `abm logs`) |
| System Health | `abm doctor`'s checks as a healthy/warning/error table |
| Settings | Device name, organization, log level (never secrets) |

## First-run setup wizard

Opening the GUI with no existing configuration launches a short wizard
(device name → organization → finish) that calls the same bootstrap logic as
`abm setup --non-interactive`. After it, you land on the Storage page to add
a destination and the Backup Jobs page to create your first job — this
project's wizard does not yet chain storage/job/first-backup into one
continuous flow the way the full CLI setup conceptually could; see
"Known limitations" below.

## Storage and providers

The Storage page renders directly from the provider registry
(`internal/provider`) — the same list `abm storage providers` prints. A new
provider added to the registry appears in the GUI automatically; there is no
separate GUI provider list to keep in sync.

- **Local / external disk**: name + folder path, then a real
  init/backup/restore capability test (`internal/backend.Probe`) before it's
  accepted — the same probe `abm storage test` runs.
- **S3-compatible / Azure / GCS / Swift**: a form built from the provider's
  declared required/optional fields; secret fields (access/secret keys,
  account keys) are password-masked inputs, sent once, and never echoed back
  by any API response afterward.
- **SFTP**: host/port/username/key file, same native-backend path as the
  CLI.
- **Cloud drives (Google Drive, OneDrive, Dropbox, Box, pCloud, MEGA,
  Jottacloud, iCloud Drive, Proton Drive)**: the GUI does **not** implement
  its own OAuth flow for these in this version. You still run
  `rclone config` once outside the GUI (see each provider's doc under
  [docs/providers/](providers/)) to create the remote, then enter its name
  in the GUI's "Configure / Connect" form — the same two-step pattern the
  CLI uses. Building a fully in-GUI OAuth flow (driving `rclone authorize`,
  capturing its auth URL, completing the token exchange) is a reasonable
  future improvement, not implemented here; see "Known limitations."
- **Experimental providers** (iCloud Drive, Proton Drive) are visibly marked
  Experimental in the provider grid, matching the registry's own maturity
  label.
- **Unsupported providers** (e.g. Sync.com) render disabled with their exact
  reason shown, never silently hidden.

## Backup progress

Running a backup starts it in the background and returns immediately; the
GUI polls `/api/runs/{id}` for status. The reported **stage** (e.g.
"Initializing repository", "Scanning files (no VSS)", "Completed") reflects
real log events emitted by the actual backup run, captured via a dedicated
`slog.Handler` attached to that run — it is not a simulated percentage.
restic's own fine-grained interim progress (`percent_done`, `bytes_done` from
its `--json` status lines) is not currently surfaced incrementally; the GUI
shows the real final result (files new/changed/unmodified, snapshot ID,
duration) once the run completes, which is honest but coarser than a live
progress bar. See "Known limitations."

## Restore safety

Exactly like the CLI: the default restore target is always a new, separate,
timestamped directory under the platform's state directory, never the
original source. In-place restore requires an explicit opt-in and a
confirmation click before the request is even sent.

## Security

- 127.0.0.1-only binding, checked twice (listener + per-request).
- CSRF token required on every mutating (non-GET) request, generated fresh
  per server process, obtained via `GET /api/csrf`, never embedded in
  page source.
- All input is validated the same way the CLI's equivalent command
  validates it (same `config.Validate`, same provider field checks).
- No endpoint executes an arbitrary shell command; every handler calls a
  specific internal Go function with specific, typed arguments.
- Secret values (repository passwords, OAuth tokens, cloud access/secret
  keys, database passwords) are never included in any API response, page,
  or log line.

## Known limitations

- No in-GUI OAuth flow for cloud-drive providers; `rclone config` is still a
  separate, one-time manual step for those.
- No "browse snapshot contents" file browser (restic `ls`-style) — restore
  by whole snapshot or `--include` pattern only, same as the CLI.
- No native OS folder-picker dialog for source paths (a web page running in
  a browser cannot browse an arbitrary server-side filesystem the way a
  desktop app could); paths are typed/pasted.
- Backup progress is stage-based (real events), not a live byte/percentage
  progress bar.
- The setup wizard covers device identity only; adding storage and a first
  job are separate pages, not additional wizard steps.
- No authenticated remote/LAN access mode exists at all in this version —
  intentionally out of scope, not partially built.
- **The frontend has not been visually verified in a real browser.**
  Everything it calls (every `/api/*` endpoint) is covered by automated
  HTTP-level tests, but nobody has opened `http://127.0.0.1:8765` in an
  actual browser window and clicked through it during this development
  session. Rendering/layout/visual bugs could exist even though the
  underlying API is solid — see [TESTING.md](TESTING.md).

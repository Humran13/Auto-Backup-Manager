# Disaster Recovery

This covers the scenario where the original VPS or Windows PC is completely
gone — hardware failure, provider termination, or total compromise — and you
are starting from a brand-new machine.

**Scope:** this restores selected files, databases, and configuration you
chose to back up. It does **not** recreate the operating system, bootloader,
or every installed application from a block-level image; v1 is not a
bare-metal imaging product (see [THREAT-MODEL.md](THREAT-MODEL.md)). Treat
restored data as the payload you rebuild a fresh OS install around, not as a
drop-in replacement for the old disk.

## Linux: fresh Ubuntu VPS

1. Install a fresh, supported Ubuntu version.
2. Install Auto-Backup-Manager: `curl -fsSL .../install.sh | sudo bash` (see [UBUNTU.md](UBUNTU.md)).
3. Configure the cloud provider you backed up to: `abm storage add ...` (see the relevant provider doc, e.g. [providers/S3.md](providers/S3.md)).
4. You will need the **repository password** from wherever it was recorded
   outside the lost machine (a password manager, a sealed envelope, etc.) —
   Auto-Backup-Manager intentionally never stores it anywhere the lost
   machine's own disk would have been the only copy. Recover the job with
   `--recover-existing` and the *original* repository path (its
   `org/device-id/job-name`, from your records — the new machine gets a
   fresh, different device-id, so this will not match the default):

   ```
   abm job add --name <job> --source <path> --destination <storage> \
       --repository-path <original-org>/<original-device-id>/<job> \
       --recover-existing
   ```

   You will be prompted for the password on stdin, never as a command-line
   argument, so it never ends up in shell history or a process listing.
5. `abm snapshots <job>` — confirm you can see the expected history.
6. `abm restore <job> latest --target /restore` (or an older snapshot ID) — restore files.
7. Restore any database dumps found in the restored files into a fresh
   database server, following [DATABASES.md](DATABASES.md) and
   [RESTORE.md](RESTORE.md)'s "Database restore" section.
8. Review whatever system inventory you captured (see below) to rebuild
   installed packages, systemd services, cron jobs, and firewall rules.
9. Bring services back online once their config and data are in place.

## Windows: replaced PC

The same flow applies, using [WINDOWS.md](WINDOWS.md) for installation and
`C:\...` paths. VSS is not relevant to restore (only to backup of a live,
locked file) so restore works identically whether or not the new machine's
backup process can use VSS.

## System inventory for rebuilding a fresh machine

Auto-Backup-Manager does not yet automate inventory collection into a single
command (a planned `abm doctor --inventory` or similar — see
[TESTING.md](TESTING.md) for current gaps). Capture it manually before
disaster strikes, and consider adding these paths as backup sources so the
inventory itself gets backed up automatically:

**Linux:**
```bash
lsb_release -a                        # OS version
dpkg --get-selections > packages.txt  # installed package inventory
systemctl list-unit-files --state=enabled > services.txt
crontab -l > crontab.txt
ufw status verbose > firewall.txt     # or iptables-save, per your setup
```

**Windows:**
```powershell
Get-ComputerInfo | Out-File os-version.txt
Get-Package | Out-File installed-software.txt   # best-effort; not every installer registers here
schtasks /Query /FO LIST /V | Out-File scheduled-tasks.txt
```

Add the directory holding these exports as a job source so they are backed
up with everything else.

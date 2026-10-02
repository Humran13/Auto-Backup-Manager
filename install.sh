#!/usr/bin/env bash
# Auto-Backup-Manager installer for Ubuntu/Linux.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/install.sh | sudo bash
#
# Safer alternative (recommended): download this script, read it, then run it:
#   curl -fsSLO https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/install.sh
#   less install.sh
#   sudo bash install.sh
#
# This script is idempotent: re-running it upgrades an existing install in
# place. It never touches /etc/auto-backup-manager/config.yaml, the secrets
# directory, or any backup repository. Uninstalling (abm uninstall) never
# deletes backup repositories either.
set -euo pipefail

# --- Pinned dependency versions -------------------------------------------
# Restic and rclone are pinned to known-good versions rather than "latest",
# so an upstream release never silently changes backup/restore behavior
# under an existing deployment. Bump these deliberately, testing each bump.
RESTIC_VERSION="0.19.1"
RCLONE_VERSION="1.75.1"

# ABM_VERSION pins an exact release tag (e.g. "v0.9.0-rc.1"), bypassing
# RELEASE.json entirely. Leave unset/"auto" to resolve the project's
# currently-approved release via RELEASE.json instead -- see resolve_abm_tag
# below for exactly why this project never queries GitHub's own
# /releases/latest API endpoint.
ABM_VERSION="${ABM_VERSION:-auto}"
# RELEASE_JSON_URL is overridable only so install_test.sh can point it at a
# local fixture file (via a file:// URL) without touching the network; real
# installs should never need to set this.
RELEASE_JSON_URL="${RELEASE_JSON_URL:-https://raw.githubusercontent.com/Humran13/Auto-Backup-Manager/main/RELEASE.json}"

REPO="Humran13/Auto-Backup-Manager"
BIN_DIR="/usr/local/bin"
CONFIG_DIR="/etc/auto-backup-manager"
STATE_DIR="/var/lib/auto-backup-manager"
LOG_DIR="/var/log/auto-backup-manager"
TMP_DIR=""
RESTIC_ARCH=""
RCLONE_ARCH=""

log()  { echo "[abm-install] $*"; }
fail() { echo "[abm-install] ERROR: $*" >&2; exit 1; }
require_cmd() { command -v "$1" >/dev/null 2>&1 || fail "required command '$1' not found"; }

# parse_json_field extracts a top-level string field's value from JSON text
# passed on stdin, without requiring jq (not guaranteed present on every
# target system). Deliberately minimal: RELEASE.json is maintainer-authored
# and flat, not arbitrary user input.
parse_json_field() {
  local field="$1"
  grep -o "\"${field}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | sed -E "s/.*:[[:space:]]*\"([^\"]*)\"/\1/" | head -n1
}

# resolve_abm_tag decides which release tag to install, in order:
#   1. ABM_VERSION, if explicitly set to something other than "auto"
#   2. RELEASE.json's "version" field, fetched from this repo's own main
#      branch
# It deliberately never calls GitHub's /repos/{repo}/releases/latest API:
# that endpoint 404s outright for a repository with no stable release at
# all, and even once one exists, it never returns a prerelease -- exactly
# the failure a real Windows install hit (see docs/TROUBLESHOOTING.md). A
# repo-local metadata file sidesteps both problems and never depends on an
# ephemeral GitHub Actions artifact URL. Prints the resolved tag on stdout,
# or nothing on failure -- callers must check for an empty result and fail
# with their own clear message rather than exposing a raw API response.
resolve_abm_tag() {
  if [ "$ABM_VERSION" != "auto" ]; then
    echo "$ABM_VERSION"
    return 0
  fi
  local json
  json="$(curl -fsSL "$RELEASE_JSON_URL" 2>/dev/null)" || return 1
  echo "$json" | parse_json_field "version"
}

# download_and_verify URL SHA256SUMS_URL FILENAME
# Downloads FILENAME and its published checksum file, and refuses to
# continue if the checksum doesn't match -- an installer that fetches
# arbitrary binaries over HTTPS without verifying them is exactly the supply
# chain risk this project's own threat model calls out. Fails closed: a
# missing asset, a missing checksum entry, or a mismatch are all fatal, with
# no insecure fallback.
download_and_verify() {
  local url="$1" sums_url="$2" filename="$3"
  log "downloading $filename"
  curl -fsSL -o "$TMP_DIR/$filename" "$url" || fail "download failed: $url"
  curl -fsSL -o "$TMP_DIR/SHA256SUMS" "$sums_url" || fail "checksum file download failed: $sums_url"
  ( cd "$TMP_DIR" && grep " $filename\$" SHA256SUMS | sha256sum -c - ) \
    || fail "checksum verification failed for $filename"
}

install_restic() {
  if command -v restic >/dev/null 2>&1 && restic version 2>/dev/null | grep -q "$RESTIC_VERSION"; then
    log "restic $RESTIC_VERSION already installed"
    return
  fi
  local base="restic_${RESTIC_VERSION}_linux_${RESTIC_ARCH}"
  local url="https://github.com/restic/restic/releases/download/v${RESTIC_VERSION}/${base}.bz2"
  local sums="https://github.com/restic/restic/releases/download/v${RESTIC_VERSION}/SHA256SUMS"
  log "installing restic ${RESTIC_VERSION}"
  download_and_verify "$url" "$sums" "${base}.bz2"
  bzip2 -d "$TMP_DIR/${base}.bz2"
  install -m 0755 "$TMP_DIR/${base}" "$BIN_DIR/restic"
}

install_rclone() {
  if command -v rclone >/dev/null 2>&1 && rclone version 2>/dev/null | grep -q "$RCLONE_VERSION"; then
    log "rclone $RCLONE_VERSION already installed"
    return
  fi
  local base="rclone-v${RCLONE_VERSION}-linux-${RCLONE_ARCH}"
  local url="https://downloads.rclone.org/v${RCLONE_VERSION}/${base}.zip"
  local sums="https://downloads.rclone.org/v${RCLONE_VERSION}/SHA256SUMS"
  log "installing rclone ${RCLONE_VERSION}"
  require_cmd unzip
  download_and_verify "$url" "$sums" "${base}.zip"
  unzip -q "$TMP_DIR/${base}.zip" -d "$TMP_DIR"
  install -m 0755 "$TMP_DIR/${base}/rclone" "$BIN_DIR/rclone"
}

install_abm() {
  # Prefer a published GitHub release asset; fall back to building from
  # source only when run from inside a checked-out copy of this repository
  # (useful for developers) -- never installed silently on an end user's
  # machine as a fallback for a missing release.
  if [ -f "./cmd/abm/main.go" ] && command -v go >/dev/null 2>&1; then
    log "building abm from local source"
    go build -o "$BIN_DIR/abm" ./cmd/abm
    return
  fi

  local tag
  tag="$(resolve_abm_tag)" || tag=""
  [ -n "$tag" ] || fail "No Auto-Backup-Manager release is available for this channel."

  if command -v abm >/dev/null 2>&1 && abm --version 2>/dev/null | grep -q "$tag"; then
    log "Auto-Backup-Manager $tag already installed"
    return
  fi

  local asset="abm_${tag}_linux_${RESTIC_ARCH}.tar.gz"
  local asset_url="https://github.com/${REPO}/releases/download/${tag}/${asset}"
  local sums_url="https://github.com/${REPO}/releases/download/${tag}/checksums.txt"
  log "installing Auto-Backup-Manager ${tag}"
  download_and_verify "$asset_url" "$sums_url" "$asset"
  tar -xzf "$TMP_DIR/$asset" -C "$TMP_DIR"
  install -m 0755 "$TMP_DIR/abm" "$BIN_DIR/abm"
}

install_gui_service() {
  log "installing local management GUI service"
  install -m 0644 /dev/stdin /etc/systemd/system/auto-backup-manager-gui.service <<'EOF'
[Unit]
Description=Auto-Backup-Manager local management GUI
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/abm gui --no-open
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now auto-backup-manager-gui.service
}

main() {
  [ "$(id -u)" -eq 0 ] || fail "this installer must be run as root (sudo bash install.sh)"

  local arch
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) RESTIC_ARCH="amd64"; RCLONE_ARCH="amd64" ;;
    aarch64|arm64) RESTIC_ARCH="arm64"; RCLONE_ARCH="arm64" ;;
    *) fail "unsupported architecture: $arch" ;;
  esac

  TMP_DIR="$(mktemp -d)"
  trap 'rm -rf "$TMP_DIR"' EXIT

  require_cmd curl
  require_cmd tar
  require_cmd sha256sum
  require_cmd systemctl

  log "creating directories"
  install -d -m 0750 "$CONFIG_DIR" "$CONFIG_DIR/secrets" "$STATE_DIR" "$STATE_DIR/locks" "$STATE_DIR/dumps" "$LOG_DIR"

  install_restic
  install_rclone
  install_abm

	install_gui_service

  log "install complete."
  log ""
  log "Auto-Backup-Manager is running at http://127.0.0.1:8765"
  log "On a VPS, keep this port private and open it through your existing SSH connection:"
  log "  ssh -L 8765:127.0.0.1:8765 <user>@<server>"
  log "Then open http://127.0.0.1:8765. All setup continues in the browser."
  log ""
  log "Uninstalling later (sudo abm uninstall) removes only the schedule; it"
  log "never deletes config, secrets, or backup repositories."
}

# Guarded so install_test.sh can source this file (to unit-test
# resolve_abm_tag/parse_json_field/install_abm's error path in isolation,
# with no network access and no root required) without running the real
# install -- every top-level side effect (root check, arch detection, temp
# dir creation) lives inside main(), never at source time.
if [ "${ABM_INSTALL_TESTING:-0}" != "1" ]; then
  main "$@"
fi

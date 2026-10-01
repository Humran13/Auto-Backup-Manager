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
ABM_VERSION="${ABM_VERSION:-latest}"

REPO="Humran13/Auto-Backup-Manager"
BIN_DIR="/usr/local/bin"
CONFIG_DIR="/etc/auto-backup-manager"
STATE_DIR="/var/lib/auto-backup-manager"
LOG_DIR="/var/log/auto-backup-manager"

log()  { echo "[abm-install] $*"; }
fail() { echo "[abm-install] ERROR: $*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || fail "this installer must be run as root (sudo bash install.sh)"

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) RESTIC_ARCH="amd64"; RCLONE_ARCH="amd64" ;;
  aarch64|arm64) RESTIC_ARCH="arm64"; RCLONE_ARCH="arm64" ;;
  *) fail "unsupported architecture: $ARCH" ;;
esac

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

require_cmd() { command -v "$1" >/dev/null 2>&1 || fail "required command '$1' not found"; }
require_cmd curl
require_cmd tar
require_cmd sha256sum
require_cmd systemctl

# download_and_verify URL SHA256SUMS_URL FILENAME
# Downloads FILENAME and its published checksum file, and refuses to
# continue if the checksum doesn't match -- an installer that fetches
# arbitrary binaries over HTTPS without verifying them is exactly the supply
# chain risk this project's own threat model calls out.
download_and_verify() {
  local url="$1" sums_url="$2" filename="$3"
  log "downloading $filename"
  curl -fsSL -o "$TMP_DIR/$filename" "$url"
  curl -fsSL -o "$TMP_DIR/SHA256SUMS" "$sums_url"
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
  # source when run from inside a checked-out copy of this repository
  # (useful before the first tagged release exists, and for developers).
  if [ -f "./cmd/abm/main.go" ] && command -v go >/dev/null 2>&1; then
    log "building abm from local source"
    go build -o "$BIN_DIR/abm" ./cmd/abm
    return
  fi

  local api_url tag asset_url sums_url
  if [ "$ABM_VERSION" = "latest" ]; then
    api_url="https://api.github.com/repos/${REPO}/releases/latest"
  else
    api_url="https://api.github.com/repos/${REPO}/releases/tags/${ABM_VERSION}"
  fi
  tag="$(curl -fsSL "$api_url" | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
  [ -n "$tag" ] || fail "no published Auto-Backup-Manager release found; clone the repo and re-run this script from its root to build from source instead"

  asset_url="https://github.com/${REPO}/releases/download/${tag}/abm_${tag}_linux_${RESTIC_ARCH}.tar.gz"
  sums_url="https://github.com/${REPO}/releases/download/${tag}/checksums.txt"
  log "installing Auto-Backup-Manager ${tag}"
  download_and_verify "$asset_url" "$sums_url" "abm_${tag}_linux_${RESTIC_ARCH}.tar.gz"
  tar -xzf "$TMP_DIR/abm_${tag}_linux_${RESTIC_ARCH}.tar.gz" -C "$TMP_DIR"
  install -m 0755 "$TMP_DIR/abm" "$BIN_DIR/abm"
}

log "creating directories"
install -d -m 0750 "$CONFIG_DIR" "$CONFIG_DIR/secrets" "$STATE_DIR" "$STATE_DIR/locks" "$STATE_DIR/dumps" "$LOG_DIR"

install_restic
install_rclone
install_abm

log "install complete."
log ""
log "Next steps:"
log "  sudo abm setup"
log "  sudo abm storage add --type <s3|sftp|local|...> ..."
log "  sudo abm job add --name my-job --source /var/www --destination <name>"
log "  sudo abm backup now my-job"
log "  sudo abm schedule set"
log ""
log "Uninstalling later (sudo abm uninstall) removes only the schedule; it"
log "never deletes config, secrets, or backup repositories."

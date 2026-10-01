#!/usr/bin/env bash
# Regression tests for install.sh's release-version resolution.
#
# These exist specifically to prevent a repeat of the real failure found
# during manual Windows testing: a public install command silently depending
# on GitHub's /releases/latest API, which 404s for a repo with no stable
# release and never returns a prerelease even once one exists. Run with:
#   bash test/install/install_test.sh
# No network access and no root are required: ABM_INSTALL_TESTING=1 stops
# install.sh from running its real `main`, so sourcing it just defines
# functions to test directly.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

failures=0
pass() { echo "ok   - $1"; }
fail() { echo "FAIL - $1"; failures=$((failures + 1)); }

# to_file_url converts a local path to a file:// URL curl can open. Plain
# Linux curl (every real CI/production target) accepts a POSIX path as-is;
# a Windows-native curl (as shipped with Git for Windows, used when running
# this test locally on a Windows dev machine) needs a Windows-style path
# instead. This only affects how the test fixtures are addressed locally --
# install.sh's own resolve_abm_tag logic being tested is identical either way.
to_file_url() {
  if command -v cygpath >/dev/null 2>&1; then
    echo "file:///$(cygpath -m "$1")"
  else
    echo "file://$1"
  fi
}

run_test() {
  local name="$1"
  shift
  if "$@"; then pass "$name"; else fail "$name"; fi
}

# --- fixtures ---------------------------------------------------------
FIXTURE_DIR="$(mktemp -d)"
trap 'rm -rf "$FIXTURE_DIR"' EXIT

cat > "$FIXTURE_DIR/release.json" <<'EOF'
{
  "channel": "rc",
  "version": "v0.9.0-rc.1",
  "prerelease": true,
  "note": "test fixture"
}
EOF

cat > "$FIXTURE_DIR/malformed.json" <<'EOF'
{ this is not valid json
EOF

cat > "$FIXTURE_DIR/no-version-field.json" <<'EOF'
{"channel": "rc"}
EOF

# --- test: explicit ABM_VERSION wins, no network needed ---------------
test_explicit_version_bypasses_release_json() {
  local tag
  tag="$(ABM_VERSION="v9.9.9" ABM_INSTALL_TESTING=1 RELEASE_JSON_URL="file:///does/not/exist" \
    bash -c 'source "'"$REPO_ROOT"'/install.sh"; resolve_abm_tag')"
  [ "$tag" = "v9.9.9" ]
}

# --- test: RELEASE.json resolution (the normal pre-1.0 path) ----------
test_release_json_resolution() {
  local tag
  tag="$(ABM_INSTALL_TESTING=1 RELEASE_JSON_URL="$(to_file_url "$FIXTURE_DIR/release.json")" \
    bash -c 'source "'"$REPO_ROOT"'/install.sh"; resolve_abm_tag')"
  [ "$tag" = "v0.9.0-rc.1" ]
}

# --- test: never calls /releases/latest ---------------------------------
# Regression guard for the exact bug: grep the script's executable lines
# (never comments, which legitimately explain why that endpoint is avoided)
# for the ephemeral-latest API endpoint actually being invoked. If this
# starts failing, someone reintroduced the dependency that broke the real
# public install.
test_never_calls_releases_latest_api() {
  ! grep -v '^[[:space:]]*#' "$REPO_ROOT/install.sh" | grep -q "releases/latest"
}

# --- test: malformed/missing RELEASE.json resolves to empty, not a crash --
test_malformed_release_json_resolves_empty() {
  local tag
  tag="$(ABM_INSTALL_TESTING=1 RELEASE_JSON_URL="$(to_file_url "$FIXTURE_DIR/malformed.json")" \
    bash -c 'source "'"$REPO_ROOT"'/install.sh"; resolve_abm_tag' || true)"
  [ -z "$tag" ]
}

test_missing_version_field_resolves_empty() {
  local tag
  tag="$(ABM_INSTALL_TESTING=1 RELEASE_JSON_URL="$(to_file_url "$FIXTURE_DIR/no-version-field.json")" \
    bash -c 'source "'"$REPO_ROOT"'/install.sh"; resolve_abm_tag' || true)"
  [ -z "$tag" ]
}

test_unreachable_release_json_resolves_empty() {
  local tag
  tag="$(ABM_INSTALL_TESTING=1 RELEASE_JSON_URL="file:///no/such/path.json" \
    bash -c 'source "'"$REPO_ROOT"'/install.sh"; resolve_abm_tag' || true)"
  [ -z "$tag" ]
}

# --- test: install_abm fails with the controlled message, not a raw API dump
test_install_abm_fails_cleanly_when_unresolvable() {
  local out
  out="$(ABM_INSTALL_TESTING=1 RELEASE_JSON_URL="file:///no/such/path.json" \
    bash -c 'source "'"$REPO_ROOT"'/install.sh"; install_abm' 2>&1)" && return 1
  echo "$out" | grep -q "No Auto-Backup-Manager release is available for this channel."
}

# --- test: the real repo RELEASE.json is well-formed and parses -------
test_real_release_json_is_valid() {
  [ -f "$REPO_ROOT/RELEASE.json" ] || return 1
  local tag
  tag="$(ABM_INSTALL_TESTING=1 RELEASE_JSON_URL="$(to_file_url "$REPO_ROOT/RELEASE.json")" \
    bash -c 'source "'"$REPO_ROOT"'/install.sh"; resolve_abm_tag')"
  [[ "$tag" == v* ]]
}

run_test "explicit ABM_VERSION bypasses RELEASE.json (no network)" test_explicit_version_bypasses_release_json
run_test "RELEASE.json resolution returns its version field" test_release_json_resolution
run_test "install.sh never depends on GitHub's /releases/latest" test_never_calls_releases_latest_api
run_test "malformed RELEASE.json resolves to empty, not a crash" test_malformed_release_json_resolves_empty
run_test "RELEASE.json missing the version field resolves to empty" test_missing_version_field_resolves_empty
run_test "unreachable RELEASE.json resolves to empty" test_unreachable_release_json_resolves_empty
run_test "install_abm fails with the controlled message, never a raw API dump" test_install_abm_fails_cleanly_when_unresolvable
run_test "the real repo RELEASE.json is valid and resolves to a tag" test_real_release_json_is_valid

echo ""
if [ "$failures" -gt 0 ]; then
  echo "$failures test(s) FAILED"
  exit 1
fi
echo "all install.sh regression tests passed"

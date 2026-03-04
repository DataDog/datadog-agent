#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="07-checks-base"
SENTINEL="$BUILD_DIR/.done/$STAGE_NAME"
LOG="$BUILD_DIR/logs/$STAGE_NAME.log"

# Redirect all output to log file (follow with: tail -f "$LOG")
mkdir -p "$BUILD_DIR/logs"
exec > "$LOG" 2>&1

log "=== Stage: $STAGE_NAME ==="

# --- Idempotency check ---
if [ -f "$SENTINEL" ]; then
    log "Already complete (sentinel: $SENTINEL) — skipping."
    exit 0
fi

# --- Input validation ---
: "${STAGING:?STAGING must be set}"
: "${EMBEDDED_DESTDIR:?EMBEDDED_DESTDIR must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"
: "${INTEGRATIONS_CORE:?INTEGRATIONS_CORE must be set}"

PIP=$EMBEDDED_DESTDIR/bin/pip3.13

# --- Pre-flight checks ---
if [ ! -x "$PIP" ]; then
    log "ERROR: $PIP not found — did Stage 02 (02-python) complete successfully?"
    exit 1
fi

if [ ! -f "$INTEGRATIONS_CORE/datadog_checks_base/pyproject.toml" ]; then
    log "ERROR: $INTEGRATIONS_CORE/datadog_checks_base/pyproject.toml not found"
    log "       Did Stage 00 (00-checkout) clone integrations-core at the correct commit?"
    exit 1
fi

# --- Cleanup on failure ---
# pip installs are not easy to roll back; the sentinel not being written is
# sufficient to trigger a re-run.
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed."
        log "       Re-run after fixing the error by deleting the sentinel:"
        log "       rm $SENTINEL"
        log "       Common causes:"
        log "         - Native deps (pydantic-core, cryptography) not installed: ensure Stage 05 and Stage 06 completed"
        log "         - Network access required for transitive pure-Python deps from PyPI"
        log "         - integrations-core not checked out: ensure INTEGRATIONS_CORE=$INTEGRATIONS_CORE is correct"
    fi
}
trap cleanup EXIT

# ─── Step 1: Install datadog-checks-base ──────────────────────────────────────
#
# Install datadog-checks-base from the pinned integrations-core checkout with
# full dependency resolution.  pip resolves all transitive pure-Python deps
# (pyyaml, requests, prometheus_client, etc.) from PyPI.  Native deps
# (pydantic-core, cryptography) were already installed from Stages 05-06 at
# satisfying versions; pip detects they satisfy the requirements and does not
# attempt to download or rebuild them.

log "Installing datadog-checks-base from $INTEGRATIONS_CORE/datadog_checks_base"
$PIP install "$INTEGRATIONS_CORE/datadog_checks_base"
log "datadog-checks-base installed successfully"

# ─── Step 2: Freeze installed state to constraints file ───────────────────────
#
# Freeze the complete installed state into a constraints file.  Stage 08 passes
# this to every check install so pip pins all transitive deps to the exact same
# versions rather than resolving to whatever is latest on PyPI at build time.
# Any missing dep that has no AIX-compatible wheel will fail loudly here rather
# than silently at runtime.

log "Freezing installed packages to $STAGING/constraints.txt"
$PIP freeze > "$STAGING/constraints.txt"
log "Constraints written to $STAGING/constraints.txt ($(wc -l < "$STAGING/constraints.txt") packages)"

# --- Mark complete ---
mkdir -p "$(dirname "$SENTINEL")"
touch "$SENTINEL"
log "=== $STAGE_NAME complete ==="

#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="08-checks-base"
LOG="$BUILD_DIR/logs/$STAGE_NAME.log"

# Redirect all output to log file (follow with: tail -f "$LOG")
mkdir -p "$BUILD_DIR/logs"
exec > "$LOG" 2>&1

log "=== Stage: $STAGE_NAME ==="

# No stage-level sentinel: the [deps] extra packages below are version-pinned
# (pip no-ops if satisfied), and re-installing datadog-checks-base itself from
# its local source directory is cheap (pure Python, no compilation).

# --- Input validation ---
: "${STAGING:?STAGING must be set}"
: "${EMBEDDED_DESTDIR:?EMBEDDED_DESTDIR must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"
: "${INTEGRATIONS_CORE:?INTEGRATIONS_CORE must be set}"

PIP=$EMBEDDED_DESTDIR/bin/pip${PYTHON_MAJ_MIN}

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
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed. Re-run this stage to retry."
        log "       Common causes:"
        log "         - Native deps (pydantic-core, cryptography) not installed: ensure Stages 06 and 07 completed"
        log "         - Network access required for transitive pure-Python deps from PyPI"
        log "         - integrations-core not checked out: ensure INTEGRATIONS_CORE=$INTEGRATIONS_CORE is correct"
    fi
}
trap cleanup EXIT

# ─── Step 1: Install datadog-checks-base ──────────────────────────────────────
#
# Install datadog-checks-base from the pinned integrations-core checkout with
# full dependency resolution. pip resolves all transitive pure-Python deps
# (pyyaml, requests, prometheus_client, etc.) from PyPI. Native deps
# (pydantic-core, cryptography) were already installed from Stages 06-07 at
# satisfying versions; pip detects they satisfy the requirements and does not
# attempt to download or rebuild them.

log "Installing datadog-checks-base from $INTEGRATIONS_CORE/datadog_checks_base"
$PIP install "$INTEGRATIONS_CORE/datadog_checks_base"
log "datadog-checks-base installed successfully"

# ─── Step 1b: Install datadog-checks-base [deps] extra packages ───────────────
#
# Read the [deps] extra directly from pyproject.toml and install everything
# except ddtrace. Versions are taken from pyproject.toml so they stay in sync
# with integrations-core automatically.
#
# ddtrace is skipped: it requires the cmake Python package as a build
# dependency, which itself cannot be built from source on AIX. ddtrace is a
# Datadog APM tracing library not needed for core infrastructure checks.
#
# jellyfish is a Rust extension that builds using the IBM Rust SDK.
# The Rust SDK must be on PATH.

log "Reading [deps] extra from $INTEGRATIONS_CORE/datadog_checks_base/pyproject.toml"
DEPS_FILE="$BUILD_DIR/.deps-extra.txt"
python3.12 << PYEOF > "$DEPS_FILE"
import tomllib, sys
with open("$INTEGRATIONS_CORE/datadog_checks_base/pyproject.toml", "rb") as f:
    t = tomllib.load(f)
deps = t["project"]["optional-dependencies"].get("deps", [])
for d in deps:
    # Skip ddtrace: requires cmake Python package which cannot be built on AIX
    if d.lower().startswith("ddtrace"):
        print(f"# skipped: {d}", file=sys.stderr)
        continue
    # Skip Windows-only deps
    if "sys_platform" in d and "win32" in d:
        continue
    print(d)
PYEOF
log "Deps to install ($(wc -l < "$DEPS_FILE") packages, ddtrace skipped):"
cat "$DEPS_FILE"

log "Installing datadog-checks-base [deps] (excluding ddtrace)"
PATH="/opt/freeware/lib/RustSDK/${RUST_VERSION}/bin:$PATH" \
  xargs "$PIP" install --no-cache-dir < "$DEPS_FILE"
log "datadog-checks-base [deps] installed successfully"

# ─── Step 2: Freeze installed state to constraints file ───────────────────────
#
# Freeze the complete installed state into a constraints file. Stage 09 passes
# this to every check install so pip pins all transitive deps to the exact same
# versions rather than resolving to whatever is latest on PyPI at build time.
# Any missing dep that has no AIX-compatible wheel will fail loudly here rather
# than silently at runtime.

log "Freezing installed packages to $STAGING/constraints.txt"
$PIP freeze > "$STAGING/constraints.txt"
log "Constraints written to $STAGING/constraints.txt ($(wc -l < "$STAGING/constraints.txt") packages)"

log "=== $STAGE_NAME complete ==="

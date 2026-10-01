#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="09-integrations"
LOG="$BUILD_DIR/logs/$STAGE_NAME.log"

# Redirect all output to log file (follow with: tail -f "$LOG")
mkdir -p "$BUILD_DIR/logs"
exec > "$LOG" 2>&1

log "=== Stage: $STAGE_NAME ==="

# --- Input validation ---
: "${STAGING:?STAGING must be set}"
: "${EMBEDDED_DESTDIR:?EMBEDDED_DESTDIR must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"
: "${INTEGRATIONS_CORE:?INTEGRATIONS_CORE must be set}"
: "${WHEEL_CACHE:?WHEEL_CACHE must be set}"

PIP=$EMBEDDED_DESTDIR/bin/pip${PYTHON_MAJ_MIN}

# --- Pre-flight checks ---
if [ ! -x "$PIP" ]; then
    log "ERROR: $PIP not found — did Stage 02 (02-python) complete successfully?"
    exit 1
fi

if [ ! -f "$STAGING/constraints.txt" ]; then
    log "ERROR: $STAGING/constraints.txt not found — Stage 08 (08-checks-base) must complete first"
    exit 1
fi

# --- Cleanup on failure ---
# pip installs are not easy to roll back; the sentinel not being written is
# sufficient to trigger a re-run.
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed."
        log "       Common causes:"
        log "         - Stage 08 constraints.txt missing: ensure Stage 08 completed"
        log "         - integrations-core check not found: verify INTEGRATIONS_CORE=$INTEGRATIONS_CORE"
        log "         - Network access required for any dep not yet in site-packages"
    fi
}
trap cleanup EXIT

# ─── Step 1: Supplement Go check configs from integrations-core ──────────────
#
# For every Go check whose conf.d directory was populated by inv agent.build,
# also copy conf.yaml.default and conf.yaml.example from integrations-core if
# they exist there. This provides the full documented configuration (e.g. snmp
# has a rich conf.yaml.example in integrations-core but only an auto_conf.yaml
# in the agent repo). Stage 11 copies agent-repo configs afterward, so
# agent-repo files take precedence when both repos provide the same filename.

AGENT_DIST_CONFD="$AGENT_SRC/bin/agent/dist/conf.d"
if [ -d "$AGENT_DIST_CONFD" ]; then
    for check_dir in "$AGENT_DIST_CONFD"/*.d; do
        [ -d "$check_dir" ] || continue
        check=$(basename "$check_dir" .d)
        CHECK_DATA="$INTEGRATIONS_CORE/$check/datadog_checks/$check/data"
        if [ -d "$CHECK_DATA" ]; then
            mkdir -p "$STAGING/etc/datadog-agent/conf.d/${check}.d"
            for conf_file in conf.yaml.example conf.yaml.default; do
                if [ -f "$CHECK_DATA/$conf_file" ]; then
                    cp "$CHECK_DATA/$conf_file" "$STAGING/etc/datadog-agent/conf.d/${check}.d/"
                    log "Copied integrations-core $conf_file for Go check: $check"
                fi
            done
        fi
    done
fi

# ─── Step 2: Install Python checks from integrations-core ─────────────────────
#
# Discover every check in the pinned integrations-core checkout whose
# manifest.json declares "Supported OS::AIX" in tile.classifier_tags, then
# install each one. This mirrors how deps/agent_integrations/source_packages.bzl
# selects checks for the other platforms (Supported OS::Linux/macOS/Windows),
# so integrations-core is the single source of truth for AIX support instead
# of a hardcoded list here.
#
# Dependency versions come from agent_requirements.in — the same pins the
# other platforms compile into their lockfiles. The subset is computed by
# lib/aix-deps-subset.py, which also skips native deps Stage 06 did not build
# (e.g. pyodbc without unixODBC headers) so the install doesn't fail on a
# source build that cannot succeed.
#
# --constraint pins all transitive deps to the exact versions frozen by Stage 08.
# --find-links allows pip to locate native AIX wheels (pydantic-core, cryptography)
# from the local cache if needed rather than hitting PyPI.

PYTHON_CHECKS=$(python3.12 -c "
import json
import os

checks = []
for name in sorted(os.listdir('$INTEGRATIONS_CORE')):
    check_dir = os.path.join('$INTEGRATIONS_CORE', name)
    if not os.path.isfile(os.path.join(check_dir, 'pyproject.toml')):
        continue
    manifest_path = os.path.join(check_dir, 'manifest.json')
    if not os.path.isfile(manifest_path):
        continue
    with open(manifest_path) as f:
        manifest = json.load(f)
    tags = manifest.get('tile', {}).get('classifier_tags', [])
    if 'Supported OS::AIX' in tags:
        checks.append(name)
print(' '.join(checks))
")

if [ -z "$PYTHON_CHECKS" ]; then
    log "ERROR: no integrations-core checks found with the Supported OS::AIX manifest tag"
    exit 1
fi

log "Discovered Python checks tagged Supported OS::AIX: $PYTHON_CHECKS"

# Compute the AIX dependency subset (see lib/aix-deps-subset.py for details).
AIX_DEPS="$BUILD_DIR/.09-aix-deps.tmp"
AGENT_REQ="$INTEGRATIONS_CORE/agent_requirements.in"
if [ ! -f "$AGENT_REQ" ]; then
    log "ERROR: $AGENT_REQ not found — is the integrations-core checkout complete?"
    exit 1
fi
python3.12 "$SCRIPT_DIR/../lib/aix-deps-subset.py" \
    "$INTEGRATIONS_CORE" "$AGENT_REQ" "$STAGING/constraints.txt" "$AIX_DEPS"
log "AIX dependency subset written to $AIX_DEPS ($(wc -l < "$AIX_DEPS" | tr -d ' ') entries):"
sed 's/^/  /' "$AIX_DEPS" >&2

log "Installing AIX dependency subset from agent_requirements.in"
$PIP install \
    --constraint "$STAGING/constraints.txt" \
    --find-links "$WHEEL_CACHE" \
    -r "$AIX_DEPS"
rm -f "$AIX_DEPS"
log "AIX dependency subset installed"

# Check code only; its [deps] extra is already installed above, so this does
# not rebuild native extensions.
for check in $PYTHON_CHECKS; do
    CHECK_DIR="$INTEGRATIONS_CORE/$check"
    log "Installing check: $check"
    $PIP install \
        --constraint "$STAGING/constraints.txt" \
        --find-links "$WHEEL_CACHE" \
        "$CHECK_DIR"
    mkdir -p "$STAGING/etc/datadog-agent/conf.d/${check}.d"
    EXAMPLE="$CHECK_DIR/datadog_checks/$check/data/conf.yaml.example"
    if [ -f "$EXAMPLE" ]; then
        cp "$EXAMPLE" "$STAGING/etc/datadog-agent/conf.d/${check}.d/"
    else
        log "WARNING: no conf.yaml.example found for $check at $EXAMPLE"
    fi
    log "Check $check installed successfully"
done

log "All Python checks processed"

log "=== $STAGE_NAME complete ==="

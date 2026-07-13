#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="08-integrations"
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
    log "ERROR: $STAGING/constraints.txt not found — Stage 07 (07-checks-base) must complete first"
    exit 1
fi

# --- Cleanup on failure ---
# pip installs are not easy to roll back; the sentinel not being written is
# sufficient to trigger a re-run.
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed."
        log "       Common causes:"
        log "         - Stage 07 constraints.txt missing: ensure Stage 07 completed"
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
# in the agent repo). Stage 10 copies agent-repo configs afterward, so
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
# Install each check from the pinned integrations-core checkout.
# --constraint pins all transitive deps to the exact versions frozen by Stage 07,
# matching the Linux omnibus approach and failing loudly if a dep is unavailable.
# --find-links allows pip to locate native AIX wheels (pydantic-core, cryptography)
# from the local cache if needed rather than hitting PyPI.
#
# IBM checks (ibm_mq, ibm_ace, ibm_db2, ibm_i) are installed regardless of
# whether the corresponding C extension (pymqi, ibm_db, pyodbc) was built in
# Stage 05. The check code installs successfully; it will surface a clear
# ImportError at runtime if the missing native extension is not present on the
# target system.

PYTHON_CHECKS="lparstats openmetrics process ibm_mq ibm_ace ibm_db2 ibm_i ibm_was ibm_spectrum_lsf"

log "Installing Python checks: $PYTHON_CHECKS"

for check in $PYTHON_CHECKS; do
    CHECK_DIR="$INTEGRATIONS_CORE/$check"
    if [ -f "$CHECK_DIR/pyproject.toml" ]; then
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
    else
        log "WARNING: $check not found in integrations-core at $CHECK_DIR — skipping"
    fi
done

log "All Python checks processed"

log "=== $STAGE_NAME complete ==="

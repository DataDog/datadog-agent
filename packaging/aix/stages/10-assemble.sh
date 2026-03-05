#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="10-assemble"
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
: "${AGENT_VERSION:?AGENT_VERSION must be set}"
: "${AGENT_VRMF:?AGENT_VRMF must be set}"
: "${STAGING:?STAGING must be set}"
: "${EMBEDDED_DESTDIR:?EMBEDDED_DESTDIR must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"

# --- Cleanup on failure ---
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed. Removing partial outputs."
        # Remove only the files written by this stage; leave earlier stage outputs intact.
        rm -f "$STAGING/etc/datadog-agent/datadog.yaml.example"
        rm -rf "$EMBEDDED_DESTDIR/share/installp"
    fi
}
trap cleanup EXIT

# ─── Step 1: Pre-flight — verify agent binary exists ──────────────────────────
#
# The agent binary is the primary deliverable.  If it is absent the staging
# tree is incomplete and packaging will produce a broken BFF.

AGENT_BIN="$STAGING/opt/datadog-agent/bin/agent"
if [ ! -f "$AGENT_BIN" ]; then
    log "ERROR: agent binary not found at $AGENT_BIN"
    log "       Did Stage 04 (04-agent) complete successfully?"
    exit 1
fi
log "Pre-flight: agent binary found at $AGENT_BIN"

# ─── Step 2: Copy main config example ─────────────────────────────────────────
#
# Install the upstream datadog.yaml as a .example file so the operator can copy
# and edit it.  The agent will not start without a real datadog.yaml; the .example
# suffix makes it clear that manual configuration is required.

log "Copying main config example"
mkdir -p "$STAGING/etc/datadog-agent"
cp /opt/datadog-agent/cmd/agent/dist/datadog.yaml \
    "$STAGING/etc/datadog-agent/datadog.yaml.example"
log "Config example written to $STAGING/etc/datadog-agent/datadog.yaml.example"

# ─── Step 3: Create required empty directories ────────────────────────────────
#
# mkinstallp will include these directories in the package so that installp
# creates them on the target system.  The agent and its postinst script expect
# them to exist at runtime.

log "Creating required runtime directories"
mkdir -p "$STAGING/var/log/datadog"
mkdir -p "$STAGING/var/run/datadog"
mkdir -p "$STAGING/opt/datadog-agent/run"
mkdir -p "$STAGING/opt/datadog-agent/checks.d"
mkdir -p "$STAGING/opt/datadog-agent/conf.d"
log "Runtime directories created"

# ─── Step 4: Copy package lifecycle scripts into the staging tree ──────────────
#
# mkinstallp requires lifecycle scripts to be present at their final installed
# path inside the staging tree.  The gen.template.in references them at
# /opt/datadog-agent/embedded/share/installp/<name>.  They must be executable.
#
# Fail clearly if any script is missing rather than producing a BFF that silently
# has no pre/post install hooks.

log "Copying package lifecycle scripts"
SCRIPTS_DIR="$EMBEDDED_DESTDIR/share/installp"
mkdir -p "$SCRIPTS_DIR"
# mkinstallp checks lifecycle scripts at their absolute installed path, not
# relative to the staging tree.  Copy them to BOTH the staging tree (so they
# are included in the BFF) AND the installed path (so mkinstallp can find
# them when building the BFF on the same host that will run the agent).
SCRIPTS_INSTALLED="$EMBEDDED/share/installp"
mkdir -p "$SCRIPTS_INSTALLED"
PKGSCRIPTS_SRC="$(dirname "$0")/../package-scripts"

for script in preinst postinst config unconfig prerm postrm; do
    SRC="$PKGSCRIPTS_SRC/$script"
    if [ ! -f "$SRC" ]; then
        log "ERROR: package script not found: $SRC"
        log "       All six lifecycle scripts (preinst postinst config unconfig prerm postrm)"
        log "       must exist under packaging/aix/package-scripts/ before running this stage."
        exit 1
    fi
    cp "$SRC" "$SCRIPTS_DIR/$script"
    chmod 755 "$SCRIPTS_DIR/$script"
    cp "$SRC" "$SCRIPTS_INSTALLED/$script"
    chmod 755 "$SCRIPTS_INSTALLED/$script"
    log "  Staging:   $SCRIPTS_DIR/$script"
    log "  Installed: $SCRIPTS_INSTALLED/$script"
done
log "All package lifecycle scripts installed"

# ─── Step 5: Set correct ownership ────────────────────────────────────────────
#
# mkinstallp records the owning uid:gid of every file and directory in the
# generated BFF.  If files are owned by a build user rather than root, installp
# will install them with that non-root ownership on the target system, which
# causes permission errors at runtime.  chown -h (portable spelling: -Rh) also
# fixes symbolic link ownership without following the link target.

log "Setting root ownership on staging tree"
chown -Rh 0:0 "$STAGING/opt" "$STAGING/etc" "$STAGING/var"
log "Ownership set"

# ─── Step 6: Print staging tree size summary ──────────────────────────────────
#
# Provides a quick sanity check that key components were built and are present.
# du -s prints single-directory totals in 512-byte blocks (AIX default).
# Use || true so a missing optional directory (e.g. rtloader if not yet built)
# does not abort the summary.

log "Staging tree size summary:"
du -s \
    "$STAGING/opt/datadog-agent/bin" \
    "$STAGING/opt/datadog-agent/embedded/lib" \
    "$STAGING/opt/datadog-agent/rtloader" 2>/dev/null || true

# --- Mark complete ---
mkdir -p "$(dirname "$SENTINEL")"
touch "$SENTINEL"
log "=== $STAGE_NAME complete ==="

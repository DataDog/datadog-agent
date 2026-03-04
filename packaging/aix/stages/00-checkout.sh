#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="00-checkout"
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
: "${BUILD_DIR:?BUILD_DIR must be set}"
: "${STAGING:?STAGING must be set}"
: "${INTEGRATIONS_CORE:?INTEGRATIONS_CORE must be set}"

# --- Cleanup on failure ---
PARTIAL_INTEGRATIONS_CORE=
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed."
        if [ -n "$PARTIAL_INTEGRATIONS_CORE" ] && [ -d "$PARTIAL_INTEGRATIONS_CORE" ]; then
            log "Removing partial integrations-core checkout: $PARTIAL_INTEGRATIONS_CORE"
            rm -rf "$PARTIAL_INTEGRATIONS_CORE"
        fi
    fi
}
trap cleanup EXIT

# ─── Step 1: Validate agent source tree ───────────────────────────────────────
#
# The agent source is NOT cloned here — it must already be present at
# /opt/datadog-agent, transferred from the build machine by the caller.
# (The local clone contains AIX-specific changes not yet merged to main.)
#
# Expected transfer method:
#   tar czf /tmp/dd-agent-src.tar.gz --exclude='.git' --exclude='bin' ... .
#   scp /tmp/dd-agent-src.tar.gz aix-host:/tmp/
#   ssh aix-host 'mkdir -p /opt/datadog-agent && gunzip -c /tmp/dd-agent-src.tar.gz | tar xf - -C /opt/datadog-agent'

AGENT_SRC=/opt/datadog-agent

if [ ! -f "$AGENT_SRC/go.mod" ]; then
    log "ERROR: agent source not found at $AGENT_SRC/go.mod"
    log "       Transfer the source tree to the AIX host first:"
    log "         On build machine: tar czf /tmp/dd-agent-src.tar.gz --exclude='.git' --exclude='bin' ."
    log "         scp /tmp/dd-agent-src.tar.gz aix-host:/tmp/"
    log "         On AIX host:      mkdir -p /opt/datadog-agent"
    log "                           gunzip -c /tmp/dd-agent-src.tar.gz | tar xf - -C /opt/datadog-agent"
    exit 1
fi

log "Agent source found at $AGENT_SRC"
log "  go.mod: $(head -1 $AGENT_SRC/go.mod)"

# ─── Step 2: Read INTEGRATIONS_CORE_VERSION from release.json ─────────────────

RELEASE_JSON="$AGENT_SRC/release.json"
if [ ! -f "$RELEASE_JSON" ]; then
    log "ERROR: $RELEASE_JSON not found — is the source tree complete?"
    exit 1
fi

log "Reading INTEGRATIONS_CORE_VERSION from $RELEASE_JSON"
INTEGRATIONS_CORE_VERSION=$(python3.12 -c \
    "import json; print(json.load(open('$RELEASE_JSON'))['dependencies']['INTEGRATIONS_CORE_VERSION'])")

if [ -z "$INTEGRATIONS_CORE_VERSION" ]; then
    log "ERROR: Could not read INTEGRATIONS_CORE_VERSION from $RELEASE_JSON"
    exit 1
fi

log "INTEGRATIONS_CORE_VERSION = $INTEGRATIONS_CORE_VERSION"

# ─── Step 3: Clone or fetch integrations-core ─────────────────────────────────

log "Checking out DataDog/integrations-core at $INTEGRATIONS_CORE_VERSION into $INTEGRATIONS_CORE"

mkdir -p "$(dirname "$INTEGRATIONS_CORE")"

if [ -d "$INTEGRATIONS_CORE/.git" ]; then
    log "integrations-core repository already exists — fetching latest refs"
    git -C "$INTEGRATIONS_CORE" fetch --quiet
else
    log "Cloning https://github.com/DataDog/integrations-core.git (shallow --depth=1)"
    PARTIAL_INTEGRATIONS_CORE="$INTEGRATIONS_CORE"
    # --depth=1 cuts clone time from ~3 min to ~30 sec.  If the pinned SHA is not
    # the current HEAD of the default branch, fetch it specifically afterwards.
    # GitHub supports fetching public commits by SHA (git protocol v2).
    git clone --depth=1 --quiet https://github.com/DataDog/integrations-core.git "$INTEGRATIONS_CORE"
    if ! git -C "$INTEGRATIONS_CORE" cat-file -e "${INTEGRATIONS_CORE_VERSION}^{commit}" 2>/dev/null; then
        log "  Pinned commit not at clone HEAD; fetching $INTEGRATIONS_CORE_VERSION"
        git -C "$INTEGRATIONS_CORE" fetch --quiet --depth=1 origin "$INTEGRATIONS_CORE_VERSION"
    fi
    PARTIAL_INTEGRATIONS_CORE=
fi

git -C "$INTEGRATIONS_CORE" checkout --quiet "$INTEGRATIONS_CORE_VERSION"
log "Checked out integrations-core at $(git -C "$INTEGRATIONS_CORE" rev-parse HEAD)"

# --- Mark complete ---
mkdir -p "$(dirname "$SENTINEL")"
touch "$SENTINEL"
log "=== $STAGE_NAME complete ==="

#!/bin/sh
set -eu

# Source shared environment (defines STAGING, SALUKI_SRC, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="05-agent-data-plane"
LOG="$BUILD_DIR/logs/$STAGE_NAME.log"

# Redirect all output to log file (follow with: tail -f "$LOG")
mkdir -p "$BUILD_DIR/logs"
exec > "$LOG" 2>&1

log "=== Stage: $STAGE_NAME ==="

# No completion sentinel: this stage always runs so the packaged ADP binary and
# license artifacts stay in sync with the pinned saluki checkout. Cargo caches
# compiled crates, so rebuilds with no changes mostly reuse prior work.

# --- Input validation ---
: "${AGENT_DATA_PLANE_VERSION:?AGENT_DATA_PLANE_VERSION must be set}"
: "${SALUKI_SRC:?SALUKI_SRC must be set}"
: "${STAGING:?STAGING must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"

ADP_AIX_BUILD_PROFILE=${ADP_AIX_BUILD_PROFILE:-aix-optimized-release}
CARGO_HOME=${CARGO_HOME:-$BUILD_DIR/cargo-home}
CARGO_TARGET_DIR=${CARGO_TARGET_DIR:-$BUILD_DIR/saluki-target}
export CARGO_HOME CARGO_TARGET_DIR

if [ "${ADP_AIX_BUILD_COMMAND+x}" != x ]; then
    ADP_AIX_BUILD_COMMAND="make build-adp-aix"
fi
ADP_AIX_BINARY_PATH=${ADP_AIX_BINARY_PATH:-}
ADP_RELEASE_TARBALL_PATH=${ADP_RELEASE_TARBALL_PATH:-}
ADP_RELEASE_TARBALL_DIR="$BUILD_DIR/agent-data-plane-release-tarball"
ADP_SPDX_LICENSES_ARCHIVE="$BUILD_DIR/agent-data-plane-spdx-licenses.tar.gz"
ADP_SPDX_LICENSES_EXTRACT_DIR="$BUILD_DIR/agent-data-plane-spdx-licenses-extract"
ADP_SPDX_LICENSES_DIR="$BUILD_DIR/agent-data-plane-spdx-licenses"
ADP_THIRD_PARTY_GENERATED_DIR="$BUILD_DIR/agent-data-plane-third-party-licenses"
ADP_BIN_DEST="$STAGING/opt/datadog-agent/embedded/bin/agent-data-plane"
ADP_LICENSES_DEST="$STAGING/opt/datadog-agent/LICENSES"

# --- Pre-flight ---
if [ ! -d "$SALUKI_SRC/.git" ]; then
    log "ERROR: saluki source not found at $SALUKI_SRC"
    log "       Did Stage 00 (00-checkout) complete successfully?"
    exit 1
fi
log "saluki source found at $SALUKI_SRC"
log "Building Agent Data Plane version $AGENT_DATA_PLANE_VERSION"

# --- Cleanup on failure ---
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed. Removing partial outputs."
        rm -f "$ADP_BIN_DEST"
        rm -f "$ADP_LICENSES_DEST/LICENSE-agent-data-plane-3rdparty.csv"
        rm -f "$ADP_SPDX_LICENSES_ARCHIVE"
        rm -rf "$ADP_LICENSES_DEST"/THIRD-PARTY-*
        rm -rf "$ADP_RELEASE_TARBALL_DIR" "$ADP_SPDX_LICENSES_EXTRACT_DIR" \
            "$ADP_SPDX_LICENSES_DIR" "$ADP_THIRD_PARTY_GENERATED_DIR"
    fi
}
trap cleanup EXIT

# ─── Step 1: Build ADP ────────────────────────────────────────────────────────

rm -rf "$ADP_RELEASE_TARBALL_DIR"

if [ -n "$ADP_AIX_BUILD_COMMAND" ]; then
    log "Building agent-data-plane via $ADP_AIX_BUILD_COMMAND"
    cd "$SALUKI_SRC"
    unset ARFLAGS
    BUILD_PROFILE="$ADP_AIX_BUILD_PROFILE" sh -c "$ADP_AIX_BUILD_COMMAND"
    if [ -z "$ADP_AIX_BINARY_PATH" ]; then
        ADP_AIX_BINARY_PATH="$CARGO_TARGET_DIR/$ADP_AIX_BUILD_PROFILE/agent-data-plane"
    fi
else
    log "ADP_AIX_BUILD_COMMAND is disabled"
    if [ -z "$ADP_AIX_BINARY_PATH" ] && [ -z "$ADP_RELEASE_TARBALL_PATH" ]; then
        log "ERROR: set ADP_AIX_BUILD_COMMAND, ADP_AIX_BINARY_PATH, or ADP_RELEASE_TARBALL_PATH"
        log "       Prebuilt artifacts must be explicit when the build hook is disabled."
        exit 1
    fi
fi

if [ -n "$ADP_RELEASE_TARBALL_PATH" ]; then
    if [ ! -f "$ADP_RELEASE_TARBALL_PATH" ]; then
        log "ERROR: ADP release tarball not found at $ADP_RELEASE_TARBALL_PATH"
        exit 1
    fi
    log "Extracting ADP release tarball: $ADP_RELEASE_TARBALL_PATH"
    mkdir -p "$ADP_RELEASE_TARBALL_DIR"
    gunzip -c "$ADP_RELEASE_TARBALL_PATH" | tar xf - -C "$ADP_RELEASE_TARBALL_DIR"
fi

ADP_TARBALL_BIN="$ADP_RELEASE_TARBALL_DIR/opt/datadog-agent/embedded/bin/agent-data-plane"
if [ -z "$ADP_AIX_BINARY_PATH" ] && [ -f "$ADP_TARBALL_BIN" ]; then
    ADP_AIX_BINARY_PATH="$ADP_TARBALL_BIN"
fi

if [ ! -f "$ADP_AIX_BINARY_PATH" ]; then
    log "ERROR: agent-data-plane binary not found at $ADP_AIX_BINARY_PATH"
    log "       Set ADP_AIX_BUILD_COMMAND, or set ADP_AIX_BINARY_PATH/ADP_RELEASE_TARBALL_PATH."
    exit 1
fi

# ─── Step 2: Install binary ───────────────────────────────────────────────────

log "Installing agent-data-plane binary"
mkdir -p "$(dirname "$ADP_BIN_DEST")"
cp "$ADP_AIX_BINARY_PATH" "$ADP_BIN_DEST"
strip -X64 "$ADP_BIN_DEST"
chmod 755 "$ADP_BIN_DEST"
log "agent-data-plane binary installed at $ADP_BIN_DEST"

# ─── Step 3: Install license artifacts ────────────────────────────────────────

ADP_LICENSE_3RDPARTY=${ADP_LICENSE_3RDPARTY:-}
if [ -z "$ADP_LICENSE_3RDPARTY" ]; then
    if [ -f "$SALUKI_SRC/LICENSE-3rdparty.csv" ]; then
        ADP_LICENSE_3RDPARTY="$SALUKI_SRC/LICENSE-3rdparty.csv"
    elif [ -f "$ADP_RELEASE_TARBALL_DIR/opt/datadog/agent-data-plane/LICENSE-3rdparty.csv" ]; then
        ADP_LICENSE_3RDPARTY="$ADP_RELEASE_TARBALL_DIR/opt/datadog/agent-data-plane/LICENSE-3rdparty.csv"
    fi
fi
if [ ! -f "$ADP_LICENSE_3RDPARTY" ]; then
    log "ERROR: ADP third-party license CSV not found"
    log "       Set ADP_LICENSE_3RDPARTY, or provide the saluki release-tarball layout."
    exit 1
fi

ADP_THIRD_PARTY_SRC=${ADP_THIRD_PARTY_SRC:-}
if [ -z "$ADP_THIRD_PARTY_SRC" ] && [ -d "$ADP_RELEASE_TARBALL_DIR/opt/datadog/agent-data-plane/LICENSES" ]; then
    ADP_THIRD_PARTY_SRC="$ADP_RELEASE_TARBALL_DIR/opt/datadog/agent-data-plane/LICENSES"
fi
if [ -z "$ADP_THIRD_PARTY_SRC" ]; then
    ADP_SPDX_LICENSES_VERSION=${ADP_SPDX_LICENSES_VERSION:-$(awk '/^export ADP_SPDX_LICENSES_VERSION[[:space:]]*:=/ {print $4; exit}' "$SALUKI_SRC/Makefile")}
    if [ -z "$ADP_SPDX_LICENSES_VERSION" ]; then
        log "ERROR: could not determine ADP_SPDX_LICENSES_VERSION from $SALUKI_SRC/Makefile"
        exit 1
    fi
    log "Generating ADP THIRD-PARTY license artifacts with SPDX license-list-data $ADP_SPDX_LICENSES_VERSION"
    rm -rf "$ADP_THIRD_PARTY_GENERATED_DIR" "$ADP_SPDX_LICENSES_EXTRACT_DIR" "$ADP_SPDX_LICENSES_DIR"
    curl -sSfL -o "$ADP_SPDX_LICENSES_ARCHIVE" \
        "https://github.com/spdx/license-list-data/archive/refs/tags/v$ADP_SPDX_LICENSES_VERSION.tar.gz"
    mkdir -p "$ADP_SPDX_LICENSES_EXTRACT_DIR"
    gunzip -c "$ADP_SPDX_LICENSES_ARCHIVE" | tar xf - -C "$ADP_SPDX_LICENSES_EXTRACT_DIR"
    mv "$ADP_SPDX_LICENSES_EXTRACT_DIR/license-list-data-$ADP_SPDX_LICENSES_VERSION" \
        "$ADP_SPDX_LICENSES_DIR"
    sh "$SALUKI_SRC/ci/tooling/collect-third-party-licenses.sh" \
        "$ADP_SPDX_LICENSES_DIR/text" \
        "$ADP_LICENSE_3RDPARTY" \
        "$ADP_THIRD_PARTY_GENERATED_DIR"
    ADP_THIRD_PARTY_SRC="$ADP_THIRD_PARTY_GENERATED_DIR"
fi
if [ ! -d "$ADP_THIRD_PARTY_SRC" ]; then
    log "ERROR: ADP THIRD-PARTY license directory not found"
    log "       Set ADP_THIRD_PARTY_SRC, or provide the saluki release-tarball layout."
    exit 1
fi

set -- "$ADP_THIRD_PARTY_SRC"/THIRD-PARTY-*
if [ ! -f "$1" ] && [ ! -d "$1" ]; then
    log "ERROR: no THIRD-PARTY-* license artifacts found in $ADP_THIRD_PARTY_SRC"
    exit 1
fi

mkdir -p "$ADP_LICENSES_DEST"
cp "$ADP_LICENSE_3RDPARTY" "$ADP_LICENSES_DEST/LICENSE-agent-data-plane-3rdparty.csv"
cp -R "$ADP_THIRD_PARTY_SRC"/THIRD-PARTY-* "$ADP_LICENSES_DEST/"
log "ADP license artifacts installed to $ADP_LICENSES_DEST"

# ─── Step 4: Verify XCOFF64 magic bytes ───────────────────────────────────────

log "Verifying agent-data-plane binary is XCOFF64"
MAGIC=$(od -A x -t x1 "$ADP_BIN_DEST" | head -1 | awk '{print $2 $3}')
if [ "$MAGIC" != "01f7" ]; then
    log "ERROR: agent-data-plane binary is not XCOFF64 (got: $MAGIC)"
    log "       Expected magic bytes: 01 f7"
    exit 1
fi
log "XCOFF64 magic verified for agent-data-plane binary (magic: $MAGIC)"

log "=== $STAGE_NAME complete ==="

#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="06-pydantic"
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
: "${WHEEL_CACHE:?WHEEL_CACHE must be set}"
: "${INTEGRATIONS_CORE:?INTEGRATIONS_CORE must be set}"

PIP=$EMBEDDED_DESTDIR/bin/pip3.13

# --- Pre-flight: confirm pip3.13 exists ---
if [ ! -x "$PIP" ]; then
    log "ERROR: $PIP not found — did Stage 02 (02-python) complete successfully?"
    exit 1
fi

# --- Pre-flight: confirm integrations-core is checked out ---
PYPROJECT="$INTEGRATIONS_CORE/datadog_checks_base/pyproject.toml"
if [ ! -f "$PYPROJECT" ]; then
    log "ERROR: $PYPROJECT not found — did Stage 00 (00-checkout) complete successfully?"
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
        log "         - Rust SDK not installed: yum install rust1.92.ppc cargo1.92.ppc rust1.92-std-static.ppc"
        log "         - Insufficient disk space (need ~7 GB free in /tmp, ~4 GB in /)"
        log "         - CC not set to GCC (xlc rejects -fPIC, -maix64 flags)"
    fi
}
trap cleanup EXIT

# ─── Step 1: Read required pydantic version from integrations-core ─────────────
#
# datadog_checks_base/pyproject.toml pins an exact pydantic version, e.g.:
#   "pydantic==2.11.7",
# pydantic itself pins exactly one pydantic-core version.  By installing the
# exact pydantic version required by the checks, we guarantee the right
# pydantic-core version is pulled in.
#
# The wheel cache is keyed by this pydantic version so that upgrading pydantic
# (via a new integrations-core commit) automatically invalidates the cache and
# triggers a fresh Rust build.

PYDANTIC_VERSION=$(grep '"pydantic==' "$PYPROJECT" | sed 's/.*pydantic==\([0-9][^"]*\)".*/\1/' | head -1)

if [ -z "$PYDANTIC_VERSION" ]; then
    log "ERROR: could not parse pydantic version from $PYPROJECT"
    log "       Expected a line like: \"pydantic==2.11.7\","
    exit 1
fi

log "Required pydantic version (from datadog_checks_base): $PYDANTIC_VERSION"

# ─── Step 2: Set Rust environment ─────────────────────────────────────────────
#
# AIX-specific Rust build flags:
#   CARGO_PROFILE_RELEASE_STRIP=none  — IBM Rust 1.92 bug: stripping .info section
#                                       from proc-macro artifacts breaks rustc
#   CARGO_PROFILE_RELEASE_LTO=off     — LLVM fat LTO uses .ipa bitcode sections
#                                       that do not exist in AIX XCOFF format;
#                                       fails after 50+ minutes of compilation
#   CC=/opt/freeware/bin/gcc          — cc-rs defaults to IBM xlc which rejects GCC
#                                       flags like -fPIC, -ffunction-sections, -maix64
#   RUSTFLAGS="-C link-arg=-bbigtoc" — pydantic-core exceeds AIX ld's 64KB TOC limit;
#                                       -bbigtoc removes this limit for AIX XCOFF.

log "Setting Rust environment for pydantic-core build"
export CC=/opt/freeware/bin/gcc
export CXX=/opt/freeware/bin/g++
export PATH=/opt/freeware/lib/RustSDK/1.92/bin:"$PATH"
export CARGO_HOME=/opt/cargo

log "  CC=$CC"
log "  CXX=$CXX"
log "  CARGO_HOME=$CARGO_HOME"
log "  Rust toolchain: $(cargo --version 2>/dev/null || echo 'cargo not found — install rust1.92.ppc')"

# ─── Step 3: Check wheel cache ────────────────────────────────────────────────
#
# The cache is keyed by pydantic version (e.g. $WHEEL_CACHE/pydantic-2.11.7/).
# This ensures that when integrations-core bumps pydantic (and thereby
# pydantic-core), the old cached wheel is not used — a new subdirectory is
# created and a fresh Rust build is triggered automatically.
#
# pydantic-core takes ~52 minutes to build from source on POWER8.  If a
# pre-built wheel is present in the versioned cache directory, install from
# it and skip the Rust build entirely.

WHEEL_CACHE_DIR="$WHEEL_CACHE/pydantic-$PYDANTIC_VERSION"
mkdir -p "$WHEEL_CACHE_DIR"

# Match only full-AIX-tag wheels (e.g. aix_7302_2419_64) not legacy aix_ppc64 renames.
# aix_*_* requires at least one underscore within the AIX portion, which aix_ppc64 lacks.
CACHED_WHEEL=$(find "$WHEEL_CACHE_DIR" -name 'pydantic_core-*-cp313-cp313-aix_*_*.whl' 2>/dev/null | head -1)

if [ -n "$CACHED_WHEEL" ]; then
    log "Found cached pydantic-core wheel: $CACHED_WHEEL"
    log "Installing pydantic==$PYDANTIC_VERSION and pydantic-core from wheel cache (skipping Rust build)"
    # --find-links lets pip use the local cached wheel for pydantic-core while
    # downloading pydantic itself (pure Python) from PyPI.
    # The wheel filename retains the original AIX platform tag so pip can match it.
    $PIP install \
        --find-links "$WHEEL_CACHE_DIR" \
        "pydantic==$PYDANTIC_VERSION"
    log "pydantic and pydantic-core installed from cache successfully"
else
    log "No cached wheel found for pydantic==$PYDANTIC_VERSION — building pydantic-core from source"
    log "WARNING: This step takes approximately 52 minutes on POWER8."
    log "         Disk space required: ~7 GB in /tmp, ~4 GB in /"
    log "         Cache directory: $WHEEL_CACHE_DIR"

    CARGO_PROFILE_RELEASE_STRIP=none \
    CARGO_PROFILE_RELEASE_LTO=off \
    RUSTFLAGS="-C link-arg=-bbigtoc" \
    ARFLAGS="" \
        $PIP install "pydantic==$PYDANTIC_VERSION" --no-binary pydantic-core

    log "pydantic-core build complete"

    # Cache the built wheel for next time.  maturin produces a system-specific
    # platform tag (aix_3_XXXXXXXX_XXXXXXXX); rename it to the portable aix_ppc64
    # tag so the same wheel can be used on any AIX 7.x POWER system.
    BUILT_WHEEL=$(find "${HOME}/.cache/pip" -name "pydantic_core-*.whl" 2>/dev/null | head -1)
    if [ -n "$BUILT_WHEEL" ]; then
        # Keep the original filename (with the full AIX platform tag from this system)
        # so that pip's --find-links can match it by tag on cache restore.
        CACHE_NAME=$(basename "$BUILT_WHEEL")
        cp "$BUILT_WHEEL" "$WHEEL_CACHE_DIR/$CACHE_NAME"
        log "Cached wheel to $WHEEL_CACHE_DIR/$CACHE_NAME"
        log "  Preserved for all future builds using pydantic==$PYDANTIC_VERSION."
    else
        log "WARNING: could not locate built pydantic-core wheel in pip cache"
        log "         Next build will rebuild from source (~52 minutes)"
    fi
fi

# ─── Step 4: Install typing_extensions ────────────────────────────────────────
#
# pydantic-core >= 2.41 requires typing_extensions >= 4.14.1.  Install it
# explicitly after pydantic-core so the constraint is satisfied even if pydantic
# itself did not pull in a new enough version.

log "Installing typing_extensions (required by pydantic-core >= 2.41)"
$PIP install "typing_extensions>=4.14.1"
log "typing_extensions installed successfully"

# --- Mark complete ---
mkdir -p "$(dirname "$SENTINEL")"
touch "$SENTINEL"
log "=== $STAGE_NAME complete ==="

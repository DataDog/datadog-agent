#!/bin/sh
# build.sh — top-level orchestrator for the AIX Datadog Agent BFF package build
#
# Usage:
#   AGENT_BUILD=1 ./build.sh
#
# Required environment variables:
#   AGENT_BUILD  — build iteration / 4th VRMF digit (e.g. 1).
#                  Must be a positive integer; must increase with each release
#                  so installp upgrade ordering works.  Cannot be auto-detected.
#
# Optional environment variables:
#   AGENT_VERSION — full version string (e.g. 7.80.0-devel.git.50.3a914cd).
#                   When not set, auto-detected by running:
#                     python3.12 -m invoke agent.version --url-safe --include-git
#                   This produces the same version string embedded in the binary.
#                   The build fails if invoke is unavailable or returns empty.
#
# VRMF (installp package version) is X.Y.Z.AGENT_BUILD — the pre/git suffix of
# AGENT_VERSION is stripped by env.sh.
# Package filename: datadog-agent-<AGENT_VERSION>-<AGENT_BUILD>.aix.ppc64.bff
#
# The agent source with full .git history must be at /opt/datadog-agent.
# All intermediate artifacts go under /opt/dd-build/.

set -eu

PATH=/opt/go/bin:/opt/freeware/bin:/usr/sbin:/usr/bin:/bin:$PATH
export PATH

if [ -z "${AGENT_BUILD:-}" ]; then
    printf 'ERROR: AGENT_BUILD must be set (e.g. AGENT_BUILD=1)\n' >&2
    printf '       This is the installp build counter and must increase with each release.\n' >&2
    exit 1
fi

if [ ! -d /opt/datadog-agent/.git ]; then
    printf 'ERROR: /opt/datadog-agent/.git not found\n' >&2
    printf '       The source tree must include full git history.\n' >&2
    exit 1
fi

if [ -z "${AGENT_VERSION:-}" ]; then
    AGENT_VERSION=$(cd /opt/datadog-agent && \
        python3.12 -m invoke agent.version --url-safe --include-git 2>&1)
    if [ -z "$AGENT_VERSION" ]; then
        printf 'ERROR: invoke agent.version returned empty output.\n' >&2
        exit 1
    fi
    printf 'INFO: AGENT_VERSION: %s\n' "$AGENT_VERSION" >&2
fi

# ── Check required tools ──────────────────────────────────────────────────────
# Fail early with a clear message if a required build tool is missing.

check_tool() {
    _tool=$1; _pkg=${2:-$1}
    if ! command -v "$_tool" >/dev/null 2>&1; then
        printf 'ERROR: required tool not found: %s\n' "$_tool" >&2
        printf '       Install with: yum install %s\n' "$_pkg" >&2
        exit 1
    fi
}

check_tool git        git
check_tool curl       curl
check_tool xz         xz
check_tool make       make
check_tool cmake      cmake
check_tool gcc        gcc
check_tool python3.12 python3.12
check_tool go         golang

# Several libraries are taken from AIX Toolbox (source builds fail on AIX).
# Check that all required -devel packages are installed.
check_aix_devel() {
    _hdr=$1; _pkg=$2
    if [ ! -f "$_hdr" ]; then
        printf 'ERROR: %s not found (required for build)\n' "$_hdr" >&2
        printf '       Install with: yum install %s\n' "$_pkg" >&2
        exit 1
    fi
}
check_aix_devel /opt/freeware/include/ffi.h          libffi-devel
check_aix_devel /opt/freeware/lib64/libffi.a          libffi-devel
check_aix_devel /opt/freeware/include/ncurses.h       ncurses-devel
check_aix_devel /opt/freeware/lib64/libncursesw.a     ncurses-devel
check_aix_devel /opt/freeware/include/readline/readline.h  readline-devel
check_aix_devel /opt/freeware/lib64/libreadline.a     readline-devel
check_aix_devel /opt/freeware/include/sqlite3.h       sqlite-devel
check_aix_devel /opt/freeware/lib64/libsqlite3.a      sqlite-devel
check_aix_devel /opt/freeware/include/gdbm.h          gdbm-devel
check_aix_devel /opt/freeware/lib/libgdbm.a           gdbm-devel
check_aix_devel /opt/freeware/include/libxslt/xslt.h  libxslt-devel
check_aix_devel /opt/freeware/lib/libxslt.a           libxslt-devel

# ── Source shared environment ─────────────────────────────────────────────────

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/lib/env.sh"

# ── Bootstrap build directories ───────────────────────────────────────────────

mkdir -p "$BUILD_DIR/logs"
mkdir -p "$BUILD_DIR/.done"
mkdir -p "$STAGING"

# ── Stage list ────────────────────────────────────────────────────────────────

STAGES="
00-checkout
01-native-libs
02-python
03-rtloader
04-agent
05-python-extensions
06-pydantic
07-checks-base
08-integrations
09-strip-bytecode
10-assemble
"

# ── Helper: run one stage script ──────────────────────────────────────────────

run_stage() {
    _stage="$1"
    _script="$SCRIPT_DIR/stages/${_stage}.sh"

    if [ ! -f "$_script" ]; then
        log "ERROR: stage script not found: $_script"
        return 1
    fi

    log "--- Starting stage: $_stage ---"
    if sh "$_script"; then
        log "--- Stage complete: $_stage ---"
    else
        log "ERROR: Stage failed: $_stage  (exit $?)"
        log "Check log: $BUILD_DIR/logs/${_stage}.log"
        return 1
    fi
}

# ── Main ──────────────────────────────────────────────────────────────────────

BUILD_START=$(date '+%Y-%m-%dT%H:%M:%S')
log "=== Datadog Agent AIX package build ==="
log "    AGENT_VERSION = $AGENT_VERSION"
log "    AGENT_BUILD   = $AGENT_BUILD"
log "    AGENT_VRMF    = $AGENT_VRMF"
log "    BUILD_DIR     = $BUILD_DIR"
log "    STAGING       = $STAGING"
log "    Started at    = $BUILD_START"

# Run all numbered stages in order
for stage in $STAGES; do
    run_stage "$stage" || exit 1
done

# Run the final packaging step
log "--- Starting stage: package ---"
if sh "$SCRIPT_DIR/package.sh"; then
    log "--- Stage complete: package ---"
else
    log "ERROR: Stage failed: package"
    log "Check log: $BUILD_DIR/logs/package.log"
    exit 1
fi

# Report the output artifact
BFF_PATH="$BUILD_DIR/datadog-agent-${AGENT_VERSION}-${AGENT_BUILD}.aix.ppc64.bff"
BUILD_END=$(date '+%Y-%m-%dT%H:%M:%S')

log "=== Build complete ==="
log "    Started : $BUILD_START"
log "    Finished: $BUILD_END"
log "    Package : $BFF_PATH"

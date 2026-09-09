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
# VRMF (installp package version) is X.Y.Z.AGENT_BUILD — derived in env.sh
# once AGENT_VERSION and AGENT_BUILD are both available.
# Package filename: datadog-agent-<AGENT_VERSION>-<AGENT_BUILD>.aix.ppc64.bff
#
# AGENT_SRC is resolved by env.sh: it walks up from the script directory until
# it finds a .git ancestor (so this build.sh must live inside a checkout of
# the datadog-agent repo).
# All intermediate artifacts go under /opt/dd-build/.

set -eu

PATH=/opt/go/bin:/opt/freeware/bin:/usr/sbin:/usr/bin:/bin:$PATH
export PATH

# ── Source shared environment ─────────────────────────────────────────────────

if [ -z "${AGENT_BUILD:-}" ]; then
    printf 'ERROR: AGENT_BUILD must be set (e.g. AGENT_BUILD=1)\n' >&2
    printf '       This is the installp build counter and must increase with each release.\n' >&2
    exit 1
fi

# dirname "$0" returns a relative path when the script is invoked with
# a relative path and stage scripts use SCRIPT_DIR after cd calls
# into build directories, so an absolute path is required.
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/lib/env.sh"

# ── Bootstrap embedded symlink ────────────────────────────────────────────────
# Python binaries (pip, ensurepip, etc.) have their shebangs baked at configure
# time as #!/opt/datadog-agent/embedded/bin/python3.13. Create a symlink from
# that path to the actual staging tree so they resolve during the build.
# Idempotent — only writes if missing or pointing at the wrong target.
if [ ! -L "$EMBEDDED" ] || [ "$(readlink "$EMBEDDED" 2>/dev/null)" != "$EMBEDDED_DESTDIR" ]; then
    if [ -e "$EMBEDDED" ] && [ ! -L "$EMBEDDED" ]; then
        rm -rf "$EMBEDDED"
    fi
    mkdir -p "$(dirname "$EMBEDDED")"
    ln -sf "$EMBEDDED_DESTDIR" "$EMBEDDED"
fi

# ── Build-specific prerequisite: Rust SDK ───────────────────────────────────
# The host toolchain (gcc, go, git, cmake, protoc, the -devel libs, ...) is
# provisioned and verified by packaging/aix/setup-host.sh; build.sh assumes
# it has been run. Fail fast here with a clear message if Rust is missing —
# the build needs it for stages 05/06/07/08.
check_tool() {
    _tool=$1; _pkg=${2:-$1}
    if ! command -v "$_tool" >/dev/null 2>&1; then
        printf 'ERROR: required tool not found: %s\n' "$_tool" >&2
        printf '       Install with: yum install %s\n' "$_pkg" >&2
        exit 1
    fi
}
check_tool "/opt/freeware/lib/RustSDK/${RUST_VERSION}/bin/cargo" \
    "rust${RUST_VERSION}.ppc cargo${RUST_VERSION}.ppc rust${RUST_VERSION}-std-static.ppc"

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
05-agent-data-plane
06-python-extensions
07-pydantic
08-checks-base
09-integrations
10-strip-bytecode
11-assemble
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

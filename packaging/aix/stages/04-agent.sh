#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="04-agent"
LOG="$BUILD_DIR/logs/$STAGE_NAME.log"

# Redirect all output to log file (follow with: tail -f "$LOG")
mkdir -p "$BUILD_DIR/logs"
exec > "$LOG" 2>&1

log "=== Stage: $STAGE_NAME ==="

# No completion sentinel: this stage always runs so the agent binaries stay in
# sync with their inputs (notably the rtloader archives they link against). Go
# caches compiled packages, so a rebuild with no changes is mostly a relink and
# stays reasonably fast.

# --- Input validation ---
: "${AGENT_VERSION:?AGENT_VERSION must be set}"
: "${STAGING:?STAGING must be set}"
: "${EMBEDDED:?EMBEDDED must be set}"
: "${EMBEDDED_DESTDIR:?EMBEDDED_DESTDIR must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"
: "${NPROC:?NPROC must be set}"
: "${CC:?CC must be set}"
: "${RUST_VERSION:?RUST_VERSION must be set}"
: "${GOPATH:?GOPATH must be set}"
: "${GOROOT:?GOROOT must be set}"
: "${CGO_ENABLED:?CGO_ENABLED must be set}"

# --- Pre-flight: confirm Stage 03 completed ---
if [ ! -f "$STAGING/opt/datadog-agent/rtloader/libdatadog-agent-rtloader.so" ]; then
    log "ERROR: libdatadog-agent-rtloader.so not found at $STAGING/opt/datadog-agent/rtloader — did Stage 03 (03-rtloader) complete successfully?"
    exit 1
fi

# --- Cleanup on failure ---
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed. Removing partial outputs."
        rm -rf "$STAGING/opt/datadog-agent/bin/agent"
        rm -f "$STAGING/opt/datadog-agent/embedded/bin/trace-agent"
        rm -f "$STAGING/opt/datadog-agent/embedded/bin/dd-procmgrd"
        rm -f "$STAGING/opt/datadog-agent/embedded/bin/dd-procmgr"
    fi
}
trap cleanup EXIT

# ─── Step 1: Create output directory ──────────────────────────────────────────

log "Creating staging bin directory"
mkdir -p "$STAGING/opt/datadog-agent/bin/agent"

# ─── Step 2: Set rtloader CGO flags ───────────────────────────────────────────
#
# The global CGO_CFLAGS/CGO_LDFLAGS from env.sh point to /opt/freeware headers
# and libs. We extend them here with rtloader-specific paths so that the Go
# packages that import rtloader (pkg/collector/python/) can find its C headers
# and link against the .so files we built in Stage 03.
#
# Note: we point -L at the BUILD paths (rtloader/build/rtloader and
# rtloader/build/three), not the staging paths. The .so files are in the build
# tree; the staging copies are for the final package only.

log "Setting rtloader CGO flags"
export CGO_CFLAGS="$CGO_CFLAGS -I$AGENT_SRC/rtloader/include"
#
# -lpython3 (via libpython3.a symlink) causes libpython3.a(shr_64.o) to appear in the agent binary's
# XCOFF startup-load chain. This is necessary but not sufficient: the binary
# must also EXPORT Python API symbols so that extension modules with IMPid="."
# (which means "look in the main program's export table") can find them.
#
# -Wl,-bE:python.exp adds ~2762 Python API symbols to the agent binary's own
# EXP (export) table. Go's CGO security filter rejects -bE in #cgo LDFLAGS,
# so it must be passed here via CGO_LDFLAGS instead. The python_aix.go file
# handles the Py_IsInitialized() call that creates the live Go→CGO reference
# needed to trigger the //go:cgo_import_dynamic for the Python shared library.
#
# The libpython3.a -> libpython3.X.a symlink is created by Stage 02.
PYTHON_EXP="$EMBEDDED_DESTDIR/lib/python${PYTHON_MAJ_MIN}/config-${PYTHON_MAJ_MIN}/python.exp"
if [ ! -f "$PYTHON_EXP" ]; then
    log "ERROR: $PYTHON_EXP not found — did Stage 02 (02-python) complete?"
    exit 1
fi
log "Using Python export file: $PYTHON_EXP"
export CGO_LDFLAGS="$CGO_LDFLAGS \
  -L$AGENT_SRC/rtloader/build/rtloader \
  -L$AGENT_SRC/rtloader/build/three \
  -L$EMBEDDED_DESTDIR/lib \
  -lpython3 \
  -Wl,-bE:$PYTHON_EXP \
  -Wl,-blibpath:/opt/datadog-agent/rtloader:/opt/datadog-agent/embedded/lib:/opt/freeware/lib64:/opt/freeware/lib:/usr/lib:/lib"

# ─── Step 3: Get commit hash ──────────────────────────────────────────────────

COMMIT=$(git -C "$AGENT_SRC" rev-parse --short HEAD)
log "Building agent version $AGENT_VERSION at commit $COMMIT"

# ─── Step 4: Build the agent binary ───────────────────────────────────────────
#
# Build tags and ldflags are determined by inv agent.build (tasks/build_tags.py).

log "Building agent binary via inv agent.build"
cd "$AGENT_SRC"
rm -f "$STAGING/opt/datadog-agent/bin/agent/agent-bin"
# OBJECT_MODE must be unset before Go's external linker runs. When both
# OBJECT_MODE=64 and AIX_OBJECT_MODE=64 are exported, the linker picks up
# the 32-bit /lib/crt0.o and fails. AIX_OBJECT_MODE=64 alone is sufficient
# for gcc-8 to select the 64-bit startup file. OBJECT_MODE=64 is still
# needed globally (env.sh) for tools like ar that don't respect AIX_OBJECT_MODE.
unset OBJECT_MODE
python3.12 -m invoke agent.build \
    --exclude-rtloader \
    --rtloader-root="$AGENT_SRC/rtloader" \
    --embedded-path="$EMBEDDED_DESTDIR" \
    --agent-bin="$STAGING/opt/datadog-agent/bin/agent/agent-bin"

strip -X64 "$STAGING/opt/datadog-agent/bin/agent/agent-bin"
log "agent binary build complete: $STAGING/opt/datadog-agent/bin/agent/agent-bin"

# Install the agent wrapper: sets LIBPATH/PATH so the binary works when invoked
# directly (not just via SRC). The wrapper execs agent-bin, so the process is
# replaced immediately — SRC PID tracking and signal handling are unaffected.
cp "$SCRIPT_DIR/../agent-wrapper.sh" "$STAGING/opt/datadog-agent/bin/agent/agent"
chmod 755 "$STAGING/opt/datadog-agent/bin/agent/agent"
log "agent wrapper installed at $STAGING/opt/datadog-agent/bin/agent/agent"

# ─── Step 5: Build the trace-agent binary ─────────────────────────────────────
#
# Build tags are determined by inv trace-agent.build (tasks/build_tags.py).

log "Building trace-agent binary via inv trace-agent.build"
cd "$AGENT_SRC"
python3.12 -m invoke trace-agent.build
mkdir -p "$STAGING/opt/datadog-agent/embedded/bin"
rm -f "$STAGING/opt/datadog-agent/embedded/bin/trace-agent-bin"
cp "$AGENT_SRC/bin/trace-agent/trace-agent" "$STAGING/opt/datadog-agent/embedded/bin/trace-agent-bin"
strip -X64 "$STAGING/opt/datadog-agent/embedded/bin/trace-agent-bin"
log "trace-agent binary build complete: $STAGING/opt/datadog-agent/embedded/bin/trace-agent-bin"

cp "$SCRIPT_DIR/../trace-agent-wrapper.sh" "$STAGING/opt/datadog-agent/embedded/bin/trace-agent"
chmod 755 "$STAGING/opt/datadog-agent/embedded/bin/trace-agent"
log "trace-agent wrapper installed at $STAGING/opt/datadog-agent/embedded/bin/trace-agent"

# ─── Step 6: Build the process-manager binaries ───────────────────────────────
#
# pkg/procmgr/rust is a member of the repo-root Cargo workspace, so the build
# runs from $AGENT_SRC against the checked-in Cargo.lock (--locked). Cargo
# resolves the whole workspace — including its git-sourced dependencies — so the
# first run needs network access; CARGO_HOME (env.sh) keeps the caches on the
# build volume.

log "Building dd-procmgrd and dd-procmgr via cargo"

CARGO=${CARGO:-/opt/freeware/lib/RustSDK/$RUST_VERSION/bin/cargo}
if [ ! -x "$CARGO" ]; then
    log "ERROR: cargo not found at $CARGO"
    exit 1
fi
# cargo resolves rustc from PATH, not from its own install directory.
PATH="$(dirname "$CARGO"):$PATH"
export PATH

# AIX-specific Rust build flags — the same set Stages 06/07 use for the Rust
# Python extensions, which is the recipe known to work with this SDK:
#   CARGO_PROFILE_RELEASE_STRIP=none  — IBM Rust bug: stripping the .info section
#                                       from proc-macro artifacts breaks rustc, and
#                                       cargo strips debuginfo in release by default.
#                                       Step 6 strips the shipped binaries with the
#                                       AIX strip afterwards instead.
#   CARGO_PROFILE_RELEASE_LTO=off     — the workspace release profile sets lto = "fat";
#                                       LLVM fat LTO uses .ipa bitcode sections that do
#                                       not exist in AIX XCOFF. Overriding from the
#                                       environment keeps the shared workspace manifest
#                                       platform-neutral.
#   RUSTFLAGS="-C link-arg=-bbigtoc"  — lifts AIX ld's 64KB TOC limit, which a binary
#                                       this size can exceed.
#   ARFLAGS unset                     — env.sh sets ARFLAGS="-X64 -cru", which includes
#                                       the ar operation code; cc-rs appends its own
#                                       "cq", producing "ar -cru cq" that AIX ar rejects
#                                       (two operation codes). OBJECT_MODE=64 already
#                                       ensures 64-bit archives.

RUSTFLAGS="-C link-arg=-bbigtoc${RUSTFLAGS:+ $RUSTFLAGS}"
CARGO_PROFILE_RELEASE_STRIP=none
CARGO_PROFILE_RELEASE_LTO=off
export RUSTFLAGS CARGO_PROFILE_RELEASE_STRIP CARGO_PROFILE_RELEASE_LTO
unset ARFLAGS

# Keep cargo's output off the (transferred) agent source tree, alongside the
# other build artefacts on the large build volume.
CARGO_TARGET_DIR=${CARGO_TARGET_DIR:-$BUILD_DIR/procmgr-target}
export CARGO_TARGET_DIR

cd "$AGENT_SRC"
# The crate inherits the workspace rust-version (1.94 today, tracking the rustc
# MODULE.bazel pins for the Bazel builds). That is a floor, not a pin, so the
# newer $RUST_VERSION SDK satisfies it; cargo only refuses to build if the SDK
# ever falls below it.
"$CARGO" build --release -p dd-procmgrd --bins

mkdir -p "$STAGING/opt/datadog-agent/embedded/bin"
for _bin in dd-procmgrd dd-procmgr; do
    _src="$CARGO_TARGET_DIR/release/$_bin"
    if [ ! -f "$_src" ]; then
        log "ERROR: $_bin not found at $_src"
        log "       cargo reported success but produced no binary — check the log above."
        exit 1
    fi
    _dest="$STAGING/opt/datadog-agent/embedded/bin/$_bin"
    rm -f "$_dest"
    cp "$_src" "$_dest"
    strip -X64 "$_dest"
    chmod 755 "$_dest"
    log "$_bin staged at $_dest"
done

# ─── Step 7: Verify XCOFF64 magic bytes ───────────────────────────────────────
#
# Every shipped binary must be XCOFF64 (magic bytes 01 f7). A non-XCOFF64 result
# would indicate a cross-compile or wrong-format build.

verify_xcoff64() {
    _bin=$1
    _hint=$2
    _name=$(basename "$_bin")
    log "Verifying $_name is XCOFF64"
    _magic=$(od -A x -t x1 "$_bin" | head -1 | awk '{print $2 $3}')
    if [ "$_magic" != "01f7" ]; then
        log "ERROR: $_name is not XCOFF64 (got: $_magic)"
        log "       Expected magic bytes: 01 f7"
        log "       $_hint"
        exit 1
    fi
    log "XCOFF64 magic verified for $_name (magic: $_magic)"
}

GO_HINT="Ensure CGO_ENABLED=1 and that GOROOT points to the AIX Go toolchain."
RUST_HINT="Ensure cargo comes from the AIX Rust SDK at /opt/freeware/lib/RustSDK/$RUST_VERSION."

verify_xcoff64 "$STAGING/opt/datadog-agent/bin/agent/agent-bin" "$GO_HINT"
verify_xcoff64 "$STAGING/opt/datadog-agent/embedded/bin/trace-agent-bin" "$GO_HINT"
verify_xcoff64 "$STAGING/opt/datadog-agent/embedded/bin/dd-procmgrd" "$RUST_HINT"
verify_xcoff64 "$STAGING/opt/datadog-agent/embedded/bin/dd-procmgr" "$RUST_HINT"

log "=== $STAGE_NAME complete ==="

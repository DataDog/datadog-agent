#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="03-rtloader"
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
: "${EMBEDDED:?EMBEDDED must be set}"
: "${EMBEDDED_DESTDIR:?EMBEDDED_DESTDIR must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"
: "${NPROC:?NPROC must be set}"
: "${CC:?CC must be set}"
: "${CXX:?CXX must be set}"
: "${CFLAGS:?CFLAGS must be set}"
: "${CXXFLAGS:?CXXFLAGS must be set}"
: "${LDFLAGS:?LDFLAGS must be set}"

# --- Pre-flight: confirm Stage 02 completed ---
if [ ! -f "$EMBEDDED_DESTDIR/lib/libpython3.13.so" ]; then
    log "ERROR: libpython3.13.so not found at $EMBEDDED_DESTDIR/lib — did Stage 02 (02-python) complete successfully?"
    exit 1
fi

# --- Cleanup on failure ---
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed. Removing partial outputs."
        rm -rf /opt/datadog-agent/rtloader/build
        rm -f "$STAGING/opt/datadog-agent/rtloader/libdatadog-agent-rtloader.so"
        rm -f "$STAGING/opt/datadog-agent/rtloader/libdatadog-agent-three.so"
    fi
}
trap cleanup EXIT

# ─── Step 1: Clean and create rtloader build directory ────────────────────────

log "Cleaning rtloader build directory"
rm -rf /opt/datadog-agent/rtloader/build
mkdir -p /opt/datadog-agent/rtloader/build

# ─── Step 2: CMake configure ──────────────────────────────────────────────────
#
# OBJECT_MODE=64 is required on AIX so that the IBM linker produces 64-bit
# XCOFF objects.  cmake is invoked from within the build directory.
#
# -DBUILD_DEMO=OFF      : skip the demo binary (not needed in the package)
# -DDISABLE_PYTHON2=ON  : only build the Python 3 binding (libdatadog-agent-three.so)
# -DPython3_INCLUDE_DIR : staging path (where headers are during the build)
# -DPython3_LIBRARY     : staging path (where libpython3.13.so is during the build)

log "Running cmake for rtloader"
cd /opt/datadog-agent/rtloader/build

OBJECT_MODE=64 cmake \
    -DCMAKE_C_COMPILER="$CC" \
    -DCMAKE_CXX_COMPILER="$CXX" \
    -DCMAKE_C_FLAGS="$CFLAGS" \
    -DCMAKE_CXX_FLAGS="$CXXFLAGS" \
    -DCMAKE_SHARED_LINKER_FLAGS="$LDFLAGS" \
    -DBUILD_DEMO=OFF \
    -DDISABLE_PYTHON2=ON \
    -DPython3_INCLUDE_DIR="$EMBEDDED_DESTDIR/include/python3.13" \
    -DPython3_LIBRARY="$EMBEDDED_DESTDIR/lib/libpython3.13.so" \
    ..

log "cmake configure complete."

# ─── Step 3: Build ────────────────────────────────────────────────────────────

log "Building rtloader with make -j$NPROC"
OBJECT_MODE=64 make -j"$NPROC"
log "rtloader build complete."

# ─── Step 4: Copy outputs to staging ──────────────────────────────────────────
#
# The two produced .so files must land in $STAGING/opt/datadog-agent/rtloader/
# so the agent binary can find them at runtime via LIBPATH.

log "Copying rtloader .so files to staging"
mkdir -p "$STAGING/opt/datadog-agent/rtloader"
cp rtloader/libdatadog-agent-rtloader.so \
   three/libdatadog-agent-three.so \
   "$STAGING/opt/datadog-agent/rtloader/"
log "Copy complete."

# ─── Step 4b: Create AIX .a archive wrappers ──────────────────────────────────
#
# On AIX, Go's CGO requires shared libraries wrapped in .a archives.
# Without these, Go cannot generate correct //go:cgo_import_dynamic directives
# (which must reference "lib.a/lib.so" format).
# Archives are created in the build tree where CGO_LDFLAGS points.

log "Creating .a archive wrappers for rtloader .so files (AIX CGO requirement)"
# On AIX, Go's compiler (lex.go) requires the archive member name to either end in
# ".o" or contain ".so." (a version number).  The conventional AIX name for the
# 64-bit shared module inside an archive is "shr_64.o".
cd /opt/datadog-agent/rtloader/build/rtloader
cp libdatadog-agent-rtloader.so shr_64.o
ar -X64 -r libdatadog-agent-rtloader.a shr_64.o
rm -f shr_64.o
cd /opt/datadog-agent/rtloader/build/three
cp libdatadog-agent-three.so shr_64.o
ar -X64 -r libdatadog-agent-three.a shr_64.o
rm -f shr_64.o
log "Archive wrappers created (member: shr_64.o in each .a)."

# ─── Step 5: Verify XCOFF64 magic bytes ───────────────────────────────────────
#
# XCOFF64 files begin with magic bytes 01 f7 (big-endian 0x01F7 = XCOFF64_MAGIC).
# We read the first line of od output (8 bytes) and check that the first two bytes
# match.  If they do not, the build produced a wrong-format binary.

log "Verifying libdatadog-agent-three.so is XCOFF64"
MAGIC=$(od -A x -t x1 "$STAGING/opt/datadog-agent/rtloader/libdatadog-agent-three.so" | head -1 | awk '{print $2 $3}')
if [ "$MAGIC" != "01f7" ]; then
    log "ERROR: libdatadog-agent-three.so is not XCOFF64 (got: $MAGIC)"
    log "       Expected magic bytes: 01 f7"
    log "       Check that OBJECT_MODE=64 is set and that $CXX produces 64-bit XCOFF output."
    exit 1
fi
log "XCOFF64 magic verified for libdatadog-agent-three.so (magic: $MAGIC)"

log "Verifying libdatadog-agent-rtloader.so is XCOFF64"
MAGIC=$(od -A x -t x1 "$STAGING/opt/datadog-agent/rtloader/libdatadog-agent-rtloader.so" | head -1 | awk '{print $2 $3}')
if [ "$MAGIC" != "01f7" ]; then
    log "ERROR: libdatadog-agent-rtloader.so is not XCOFF64 (got: $MAGIC)"
    log "       Expected magic bytes: 01 f7"
    log "       Check that OBJECT_MODE=64 is set and that $CXX produces 64-bit XCOFF output."
    exit 1
fi
log "XCOFF64 magic verified for libdatadog-agent-rtloader.so (magic: $MAGIC)"

# --- Mark complete ---
mkdir -p "$(dirname "$SENTINEL")"
touch "$SENTINEL"
log "=== $STAGE_NAME complete ==="

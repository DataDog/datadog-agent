#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="09-strip-bytecode"
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

# --- Cleanup on failure ---
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed. Removing partial outputs."
        rm -f "$EMBEDDED_DESTDIR/.pyc_compiled_files.txt"
    fi
}
trap cleanup EXIT

# ─── Step 1: Strip debug info from shared libraries ───────────────────────────
#
# /opt/freeware/bin/strip supports -X64 (required for XCOFF64 binaries on AIX).
# The system /usr/bin/strip is 32-bit only and will refuse or silently corrupt
# 64-bit XCOFF objects.  Use while-read rather than for-f-in-$(find) to avoid
# command substitution size limits and to handle filenames with spaces safely.

log "Stripping debug info from .so files under $EMBEDDED_DESTDIR/lib"
find "$EMBEDDED_DESTDIR/lib" -name "*.so*" | while read f; do
    /opt/freeware/bin/strip -X64 "$f" 2>/dev/null || true
done
log "Strip pass complete"

# ─── Step 2: Remove build artefacts not needed at runtime ─────────────────────
#
# Headers, pkg-config metadata, and man pages are only needed during compilation.
# Source files (.c/.h) inside the Python stdlib tree are not needed at runtime.
# __pycache__ directories may contain stale .pyc files from an earlier compileall
# run or a pip install; delete them before the fresh compileall in Step 4 so
# there are no stale bytecode files with incorrect magic numbers.

log "Removing build-time artefacts (headers, pkgconfig, man pages, .c/.h files)"
rm -rf "$EMBEDDED_DESTDIR/include"
rm -rf "$EMBEDDED_DESTDIR/lib/pkgconfig"
rm -rf "$EMBEDDED_DESTDIR/share/man"
find "$EMBEDDED_DESTDIR/lib/python3.13" -name "*.c" -delete
find "$EMBEDDED_DESTDIR/lib/python3.13" -name "*.h" -delete
find "$EMBEDDED_DESTDIR/lib/python3.13" -depth -name "__pycache__" \
    -exec sh -c 'find "$1" -name "*.pyc" -delete' _ {} \; 2>/dev/null || true
log "Build artefacts removed"

# ─── Step 3: Compile .py to .pyc for faster agent startup ─────────────────────
#
# -x 'test/' skips test directories which contain code that may not compile
# cleanly or that imports Linux-only modules.  -q suppresses per-file output;
# errors (if any) are still printed.  || true: a few files may legitimately
# fail to compile (e.g. Python 2-only syntax in vendored code); do not abort
# the stage for these.

log "Compiling .py to .pyc under $EMBEDDED_DESTDIR/lib/python3.13/site-packages"
"$EMBEDDED_DESTDIR/bin/python3.13" -m compileall \
    "$EMBEDDED_DESTDIR/lib/python3.13/site-packages" \
    -x 'test/' -q 2>/dev/null || true
log "Bytecode compilation complete"

# ─── Step 4: Record compiled bytecode files ───────────────────────────────────
#
# The preinst and prerm package scripts read this manifest to delete .pyc files
# before file replacement (upgrade) or removal (uninstall).  mkinstallp does not
# track .pyc files created post-install (they are generated at package-build time
# and again at postinst time), so the manifest is the only reliable way to clean
# them up without leaving orphaned bytecode behind.

log "Recording .pyc/.pyo files to $EMBEDDED_DESTDIR/.pyc_compiled_files.txt"
find "$EMBEDDED_DESTDIR" \( -name "*.pyc" -o -name "*.pyo" \) -print \
    > "$EMBEDDED_DESTDIR/.pyc_compiled_files.txt"
log "Recorded $(wc -l < "$EMBEDDED_DESTDIR/.pyc_compiled_files.txt") .pyc files"

# --- Mark complete ---
mkdir -p "$(dirname "$SENTINEL")"
touch "$SENTINEL"
log "=== $STAGE_NAME complete ==="

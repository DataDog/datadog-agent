#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="09-strip-bytecode"
LOG="$BUILD_DIR/logs/$STAGE_NAME.log"

# Redirect all output to log file (follow with: tail -f "$LOG")
mkdir -p "$BUILD_DIR/logs"
exec > "$LOG" 2>&1

log "=== Stage: $STAGE_NAME ==="

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
# strip -X64 selects 64-bit XCOFF object mode; the system /usr/bin/strip
# handles XCOFF64 correctly on AIX 7.x.
# -type f skips symlinks so each physical file is processed exactly once
# (the lib directory contains versioned symlinks like libxml2.so -> libxml2.so.16.0.5).
# Use while-read rather than for-f-in-$(find) to avoid command substitution
# size limits and to handle filenames with spaces safely.

log "Stripping debug info from .so files under $EMBEDDED_DESTDIR/lib"
find "$EMBEDDED_DESTDIR/lib" -type f -name "*.so*" | while IFS= read -r f; do
    # AIX strip exits 255 with "0654-420 already stripped" for files distributed
    # without debug symbols (e.g. toolbox libs like liblzma, libxml2); ignore it.
    strip -X64 "$f" 2>/dev/null || true
done
log "Strip pass complete"

# ─── Step 2: Remove build artefacts not needed at runtime ─────────────────────
#
# pkg-config metadata and man pages are only needed during compilation.
# Source files (.c/.h) inside the Python stdlib tree are not needed at runtime.
# Note: embedded/include (Python.h etc.) is intentionally kept so users can
# build C extension packages (e.g. ibm_db) against the embedded Python.
# __pycache__ directories may contain stale .pyc files from an earlier compileall
# run or a pip install; delete them before the fresh compileall in Step 4 so
# there are no stale bytecode files with incorrect magic numbers.

log "Removing build-time artefacts (pkgconfig, man pages, .c/.h files)"
# Keep embedded/include (Python headers) — users need Python.h to build C
# extensions such as ibm_db. Linux/macOS omnibus packages also ship these
# headers; we match that behaviour here.
rm -rf "$EMBEDDED_DESTDIR/lib/pkgconfig"
rm -rf "$EMBEDDED_DESTDIR/share/man"
find "$EMBEDDED_DESTDIR/lib/python${PYTHON_MAJ_MIN}" -name "*.c" -exec rm -f {} \;
find "$EMBEDDED_DESTDIR/lib/python${PYTHON_MAJ_MIN}" -name "*.h" -exec rm -f {} \;
find "$EMBEDDED_DESTDIR/lib/python${PYTHON_MAJ_MIN}" -name "*.pyc" -exec rm -f {} \; 2>/dev/null || true
log "Build artefacts removed"

# ─── Step 2b: Fix Python entry-point script shebangs ─────────────────────────
#
# pip and other Python packages install wrapper scripts (pip3.13, easy_install,
# etc.) whose shebang is set to sys.executable at install time. Because Python
# runs from the staging tree ($EMBEDDED_DESTDIR) during the build, sys.executable
# resolves via realpath() to the staging path, not the final install path
# ($EMBEDDED). The resulting shebang is therefore wrong on the target host and
# causes "No such file or directory" when any of these scripts are invoked.
#
# Fix: rewrite every non-symlink script in embedded/bin/ whose first line
# contains the staging path, replacing it with the final install path.

log "Fixing Python entry-point script shebangs ($EMBEDDED_DESTDIR -> $EMBEDDED)"
find "$EMBEDDED_DESTDIR/bin" -type f | while IFS= read -r f; do
    case $(head -1 "$f" 2>/dev/null) in
        "#!${EMBEDDED_DESTDIR}/bin/python"*)
            cp -p "$f" "${f}.tmp" && sed "1s|#!${EMBEDDED_DESTDIR}/bin/|#!${EMBEDDED}/bin/|" "$f" > "${f}.tmp" && mv "${f}.tmp" "$f"
            log "Fixed shebang: $(basename "$f")"
            ;;
    esac
done
log "Shebang fix complete"

# ─── Step 3: Compile .py to .pyc for faster agent startup ─────────────────────
#
# -x 'test/' skips test directories which contain code that may not compile
# cleanly or that imports Linux-only modules. -q suppresses per-file output;
# errors (if any) are still printed. || true: a few files may legitimately
# fail to compile (e.g. Python 2-only syntax in vendored code); do not abort
# the stage for these.

log "Compiling .py to .pyc under $EMBEDDED_DESTDIR/lib/python${PYTHON_MAJ_MIN}/site-packages"
"$EMBEDDED_DESTDIR/bin/python${PYTHON_MAJ_MIN}" -m compileall \
    "$EMBEDDED_DESTDIR/lib/python${PYTHON_MAJ_MIN}/site-packages" \
    -x 'test/' -q 2>/dev/null || true
log "Bytecode compilation complete"

# ─── Step 4: Record compiled bytecode files ───────────────────────────────────
#
# The preinst and prerm package scripts read this manifest to delete .pyc files
# before file replacement (upgrade) or removal (uninstall). mkinstallp does not
# track .pyc files created post-install (they are generated at package-build time
# and again at postinst time), so the manifest is the only reliable way to clean
# them up without leaving orphaned bytecode behind.

log "Recording .pyc/.pyo files to $EMBEDDED_DESTDIR/.pyc_compiled_files.txt"
find "$EMBEDDED_DESTDIR" \( -name "*.pyc" -o -name "*.pyo" \) -print \
    | sed "s|^$STAGING||" \
    > "$EMBEDDED_DESTDIR/.pyc_compiled_files.txt"
log "Recorded $(wc -l < "$EMBEDDED_DESTDIR/.pyc_compiled_files.txt") .pyc files"

log "=== $STAGE_NAME complete ==="

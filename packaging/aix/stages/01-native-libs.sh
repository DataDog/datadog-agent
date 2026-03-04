#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="01-native-libs"
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
: "${STAGING:?STAGING must be set}"
: "${EMBEDDED:?EMBEDDED must be set}"
: "${EMBEDDED_DESTDIR:?EMBEDDED_DESTDIR must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"
: "${NPROC:?NPROC must be set}"
: "${CC:?CC must be set}"
: "${CFLAGS:?CFLAGS must be set}"

# --- Cleanup on failure ---
CURRENT_LIB=
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed (building: ${CURRENT_LIB:-unknown})."
        if [ -n "$CURRENT_LIB" ]; then
            log "Removing partial build directory for $CURRENT_LIB"
            rm -rf "$BUILD_DIR/build/$CURRENT_LIB"
        fi
    fi
}
trap cleanup EXIT

# --- Prepare source and build directories ---
mkdir -p "$BUILD_DIR/sources"
mkdir -p "$BUILD_DIR/build"
mkdir -p "$EMBEDDED_DESTDIR/bin"
mkdir -p "$EMBEDDED_DESTDIR/lib"
mkdir -p "$EMBEDDED_DESTDIR/lib/pkgconfig"
mkdir -p "$EMBEDDED_DESTDIR/include"
mkdir -p "$EMBEDDED_DESTDIR/share"

# ─── Version pins ──────────────────────────────────────────────────────────────
#
# Versions of built-from-source libraries are kept in sync with the Linux
# omnibus pipeline.  Source of truth: deps/repos.MODULE.bazel.
#
# Libraries taken from AIX Toolbox (yum install <pkg>-devel) use whatever
# version is provided by the toolbox.
#
ZLIB_VERSION="1.3.1"
BZIP2_VERSION="1.0.8"
OPENSSL_VERSION="3.5.5"
XZ_VERSION="5.8.1"
LIBXML2_VERSION="2.14.5"    # built from source (AIX Toolbox also available but we build)
LIBXSLT_VERSION="1.1.45"   # from AIX Toolbox (yum install libxslt-devel; source build fails on AIX)

# These are sourced from AIX Toolbox (build from source fails on AIX)
LIBFFI_VERSION="3.4.4"     # yum install libffi-devel
NCURSES_VERSION="6.5"      # yum install ncurses-devel
READLINE_VERSION="8.2"     # yum install readline-devel
SQLITE_VERSION="3.50.4"    # yum install sqlite-devel
GDBM_VERSION="1.23"        # yum install gdbm-devel

# ─── Helpers ──────────────────────────────────────────────────────────────────

# AIX system tar does not support decompression flags (-z, -j, -J).
# Always decompress explicitly and pipe into tar xf -.
extract_gz() {
    gunzip -c "$1" | tar xf - -C "$2"
}

extract_xz() {
    xz -dc "$1" | tar xf - -C "$2"
}

# lib_done NAME VERSION — true if library is already installed to staging
lib_done() {
    [ -f "$BUILD_DIR/.done/01-lib-$1-$2" ]
}

# lib_mark NAME VERSION — record successful installation
lib_mark() {
    mkdir -p "$BUILD_DIR/.done"
    touch "$BUILD_DIR/.done/01-lib-$1-$2"
}

# stage_toolbox_lib LIB_NAME VERSION LIB64 HEADERS...
#   Copy a library and headers from AIX Toolbox into the staging tree.
#   LIB64 = full path of the 64-bit .a file
#   HEADERS = space-separated list of header source paths to copy
stage_toolbox_lib() {
    _name=$1; _ver=$2; _lib64=$3
    shift 3
    if lib_done "$_name" "$_ver"; then
        log "${_name} ${_ver} already staged — skipping"
        return 0
    fi
    log "Staging ${_name} ${_ver} (from AIX Toolbox)"
    if [ ! -f "$_lib64" ]; then
        log "ERROR: ${_name} library not found: $_lib64"
        log "       Install with: yum install ${_name}-devel"
        exit 1
    fi
    cp "$_lib64" "$EMBEDDED_DESTDIR/lib/"
    for _hdr in "$@"; do
        if [ -f "$_hdr" ]; then
            # Preserve one level of subdirectory (e.g. readline/readline.h)
            _subdir=$(dirname "$_hdr" | sed "s|/opt/freeware/include||")
            if [ -n "$_subdir" ] && [ "$_subdir" != "/" ] && [ "$_subdir" != "." ]; then
                mkdir -p "$EMBEDDED_DESTDIR/include/$_subdir"
                cp "$_hdr" "$EMBEDDED_DESTDIR/include/$_subdir/"
            else
                cp "$_hdr" "$EMBEDDED_DESTDIR/include/"
            fi
        fi
    done
    lib_mark "$_name" "$_ver"
    log "${_name} ${_ver} staged"
}

# ─── Open-file limit ──────────────────────────────────────────────────────────
# shellcheck disable=SC3045  # ulimit -n is supported by AIX sh (not POSIX sh)
ulimit -n 65536
# shellcheck disable=SC3045
log "Open-file limit raised to $(ulimit -n)"

# ─── Library build / stage functions ─────────────────────────────────────────
# Libraries are processed sequentially.
# Per-library sentinels allow re-runs to skip already-completed libraries.

# ── zlib (build from source) ──────────────────────────────────────────────────
if lib_done zlib "$ZLIB_VERSION"; then
    log "zlib ${ZLIB_VERSION} already installed — skipping"
else
    CURRENT_LIB="zlib-${ZLIB_VERSION}"
    log "Building zlib ${ZLIB_VERSION}"
    TARBALL="$BUILD_DIR/sources/zlib-${ZLIB_VERSION}.tar.gz"
    # Primary: GitHub releases (canonical, stable across versions)
    # Fallback: zlib.net (older releases moved to /fossils/)
    [ -f "$TARBALL" ] || curl -fSL -o "$TARBALL" \
        "https://github.com/madler/zlib/releases/download/v${ZLIB_VERSION}/zlib-${ZLIB_VERSION}.tar.gz" || \
        curl -fSL -o "$TARBALL" "https://zlib.net/fossils/zlib-${ZLIB_VERSION}.tar.gz" || \
        curl -fSL -o "$TARBALL" "https://zlib.net/zlib-${ZLIB_VERSION}.tar.gz"
    rm -rf "$BUILD_DIR/build/zlib-${ZLIB_VERSION}"
    extract_gz "$TARBALL" "$BUILD_DIR/build"
    cd "$BUILD_DIR/build/zlib-${ZLIB_VERSION}"
    ./configure --prefix="$EMBEDDED"
    make -j"$NPROC"
    make install DESTDIR="$STAGING"
    cd "$BUILD_DIR"
    lib_mark zlib "$ZLIB_VERSION"
    log "zlib ${ZLIB_VERSION} done"
    CURRENT_LIB=
fi

# ── bzip2 (build from source, static only) ────────────────────────────────────
if lib_done bzip2 "$BZIP2_VERSION"; then
    log "bzip2 ${BZIP2_VERSION} already installed — skipping"
else
    CURRENT_LIB="bzip2-${BZIP2_VERSION}"
    log "Building bzip2 ${BZIP2_VERSION}"
    TARBALL="$BUILD_DIR/sources/bzip2-${BZIP2_VERSION}.tar.gz"
    [ -f "$TARBALL" ] || curl -fSL -o "$TARBALL" "https://sourceware.org/pub/bzip2/bzip2-${BZIP2_VERSION}.tar.gz"
    rm -rf "$BUILD_DIR/build/bzip2-${BZIP2_VERSION}"
    extract_gz "$TARBALL" "$BUILD_DIR/build"
    cd "$BUILD_DIR/build/bzip2-${BZIP2_VERSION}"
    # AIX does not support Linux-style .so shared libs for bzip2 (gcc -shared + -Wl,-soname fails).
    # Build and install the static library only; Python links against libbz2.a at compile time.
    make CC="$CC" CFLAGS="$CFLAGS"
    make install PREFIX="$EMBEDDED" DESTDIR="$STAGING"
    cd "$BUILD_DIR"
    lib_mark bzip2 "$BZIP2_VERSION"
    log "bzip2 ${BZIP2_VERSION} done"
    CURRENT_LIB=
fi

# ── OpenSSL (build from source) ───────────────────────────────────────────────
if lib_done openssl "$OPENSSL_VERSION"; then
    log "OpenSSL ${OPENSSL_VERSION} already installed — skipping"
else
    CURRENT_LIB="openssl-${OPENSSL_VERSION}"
    log "Building OpenSSL ${OPENSSL_VERSION}"
    TARBALL="$BUILD_DIR/sources/openssl-${OPENSSL_VERSION}.tar.gz"
    # openssl.org/source redirects to GitHub for recent releases; use GitHub directly
    [ -f "$TARBALL" ] || curl -fSL -o "$TARBALL" \
        "https://github.com/openssl/openssl/releases/download/openssl-${OPENSSL_VERSION}/openssl-${OPENSSL_VERSION}.tar.gz" || \
        curl -fSL -o "$TARBALL" "https://www.openssl.org/source/openssl-${OPENSSL_VERSION}.tar.gz"
    rm -rf "$BUILD_DIR/build/openssl-${OPENSSL_VERSION}"
    extract_gz "$TARBALL" "$BUILD_DIR/build"
    cd "$BUILD_DIR/build/openssl-${OPENSSL_VERSION}"
    ./Configure aix64-gcc \
        --prefix="$EMBEDDED" \
        --openssldir="$EMBEDDED/ssl" \
        -Wl,-brtl \
        shared
    make -j"$NPROC"
    make install_sw DESTDIR="$STAGING"
    cd "$BUILD_DIR"
    lib_mark openssl "$OPENSSL_VERSION"
    log "OpenSSL ${OPENSSL_VERSION} done"
    CURRENT_LIB=
fi

# ── xz (build from source) ────────────────────────────────────────────────────
if lib_done xz "$XZ_VERSION"; then
    log "xz ${XZ_VERSION} already installed — skipping"
else
    CURRENT_LIB="xz-${XZ_VERSION}"
    log "Building xz ${XZ_VERSION}"
    TARBALL="$BUILD_DIR/sources/xz-${XZ_VERSION}.tar.gz"
    # tukaani.org redirects to GitHub; use GitHub directly
    [ -f "$TARBALL" ] || curl -fSL -o "$TARBALL" \
        "https://github.com/tukaani-project/xz/releases/download/v${XZ_VERSION}/xz-${XZ_VERSION}.tar.gz" || \
        curl -fSL -o "$TARBALL" "https://tukaani.org/xz/xz-${XZ_VERSION}.tar.gz"
    rm -rf "$BUILD_DIR/build/xz-${XZ_VERSION}"
    extract_gz "$TARBALL" "$BUILD_DIR/build"
    cd "$BUILD_DIR/build/xz-${XZ_VERSION}"
    ./configure \
        --prefix="$EMBEDDED" \
        --disable-static
    make -j"$NPROC"
    make install DESTDIR="$STAGING"
    cd "$BUILD_DIR"
    lib_mark xz "$XZ_VERSION"
    log "xz ${XZ_VERSION} done"
    CURRENT_LIB=
fi

# ── libffi (AIX Toolbox: yum install libffi-devel) ────────────────────────────
#
# libffi source build fails on AIX: configure explicitly rejects powerpc64-ibm-aix
# and the autoconf bootstrap also fails.  The AIX Toolbox provides a working
# 64-bit libffi.  We stage it into the embedded tree so it ships with the package.
#
stage_toolbox_lib libffi "$LIBFFI_VERSION" \
    /opt/freeware/lib64/libffi.a \
    /opt/freeware/include/ffi.h \
    /opt/freeware/include/ffi_common.h \
    /opt/freeware/include/ffitarget.h
# Write pkg-config file
if [ ! -f "$EMBEDDED_DESTDIR/lib/pkgconfig/libffi.pc" ]; then
    cat > "$EMBEDDED_DESTDIR/lib/pkgconfig/libffi.pc" <<PCEOF
prefix=$EMBEDDED
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include
Name: libffi
Description: Library supporting Foreign Function Interfaces
Version: ${LIBFFI_VERSION}
Libs: -L\${libdir} -lffi
Cflags: -I\${includedir}
PCEOF
fi

# ── ncurses (AIX Toolbox: yum install ncurses-devel) ─────────────────────────
#
# ncurses source build fails on AIX: the ar command gets an "unknown" format
# argument from ncurses's configure, causing the archive creation to fail.
# The AIX Toolbox provides ncurses 6.5 with both narrow and wide-char variants.
#
if lib_done ncurses "$NCURSES_VERSION"; then
    log "ncurses ${NCURSES_VERSION} already staged — skipping"
else
    log "Staging ncurses ${NCURSES_VERSION} (from AIX Toolbox)"
    for lib in ncurses ncursesw; do
        [ -f "/opt/freeware/lib64/lib${lib}.a" ] && \
            cp "/opt/freeware/lib64/lib${lib}.a" "$EMBEDDED_DESTDIR/lib/"
    done
    for hdr in ncurses.h curses.h term.h termcap.h ncurses_dll.h; do
        [ -f "/opt/freeware/include/$hdr" ] && cp "/opt/freeware/include/$hdr" "$EMBEDDED_DESTDIR/include/"
    done
    mkdir -p "$EMBEDDED_DESTDIR/include/ncurses"
    for hdr in /opt/freeware/include/ncurses/*.h; do
        [ -f "$hdr" ] && cp "$hdr" "$EMBEDDED_DESTDIR/include/ncurses/"
    done
    # pkg-config
    for lib in ncurses ncursesw; do
        cat > "$EMBEDDED_DESTDIR/lib/pkgconfig/${lib}.pc" <<PCEOF
prefix=$EMBEDDED
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include
Name: ${lib}
Description: Ncurses library
Version: ${NCURSES_VERSION}
Libs: -L\${libdir} -l${lib}
Cflags: -I\${includedir}
PCEOF
    done
    lib_mark ncurses "$NCURSES_VERSION"
    log "ncurses ${NCURSES_VERSION} staged"
fi

# ── readline (AIX Toolbox: yum install readline-devel) ───────────────────────
stage_toolbox_lib readline "$READLINE_VERSION" \
    /opt/freeware/lib64/libreadline.a \
    /opt/freeware/include/readline/readline.h \
    /opt/freeware/include/readline/history.h \
    /opt/freeware/include/readline/keymaps.h \
    /opt/freeware/include/readline/rlconf.h \
    /opt/freeware/include/readline/rlstdc.h \
    /opt/freeware/include/readline/rltypedefs.h \
    /opt/freeware/include/readline/tilde.h
[ -f /opt/freeware/lib64/libhistory.a ] && \
    cp /opt/freeware/lib64/libhistory.a "$EMBEDDED_DESTDIR/lib/"

# ── SQLite (AIX Toolbox: yum install sqlite-devel) ───────────────────────────
stage_toolbox_lib sqlite "$SQLITE_VERSION" \
    /opt/freeware/lib64/libsqlite3.a \
    /opt/freeware/include/sqlite3.h \
    /opt/freeware/include/sqlite3ext.h

# ── gdbm (AIX Toolbox: yum install gdbm-devel) ───────────────────────────────
#
# gdbm-devel provides libgdbm.a in /opt/freeware/lib (no lib64 variant).
# The archive contains 64-bit shared objects (verified with ar -X64 -t).
#
stage_toolbox_lib gdbm "$GDBM_VERSION" \
    /opt/freeware/lib/libgdbm.a \
    /opt/freeware/include/gdbm.h
# gdbm_compat for Python's dbm.ndbm module
[ -f /opt/freeware/lib/libgdbm_compat.a ] && \
    cp /opt/freeware/lib/libgdbm_compat.a "$EMBEDDED_DESTDIR/lib/"
[ -f /opt/freeware/include/gdbm-ndbm.h ] && \
    cp /opt/freeware/include/gdbm-ndbm.h "$EMBEDDED_DESTDIR/include/"

# ── libxml2 (build from source) ───────────────────────────────────────────────
if lib_done libxml2 "$LIBXML2_VERSION"; then
    log "libxml2 ${LIBXML2_VERSION} already installed — skipping"
else
    CURRENT_LIB="libxml2-${LIBXML2_VERSION}"
    log "Building libxml2 ${LIBXML2_VERSION}"
    TARBALL="$BUILD_DIR/sources/libxml2-${LIBXML2_VERSION}.tar.xz"
    LIBXML2_MINOR=$(echo "$LIBXML2_VERSION" | sed 's/\.[0-9]*$//')
    [ -f "$TARBALL" ] || curl -fSL -o "$TARBALL" \
        "https://download.gnome.org/sources/libxml2/${LIBXML2_MINOR}/libxml2-${LIBXML2_VERSION}.tar.xz"
    rm -rf "$BUILD_DIR/build/libxml2-${LIBXML2_VERSION}"
    extract_xz "$TARBALL" "$BUILD_DIR/build"
    cd "$BUILD_DIR/build/libxml2-${LIBXML2_VERSION}"
    ./configure \
        --prefix="$EMBEDDED" \
        --without-python \
        --disable-static
    make -j"$NPROC"
    make install DESTDIR="$STAGING"
    cd "$BUILD_DIR"
    lib_mark libxml2 "$LIBXML2_VERSION"
    log "libxml2 ${LIBXML2_VERSION} done"
    CURRENT_LIB=
fi

# ── libxslt (AIX Toolbox: yum install libxslt-devel) ─────────────────────────
#
# libxslt source build fails on AIX (configure-generated Makefile has incorrect
# include paths, causing fatal errors for libxslt.h during compilation).
# The AIX Toolbox provides libxslt 1.1.45.
#
stage_toolbox_lib libxslt "$LIBXSLT_VERSION" \
    /opt/freeware/lib/libxslt.a
# Also stage libexslt (EXSLT extension library, needed by lxml)
[ -f /opt/freeware/lib/libexslt.a ] && \
    cp /opt/freeware/lib/libexslt.a "$EMBEDDED_DESTDIR/lib/"
# Headers: copy libxslt/ and libexslt/ include subdirectories
for xdir in libxslt libexslt; do
    if [ -d "/opt/freeware/include/$xdir" ]; then
        mkdir -p "$EMBEDDED_DESTDIR/include/$xdir"
        cp /opt/freeware/include/"$xdir"/*.h "$EMBEDDED_DESTDIR/include/$xdir/"
    fi
done
# pkg-config
for lib in libxslt libexslt; do
    if [ -f "/opt/freeware/lib/pkgconfig/${lib}.pc" ]; then
        cp "/opt/freeware/lib/pkgconfig/${lib}.pc" "$EMBEDDED_DESTDIR/lib/pkgconfig/"
    fi
done

# ─── Ensure standard directories exist ────────────────────────────────────────

log "Ensuring standard embedded directories exist"
mkdir -p "$EMBEDDED_DESTDIR/bin"
mkdir -p "$EMBEDDED_DESTDIR/lib"
mkdir -p "$EMBEDDED_DESTDIR/include"
mkdir -p "$EMBEDDED_DESTDIR/share"

# --- Mark complete ---
mkdir -p "$(dirname "$SENTINEL")"
touch "$SENTINEL"
log "=== $STAGE_NAME complete ==="

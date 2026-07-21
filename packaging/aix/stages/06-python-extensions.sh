#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="06-python-extensions"
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
: "${EMBEDDED_DESTDIR:?EMBEDDED_DESTDIR must be set}"
: "${BUILD_DIR:?BUILD_DIR must be set}"
: "${WHEEL_CACHE:?WHEEL_CACHE must be set}"
: "${INTEGRATIONS_CORE:?INTEGRATIONS_CORE must be set}"

PIP=$EMBEDDED_DESTDIR/bin/pip${PYTHON_MAJ_MIN}
PYTHON_BIN=$EMBEDDED_DESTDIR/bin/python${PYTHON_MAJ_MIN}

# ── Derive C-extension versions from the pinned integrations-core checkout ────
#
# These packages are C extensions that must be compiled from source on AIX.
# Their versions are read from integrations-core rather than hardcoded here so
# that updating integrations-core (via INTEGRATIONS_CORE_VERSION in release.json)
# automatically picks up the correct version without a separate edit to this file.
#
# Sources:
#   pymqi, pyodbc  — integrations-core/agent_requirements.in (format: "pkg==x.y.z")

AGENT_REQ="$INTEGRATIONS_CORE/agent_requirements.in"
PYMQI_VERSION=$(grep '^pymqi==' "$AGENT_REQ" | cut -d= -f3)
PYODBC_VERSION=$(grep '^pyodbc==' "$AGENT_REQ" | cut -d= -f3)

if [ -z "$PYMQI_VERSION" ] || [ -z "$PYODBC_VERSION" ]; then
    log "ERROR: could not read one or more C-extension versions from integrations-core"
    log "  PYMQI_VERSION='$PYMQI_VERSION'  (source: $AGENT_REQ)"
    log "  PYODBC_VERSION='$PYODBC_VERSION'  (source: $AGENT_REQ)"
    exit 1
fi
log "C-extension versions from integrations-core: pymqi=$PYMQI_VERSION pyodbc=$PYODBC_VERSION"

# --- Pre-flight: confirm pip${PYTHON_MAJ_MIN} exists ---
if [ ! -x "$PIP" ]; then
    log "ERROR: $PIP not found — did Stage 02 (02-python) complete successfully?"
    exit 1
fi

# --- Pre-flight: confirm integrations-core is checked out ---
LOCKFILE="$INTEGRATIONS_CORE/.deps/resolved/linux-x86_64_${PYTHON_MAJ_MIN}.txt"
if [ ! -f "$LOCKFILE" ]; then
    log "ERROR: $LOCKFILE not found — did Stage 00 (00-checkout) complete successfully?"
    exit 1
fi

# --- Cleanup on failure ---
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed."
        log "       Re-run after fixing the error by deleting the sentinel:"
        log "       rm $SENTINEL"
    fi
}
trap cleanup EXIT

# ─── Version pins ──────────────────────────────────────────────────────────────
#
# Versions are read from the integrations-core lockfile so they stay in sync
# with the Linux omnibus pipeline automatically when the integrations-core
# commit is updated in release.json.
#
# The lockfile uses PEP 508 URL format:
#   package @ https://...registry.../package-VERSION-...-platform.whl#sha256=...
# We extract the version from the package name embedded in the wheel filename.

extract_version() {
    _pkg=$1
    # Match "pkgname @ https://...pkgname-VERSION-..." and extract VERSION
    grep -i "^${_pkg} @ " "$LOCKFILE" | head -1 \
        | sed "s/.*${_pkg}-\([0-9][0-9.]*\)-.*/\1/"
}

CFFI_VERSION=$(extract_version cffi)
PSUTIL_VERSION=$(extract_version psutil)
LXML_VERSION=$(extract_version lxml)
CRYPTOGRAPHY_VERSION=$(extract_version cryptography)

if [ -z "$CFFI_VERSION" ] || [ -z "$PSUTIL_VERSION" ] || [ -z "$LXML_VERSION" ] || [ -z "$CRYPTOGRAPHY_VERSION" ]; then
    log "ERROR: could not read one or more package versions from $LOCKFILE"
    log "  cffi=$CFFI_VERSION psutil=$PSUTIL_VERSION lxml=$LXML_VERSION cryptography=$CRYPTOGRAPHY_VERSION"
    log "  Check that $LOCKFILE contains cffi, psutil, lxml, cryptography entries."
    exit 1
fi

log "Package versions (from integrations-core lockfile):"
log "  cffi==$CFFI_VERSION"
log "  psutil==$PSUTIL_VERSION"
log "  lxml==$LXML_VERSION"
log "  cryptography==$CRYPTOGRAPHY_VERSION"

# ─── Step 1: cffi (C extension, bundled libffi) ────────────────────────────────
#
# cffi is required by cryptography (and many other packages). It builds against
# the libffi we bundled in Stage 1 via the standard CFLAGS/CPPFLAGS/LDFLAGS that
# point to $EMBEDDED_DESTDIR.
#
# AIX/GCC TLS issue (0509-187 / USE__THREAD):
#   cffi's setup.py calls config.try_compile('__thread int x;') which succeeds
#   on AIX/GCC (the syntax is valid) and then adds -DUSE__THREAD to the build.
#   -DUSE__THREAD makes _cffi_backend.c use __thread for TLS variables, which
#   AIX's dynamic linker rejects in shared libraries (error 0509-187: local-exec
#   model). -ftls-model=global-dynamic is not a fix because GCC on AIX generates
#   calls to __tls_get_addr (ELF TLS) which does not exist on XCOFF/AIX.
#
#   Fix: patch cffi's setup.py before install to skip USE__THREAD on AIX.
#   Without USE__THREAD, cffi uses pthread_key_t for TLS which works correctly.
#
# AIX/GCC __lwsync note: cffi's call_python.c uses __lwsync() which is an IBM
#   XLC intrinsic not available in GCC. We redefine it to GCC's
#   __sync_synchronize() which provides an equivalent full memory barrier.

log "Installing cffi==$CFFI_VERSION (C extension, bundled libffi)"
log "  Downloading cffi source and patching for AIX TLS compatibility"

# Download cffi source via pip (handles URL resolution, caching, etc.)
CFFI_SRCDIR="$TMPDIR/cffi-${CFFI_VERSION}-aix-src"
rm -rf "$CFFI_SRCDIR"
mkdir -p "$CFFI_SRCDIR"
$PIP download --no-deps --no-binary cffi "cffi==$CFFI_VERSION" -d "$CFFI_SRCDIR"
CFFI_TARBALL=$(find "$CFFI_SRCDIR" -name 'cffi-*.tar.gz' 2>/dev/null | head -1)
if [ -z "$CFFI_TARBALL" ]; then
    log "ERROR: could not download cffi source tarball"
    exit 1
fi
log "  cffi source: $CFFI_TARBALL"
CFFI_BUILDDIR="$TMPDIR/cffi-${CFFI_VERSION}-build"
rm -rf "$CFFI_BUILDDIR"
# Use Python tarfile module for extraction: AIX native tar rejects modern
# tar formats (pax headers) used by cffi's PyPI distribution.
$PYTHON_BIN -c "import tarfile, os; tarfile.open('$CFFI_TARBALL').extractall('$TMPDIR')"
mv "$TMPDIR/cffi-${CFFI_VERSION}" "$CFFI_BUILDDIR"

# Patch setup.py: add sys.platform != 'aix' check to ask_supports_thread()
# so that __thread is not used in the shared library (causes 0509-187 on AIX).
sed "s/sys.platform != 'win32' and\$/sys.platform != 'win32' and sys.platform != 'aix' and/" \
    "$CFFI_BUILDDIR/setup.py" > "$CFFI_BUILDDIR/setup.py.new"
mv "$CFFI_BUILDDIR/setup.py.new" "$CFFI_BUILDDIR/setup.py"
log "  setup.py patched: AIX check added to ask_supports_thread()"

CFLAGS="-maix64 -D__lwsync=__sync_synchronize" \
    $PIP install --no-cache-dir "$CFFI_BUILDDIR"
log "cffi==$CFFI_VERSION installed successfully"

# ─── Step 2: psutil (C extension) ─────────────────────────────────────────────
#
# psutil provides process and system information. --no-binary forces a source
# build so it is compiled against our bundled headers.

log "Installing psutil==$PSUTIL_VERSION (C extension)"
$PIP install --no-binary psutil "psutil==$PSUTIL_VERSION"
log "psutil==$PSUTIL_VERSION installed successfully"

# ─── Step 3: lxml (C extension, bundled libxml2/libxslt) ──────────────────────
#
# lxml is required by the ibm_was check. We pass explicit CFLAGS/LDFLAGS to
# ensure it links against our bundled libxml2 and libxslt from Stage 1.
# The libxml2 headers are under include/libxml2/ so we add that path explicitly.

log "Installing lxml==$LXML_VERSION (C extension, bundled libxml2/libxslt)"
CFLAGS="-maix64 -I$EMBEDDED_DESTDIR/include/libxml2" \
LDFLAGS="-maix64 -Wl,-brtl -L$EMBEDDED_DESTDIR/lib" \
    $PIP install --no-binary lxml "lxml==$LXML_VERSION"
log "lxml==$LXML_VERSION installed successfully"

# ─── Step 4: cryptography (Rust/PyO3 extension) ───────────────────────────────
#
# cryptography requires a Rust build. The wheel cache (keyed by version) avoids
# the ~15-minute Rust compilation on subsequent builds.
#
# AIX-specific Rust flags:
#   CARGO_PROFILE_RELEASE_STRIP=none  — IBM Rust 1.92 bug: stripping .info section
#                                       from proc-macro artifacts breaks rustc
#   CARGO_PROFILE_RELEASE_LTO=off     — LLVM fat LTO uses .ipa bitcode sections
#                                       that do not exist in AIX XCOFF format
#   CC=/opt/freeware/bin/gcc          — cc-rs defaults to IBM xlc which rejects GCC
#                                       flags like -fPIC, -ffunction-sections, -maix64
#   OPENSSL_DIR                       — tells the openssl-sys crate where our
#                                       bundled OpenSSL lives (staging path)
#   ARFLAGS unset                     — env.sh sets ARFLAGS="-X64 -cru" which
#                                       includes the ar operation code; cc-rs appends
#                                       its own "cq" operation, producing "ar -cru cq"
#                                       which AIX ar rejects (two operation codes).
#                                       OBJECT_MODE=64 already ensures 64-bit archives.
#   RUSTFLAGS="-C link-arg=-bbigtoc" — cryptography-rust has ~144KB TOC entries
#                                       which exceeds AIX ld's default 64KB TOC limit.
#                                       -bbigtoc removes this limit for AIX XCOFF.

log "Installing cryptography==$CRYPTOGRAPHY_VERSION (Rust/PyO3 extension)"
log "  Setting Rust environment: PATH=/opt/freeware/lib/RustSDK/$RUST_VERSION/bin:..."
export PATH=/opt/freeware/lib/RustSDK/"$RUST_VERSION"/bin:"$PATH"
export CARGO_HOME=/opt/cargo

# Check wheel cache (keyed by version so a version bump triggers a fresh build)
CRYPTO_CACHE_DIR="$WHEEL_CACHE/cryptography-$CRYPTOGRAPHY_VERSION"
mkdir -p "$CRYPTO_CACHE_DIR"
# Match only full-AIX-tag wheels (e.g. aix_7302_2419_64) not legacy aix_ppc64 renames.
# aix_*_* requires at least one underscore within the AIX portion, which aix_ppc64 lacks.
CACHED_CRYPTO=$(find "$CRYPTO_CACHE_DIR" -name "cryptography-${CRYPTOGRAPHY_VERSION}-*-aix_*_*.whl" 2>/dev/null | head -1)

if [ -n "$CACHED_CRYPTO" ]; then
    log "Found cached cryptography wheel: $CACHED_CRYPTO"
    # Install by direct path — bypasses PyPI lookup and works with any AIX tag
    # that matches the current interpreter's supported tags.
    $PIP install "$CACHED_CRYPTO"
    log "cryptography==$CRYPTOGRAPHY_VERSION installed from cache"
else
    log "No cached wheel found for cryptography==$CRYPTOGRAPHY_VERSION — building from source"
    # Pre-install maturin (cryptography build dep) into staging.
    # We use --no-build-isolation below so pip does not create an isolated venv,
    # allowing cryptography's build script to import cffi from our staging env
    # (which was compiled with AIX fixes) rather than from a fresh isolated build
    # that would fail with AIX TLS error 0509-187.
    # ARFLAGS="" prevents the cc-rs "ar -cru cq" double-operation conflict.
    log "Pre-installing maturin (required for --no-build-isolation cryptography build)"
    ARFLAGS="" \
        $PIP install "maturin>=1,<2"
    log "maturin installed"

    OPENSSL_DIR=$EMBEDDED_DESTDIR \
    CARGO_PROFILE_RELEASE_STRIP=none \
    CARGO_PROFILE_RELEASE_LTO=off \
    RUSTFLAGS="-C link-arg=-bbigtoc" \
    ARFLAGS="" \
        $PIP install --no-build-isolation --no-binary cryptography "cryptography==$CRYPTOGRAPHY_VERSION"
    log "cryptography==$CRYPTOGRAPHY_VERSION build complete"

    # Remove maturin (build-time tool; not needed at runtime)
    $PIP uninstall -y maturin 2>/dev/null || true

    # Cache the built wheel for subsequent builds.
    # Keep the original filename (with the full AIX platform tag from this system)
    # so that pip can match it by tag on cache restore.
    BUILT_WHEEL=$(find "${HOME}/.cache/pip" -name "cryptography-${CRYPTOGRAPHY_VERSION}-*.whl" 2>/dev/null | head -1)
    if [ -n "$BUILT_WHEEL" ]; then
        CACHE_NAME=$(basename "$BUILT_WHEEL")
        cp "$BUILT_WHEEL" "$CRYPTO_CACHE_DIR/$CACHE_NAME"
        log "Cached wheel to $CRYPTO_CACHE_DIR/$CACHE_NAME"
    else
        log "WARNING: could not locate built cryptography wheel — next build will rebuild from source"
    fi
fi

log "cryptography==$CRYPTOGRAPHY_VERSION installed successfully"

# ─── Step 5: pymqi (conditional — IBM MQ Client required) ─────────────────────
#
# pymqi is a C extension wrapping the IBM MQ C Client API. It is required by
# the ibm_mq and ibm_ace checks. The MQ Client shared libraries (libmqm.a,
# libmqmcs.a) are NOT bundled — they are a user-installed prerequisite on the
# target system. We skip gracefully if the build host does not have MQ headers.
#
# IBM MQ installs to /opt/mqm by default but some installations (e.g. the
# AIX installp packages) land at /usr/mqm instead. We check both.

MQ_HOME=""
if [ -d /opt/mqm/inc ]; then
    MQ_HOME=/opt/mqm
elif [ -d /usr/mqm/inc ]; then
    MQ_HOME=/usr/mqm
fi

if [ -z "$MQ_HOME" ]; then
    log "ERROR: IBM MQ Client not found at /opt/mqm or /usr/mqm."
    log "       Install IBM MQ Client 9.1+ (mqm.base.runtime + mqm.base.sdk + mqm.client.rte)"
    log "       from IBM Fix Central (9.x.x.x-IBM-MQC-AixPPC64) before running this stage."
    exit 1
fi

log "IBM MQ Client found at $MQ_HOME — building pymqi==$PYMQI_VERSION"
CFLAGS="$CFLAGS -I${MQ_HOME}/inc" \
LDFLAGS="$LDFLAGS -L${MQ_HOME}/lib64 -L${MQ_HOME}/lib -Wl,-brtl -lmqm" \
    $PIP install --no-binary pymqi "pymqi==$PYMQI_VERSION"
log "pymqi==$PYMQI_VERSION installed successfully"

# pymqi's CMQC.py was generated on Linux (x86_64) and hardcodes MQENC_NATIVE=0x222
# (little-endian). On AIX (big-endian) the correct value is 0x111 (MQENC_NORMAL).
# Without this patch MQMD.Encoding=0x222 on every PCF PUT, which tells the AIX
# command server the message is little-endian while the data is actually big-endian
# (Python struct uses native byte order) → MQRCCF_CFH_LENGTH_ERROR (reason 3002).
PYMQI_CMQC="$EMBEDDED_DESTDIR/lib/python${PYTHON_MAJ_MIN}/site-packages/pymqi/CMQC.py"
if [ ! -f "$PYMQI_CMQC" ]; then
    log "ERROR: pymqi CMQC.py not found at expected path: $PYMQI_CMQC"
    exit 1
fi
if grep -q 'MQENC_NATIVE = 0x00000111' "$PYMQI_CMQC" 2>/dev/null; then
    log "pymqi CMQC.py already patched — skipping (pip left the previous install in place)"
else
    patch "$PYMQI_CMQC" < "$SCRIPT_DIR/../patches/pymqi-CMQC-aix-endian.patch"
    log "pymqi CMQC.py patched: MQENC_NATIVE 0x222→0x111 (AIX big-endian)"
fi
find "$(dirname "$PYMQI_CMQC")/__pycache__" -name "CMQC.cpython-*.pyc" -delete 2>/dev/null || true

# ─── Step 6: pyodbc (conditional — unixODBC headers required) ─────────────────
#
# pyodbc is a C++ extension wrapping unixODBC. It is required by the ibm_i
# check. The IBM i Access ODBC driver is a separate user-installed prerequisite
# on the target system. We skip gracefully if the build host lacks sql.h.

if [ -f /opt/freeware/include/sql.h ] || [ -f /usr/include/sql.h ]; then
    log "unixODBC headers found — building pyodbc"
    CFLAGS="$CFLAGS -I/opt/freeware/include" \
    LDFLAGS="$LDFLAGS -L/opt/freeware/lib -lodbc" \
        $PIP install --no-binary pyodbc "pyodbc==$PYODBC_VERSION"
    log "pyodbc installed successfully"
else
    log "WARNING: unixODBC headers not found — skipping pyodbc (ibm_i check will not work)"
    log "         Install unixODBC development headers (yum install unixODBC unixODBC-devel) and re-run this stage."
fi

# ibm_db (ibm_db2 check) is intentionally not built here. Unlike other C
# extensions, ibm_db auto-downloads the IBM DB2 CLI driver at pip install time
# and does not require a pre-installed DB2 client on the build host. This
# matches Linux behaviour — customers install it via the embedded pip after
# deployment: sudo -Hu dd-agent pip install ibm_db==<version>

# --- Mark complete ---
mkdir -p "$(dirname "$SENTINEL")"
touch "$SENTINEL"
log "=== $STAGE_NAME complete ==="

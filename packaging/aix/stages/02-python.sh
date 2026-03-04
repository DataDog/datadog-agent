#!/bin/sh
set -eu

# Source shared environment (defines STAGING, EMBEDDED, EMBEDDED_DESTDIR, etc.)
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/../lib/env.sh"

STAGE_NAME="02-python"
SENTINEL="$BUILD_DIR/.done/$STAGE_NAME"
LOG="$BUILD_DIR/logs/$STAGE_NAME.log"

PYTHON_VERSION="3.13.12"
PYTHON_TARBALL="Python-${PYTHON_VERSION}.tgz"
PYTHON_URL="https://www.python.org/ftp/python/${PYTHON_VERSION}/${PYTHON_TARBALL}"
PYTHON_SRC="$BUILD_DIR/build/Python-${PYTHON_VERSION}"

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
: "${CXX:?CXX must be set}"
: "${CFLAGS:?CFLAGS must be set}"
: "${CPPFLAGS:?CPPFLAGS must be set}"
: "${LDFLAGS:?LDFLAGS must be set}"
: "${ARFLAGS:?ARFLAGS must be set}"
: "${NM:?NM must be set}"

# Confirm Stage 1 outputs are present before spending ~20 minutes compiling Python
if [ ! -f "$EMBEDDED_DESTDIR/lib/libssl.so" ] && [ ! -f "$EMBEDDED_DESTDIR/lib/libssl.a" ]; then
    log "ERROR: OpenSSL not found at $EMBEDDED_DESTDIR/lib/libssl.so — did Stage 01 (01-native-libs) complete successfully?"
    exit 1
fi

# --- Cleanup on failure ---
cleanup() {
    if [ $? -ne 0 ]; then
        log "ERROR: $STAGE_NAME failed. Removing partial outputs."
        # Remove the partial Python build tree
        rm -rf "$PYTHON_SRC"
        # Remove partial install (python3.13 binary is the last step indicator)
        if [ -f "$EMBEDDED_DESTDIR/bin/python3.13" ]; then
            log "Removing partial $EMBEDDED_DESTDIR/bin/python3.13"
            rm -f "$EMBEDDED_DESTDIR/bin/python3.13"
        fi
        # Do NOT remove $STAGING entirely — other stages may have already run.
    fi
}
trap cleanup EXIT

# --- Prepare directories ---
mkdir -p "$BUILD_DIR/sources"
mkdir -p "$BUILD_DIR/build"

# ─── Step 1: Download tarball ─────────────────────────────────────────────────

TARBALL="$BUILD_DIR/sources/$PYTHON_TARBALL"
if [ -f "$TARBALL" ]; then
    log "Tarball already present: $TARBALL — skipping download."
else
    log "Downloading Python ${PYTHON_VERSION} from $PYTHON_URL"
    curl -fSL -o "$TARBALL" "$PYTHON_URL"
    log "Download complete: $TARBALL"
fi

# ─── Step 2: Extract source ───────────────────────────────────────────────────

if [ -d "$PYTHON_SRC" ]; then
    log "Build directory already exists: $PYTHON_SRC — removing for clean build."
    rm -rf "$PYTHON_SRC"
fi

log "Extracting $TARBALL to $BUILD_DIR/build/"
# AIX system tar does not support decompression flags; decompress explicitly.
gunzip -c "$TARBALL" | tar xf - -C "$BUILD_DIR/build"
log "Extraction complete."

# ─── Step 3: Apply AIX patches ───────────────────────────────────────────────
#
# NOTE: These patches were identified from datadog-unix-agent's AIX patches for
# Python 3.8 (omnibus/config/patches/python3/).  For Python 3.13, patch offsets
# will have shifted and some may no longer apply or may not be needed at all.
# We use sed-based substitutions rather than patch(1) files to avoid offset
# sensitivity.  If a patch no longer applies (pattern not found), we log a
# warning rather than failing — it may mean the upstream fixed the issue.
#
# IMPORTANT: Validate all patches by doing a trial build of Python 3.13.12 on
# AIX before finalising this script.  Add or remove sed substitutions based on
# actual configure/compile errors encountered.

log "Applying AIX-specific patches to Python ${PYTHON_VERSION} source"

cd "$PYTHON_SRC"

# Patch: remove libintl dependency from Modules/getpath.c
# libintl (GNU message catalog library) is not present on stock AIX.
# The unix-agent carries this patch for Python 3.8; the include may or may
# not still be present in 3.13 — apply defensively.
if grep -q 'libintl\.h' Modules/getpath.c 2>/dev/null; then
    sed 's/#include <libintl\.h>/\/* libintl not available on AIX *\//g' \
        Modules/getpath.c > Modules/getpath.c.tmp
    mv Modules/getpath.c.tmp Modules/getpath.c
    log "Applied: removed '#include <libintl.h>' from Modules/getpath.c"
else
    log "INFO: libintl.h not found in Modules/getpath.c — patch not needed (OK for 3.13)."
fi

# Patch: remove libintl dependency from Python/gettext.c (if present in 3.13)
# Some Python versions include libintl in additional translation-related files.
if grep -q 'libintl\.h' Python/gettext.c 2>/dev/null; then
    sed 's/#include <libintl\.h>/\/* libintl not available on AIX *\//g' \
        Python/gettext.c > Python/gettext.c.tmp
    mv Python/gettext.c.tmp Python/gettext.c
    log "Applied: removed '#include <libintl.h>' from Python/gettext.c"
else
    log "INFO: libintl.h not found in Python/gettext.c — patch not needed."
fi

# Patch: Disable __thread TLS in libpython3.13.so on AIX
#
# On AIX, GCC's __thread (and C11's _Thread_local) storage class uses the
# XCOFF "local-exec" TLS model.  This model is ONLY valid for the main
# executable.  When Python uses __thread in libpython3.13.so (a startup-linked
# shared library), AIX's loader rejects the program with:
#   0509-187 The local-exec model was used for thread-local storage,
#            but the module is not the main program.
#
# Fix: (1) Disable HAVE_THREAD_LOCAL on AIX in Include/pyport.h so Python
#      falls back to its PyThread_tss_*() API.
#      (2) Implement the PyThread_tss_t fallback in Python/pystate.c
#          (the upstream code has #error stubs where this fallback should go).
#
# A Python 3.12 interpreter (from the AIX Toolbox) is used to apply these
# multi-line substitutions because sed on AIX doesn't support \n in replacements.

if grep -q 'HAVE_THREAD_LOCAL' Include/pyport.h 2>/dev/null; then

if python3.12 << 'PATCH_EOF'
import sys

# ─── Patch 1: Include/pyport.h ─────────────────────────────────────────────
# On AIX, both _Thread_local (C11) and __thread (GCC extension) use the
# XCOFF "local-exec" TLS model, which AIX's loader rejects in shared libs.
# Insert an _AIX guard BEFORE all TLS model checks so HAVE_THREAD_LOCAL
# is undefined on AIX.  Python then falls back to PyThread_tss_*().
#
# Original block:                       Patched block:
#   #define HAVE_THREAD_LOCAL 1           #define HAVE_THREAD_LOCAL 1
#   #ifdef thread_local                   #if defined(_AIX)
#     ...                                    #undef HAVE_THREAD_LOCAL
#   #elif __STDC_VERSION__ >= 201112L     #elif defined(thread_local)
#     #define _Py_thread_local _Thread_    ...  (rest unchanged)
#   ...
with open('Include/pyport.h', 'r') as f:
    src = f.read()

OLD = (
    '#    define HAVE_THREAD_LOCAL 1\n'
    '#    ifdef thread_local\n'
    '#      define _Py_thread_local thread_local\n'
    '#    elif __STDC_VERSION__ >= 201112L && !defined(__STDC_NO_THREADS__)\n'
    '#      define _Py_thread_local _Thread_local\n'
    '#    elif defined(_MSC_VER)  /* AKA NT_THREADS */\n'
    '#      define _Py_thread_local __declspec(thread)\n'
    '#    elif defined(__GNUC__)  /* includes clang */'
)
NEW = (
    '#    define HAVE_THREAD_LOCAL 1\n'
    '#    if defined(_AIX)\n'
    '       /* AIX: _Thread_local and __thread use local-exec TLS model which\n'
    '          is incompatible with shared library loading.  Fall back to the\n'
    '          PyThread_tss_*() API by undefining HAVE_THREAD_LOCAL. */\n'
    '#      undef HAVE_THREAD_LOCAL\n'
    '#    elif defined(thread_local)\n'
    '#      define _Py_thread_local thread_local\n'
    '#    elif __STDC_VERSION__ >= 201112L && !defined(__STDC_NO_THREADS__)\n'
    '#      define _Py_thread_local _Thread_local\n'
    '#    elif defined(_MSC_VER)  /* AKA NT_THREADS */\n'
    '#      define _Py_thread_local __declspec(thread)\n'
    '#    elif defined(__GNUC__)  /* includes clang */'
)
if OLD in src:
    src = src.replace(OLD, NEW, 1)
    print('pyport.h: inserted _AIX guard before TLS model selection')
else:
    print('pyport.h: HAVE_THREAD_LOCAL block not found — skipping', file=sys.stderr)

with open('Include/pyport.h', 'w') as f:
    f.write(src)

# ─── Patch 2: Python/pystate.c ─────────────────────────────────────────────
# Replace the three #error stubs in current_fast_{get,set,clear} with a
# working Py_tss_t-based implementation.
with open('Python/pystate.c', 'r') as f:
    src = f.read()

# 2a: Add Py_tss_t declaration after the #ifdef HAVE_THREAD_LOCAL block
OLD_DECL = (
    '#ifdef HAVE_THREAD_LOCAL\n'
    '_Py_thread_local PyThreadState *_Py_tss_tstate = NULL;\n'
    '#endif'
)
NEW_DECL = (
    '#ifdef HAVE_THREAD_LOCAL\n'
    '_Py_thread_local PyThreadState *_Py_tss_tstate = NULL;\n'
    '#else\n'
    '/* AIX: __thread uses local-exec TLS incompatible with shared libraries.\n'
    '   Use a POSIX thread-specific key as fallback. */\n'
    'static Py_tss_t _Py_tss_current = Py_tss_NEEDS_INIT;\n'
    '#endif'
)

# 2b: current_fast_get fallback
OLD_GET = (
    '    // XXX Fall back to the PyThread_tss_*() API.\n'
    '#  error "no supported thread-local variable storage classifier"\n'
    '#endif\n'
    '}\n'
    '\n'
    'static inline void\n'
    'current_fast_set'
)
NEW_GET = (
    '    /* AIX fallback: use Py_tss_t */\n'
    '    if (!PyThread_tss_is_created(&_Py_tss_current)) {\n'
    '        return NULL;\n'
    '    }\n'
    '    return (PyThreadState *)PyThread_tss_get(&_Py_tss_current);\n'
    '#endif\n'
    '}\n'
    '\n'
    'static inline void\n'
    'current_fast_set'
)

# 2c: current_fast_set fallback
OLD_SET = (
    '    // XXX Fall back to the PyThread_tss_*() API.\n'
    '#  error "no supported thread-local variable storage classifier"\n'
    '#endif\n'
    '}\n'
    '\n'
    'static inline void\n'
    'current_fast_clear'
)
NEW_SET = (
    '    /* AIX fallback: use Py_tss_t */\n'
    '    if (!PyThread_tss_is_created(&_Py_tss_current)) {\n'
    '        (void)PyThread_tss_create(&_Py_tss_current);\n'
    '    }\n'
    '    (void)PyThread_tss_set(&_Py_tss_current, tstate);\n'
    '#endif\n'
    '}\n'
    '\n'
    'static inline void\n'
    'current_fast_clear'
)

# 2d: current_fast_clear fallback
OLD_CLR = (
    '    // XXX Fall back to the PyThread_tss_*() API.\n'
    '#  error "no supported thread-local variable storage classifier"\n'
    '#endif\n'
    '}\n'
    '\n'
    '#define tstate_verify_not_active'
)
NEW_CLR = (
    '    /* AIX fallback: use Py_tss_t */\n'
    '    if (PyThread_tss_is_created(&_Py_tss_current)) {\n'
    '        (void)PyThread_tss_set(&_Py_tss_current, NULL);\n'
    '    }\n'
    '#endif\n'
    '}\n'
    '\n'
    '#define tstate_verify_not_active'
)

patches = [
    ('pystate.c decl', OLD_DECL, NEW_DECL),
    ('pystate.c current_fast_get', OLD_GET, NEW_GET),
    ('pystate.c current_fast_set', OLD_SET, NEW_SET),
    ('pystate.c current_fast_clear', OLD_CLR, NEW_CLR),
]
for name, old, new in patches:
    if old in src:
        src = src.replace(old, new, 1)
        print(f'{name}: patched')
    else:
        print(f'{name}: pattern not found — skipping', file=sys.stderr)

with open('Python/pystate.c', 'w') as f:
    f.write(src)
PATCH_EOF
then
    log "Applied: AIX TLS __thread patches to pyport.h and pystate.c"
else
    log "ERROR: Python TLS patch script failed"
    exit 1
fi
else
    log "INFO: pyport.h __GNUC__ pattern not found — TLS patch not needed (already patched?)."
fi

log "AIX patching complete."

# ─── Step 4: Configure ───────────────────────────────────────────────────────
#
# Key AIX-specific flags (inherited from env.sh):
#   -maix64          : 64-bit PowerPC code model
#   -Wl,-brtl        : enable runtime linking (required for dlopen on AIX)
#   ARFLAGS=-X64     : 64-bit archive member handling
#   NM="/usr/bin/nm -X64" : 64-bit symbol table
#
# --with-openssl=$EMBEDDED_DESTDIR : points to staging path (where OpenSSL
#   headers and libs ARE during the build, not $EMBEDDED which is the final path)
# --with-dbmliborder=gdbm : use gdbm built in Stage 1
# --without-ensurepip : we bootstrap pip manually below (step 7)

log "Configuring Python ${PYTHON_VERSION} (--prefix=$EMBEDDED)"
log "  (Note: configure can take several minutes on POWER8)"

cd "$PYTHON_SRC"
./configure \
    --prefix="$EMBEDDED" \
    --enable-shared \
    --with-openssl="$EMBEDDED_DESTDIR" \
    --with-dbmliborder=gdbm \
    --without-ensurepip \
    --without-mimalloc \
    CC="$CC" \
    CXX="$CXX" \
    CFLAGS="$CFLAGS -I$EMBEDDED_DESTDIR/include" \
    CPPFLAGS="$CPPFLAGS" \
    LDFLAGS="$LDFLAGS" \
    ARFLAGS="$ARFLAGS" \
    NM="$NM"

log "Configure complete."

# ─── Step 5: Build ───────────────────────────────────────────────────────────
#
# Building CPython from source takes approximately 20 minutes on POWER8.
# This is expected — do not interrupt the build.

log "Building Python ${PYTHON_VERSION} with make -j$NPROC"
log "  (This step takes approximately 20 minutes on POWER8 — please be patient.)"

cd "$PYTHON_SRC"
make -j"$NPROC"

log "Build complete."

# ─── Step 6: Install to staging ──────────────────────────────────────────────
#
# DESTDIR=$STAGING causes files to land in $STAGING/<prefix>, i.e.
# $STAGING/opt/datadog-agent/embedded/... = $EMBEDDED_DESTDIR/...
# The $EMBEDDED prefix is baked into all binaries; at runtime on the installed
# system the files are at $EMBEDDED and all paths are correct.

log "Installing Python ${PYTHON_VERSION} to staging tree (DESTDIR=$STAGING)"

cd "$PYTHON_SRC"
make install DESTDIR="$STAGING"

log "Install to staging complete."
log "Python executable: $EMBEDDED_DESTDIR/bin/python3.13"

# Verify the executable landed where expected
if [ ! -f "$EMBEDDED_DESTDIR/bin/python3.13" ]; then
    log "ERROR: $EMBEDDED_DESTDIR/bin/python3.13 not found after make install — install failed."
    exit 1
fi

# ─── Step 7: Bootstrap pip ───────────────────────────────────────────────────
#
# We configured with --without-ensurepip, so we must bootstrap pip manually.
# We invoke the STAGING executable ($EMBEDDED_DESTDIR/bin/python3.13).
# Python discovers sys.prefix from its executable path at runtime, so it finds
# its stdlib under $EMBEDDED_DESTDIR/lib/python3.13/ and installs packages into
# the staging tree — not into $EMBEDDED (which does not exist yet on this host).
# At runtime on the user's system the files are at $EMBEDDED, which is correct.

log "Bootstrapping pip using staging Python executable"
"$EMBEDDED_DESTDIR/bin/python3.13" -m ensurepip
log "ensurepip complete."

log "Upgrading pip to 24.0, setuptools to 75.1.0, and installing wheel"
"$EMBEDDED_DESTDIR/bin/pip3.13" install --upgrade "pip==24.0" "setuptools==75.1.0" "wheel"
log "pip bootstrap complete."

# ─── Step 7b: Create AIX .a archive wrapper for libpython3.13.so ─────────────
#
# On AIX, Go's CGO requires shared libraries to be wrapped in .a archives.
# Without this, the Go cgo tool cannot generate correct //go:cgo_import_dynamic
# directives (which must reference "libpython3.13.a/libpython3.13.so").
# This archive is created in-place next to the .so file.

log "Creating libpython3.13.a archive wrapper (required for AIX CGO)"
# On AIX, Go's compiler requires archive member names to end in ".o" or contain ".so."
# Convention: name the 64-bit shared module member "shr_64.o" inside the .a archive.
cd "$EMBEDDED_DESTDIR/lib"
cp libpython3.13.so shr_64.o
ar -X64 -r libpython3.13.a shr_64.o
rm -f shr_64.o
log "Created: $EMBEDDED_DESTDIR/lib/libpython3.13.a (member: shr_64.o)"

# ─── Step 7c: Create runtime-path symlink (needed to build C extensions) ─────
#
# Python's sys.prefix is baked-in as $EMBEDDED (/opt/datadog-agent/embedded).
# When building C extensions (cffi, psutil, etc.) in later stages, Python looks
# for ld_so_aix and config files at that prefix.  We create a symlink from the
# runtime path to the staging path so Python can find these files during the build.
# The 10-assemble stage will remove this symlink and replace it with the real files.

if [ -L "$EMBEDDED" ] && [ "$(readlink "$EMBEDDED")" = "$EMBEDDED_DESTDIR" ]; then
    log "INFO: $EMBEDDED symlink already correct."
else
    if [ -e "$EMBEDDED" ] && [ ! -L "$EMBEDDED" ]; then
        log "INFO: $EMBEDDED is a real path — removing to create symlink"
        rm -rf "$EMBEDDED"
    fi
    mkdir -p "$(dirname "$EMBEDDED")"
    ln -sf "$EMBEDDED_DESTDIR" "$EMBEDDED"
    log "Created runtime-path symlink: $EMBEDDED -> $EMBEDDED_DESTDIR"
fi

# ─── Step 8: Convenience symlinks ────────────────────────────────────────────

log "Creating convenience symlinks python3 -> python3.13 and pip3 -> pip3.13"
ln -sf python3.13 "$EMBEDDED_DESTDIR/bin/python3"
ln -sf pip3.13    "$EMBEDDED_DESTDIR/bin/pip3"
log "Symlinks created."

# ─── Step 9: Remove test directories to save space ───────────────────────────

log "Removing Python test directories to reduce package size"
rm -rf "$EMBEDDED_DESTDIR/lib/python3.13/test"
rm -rf "$EMBEDDED_DESTDIR/lib/python3.13/unittest/test"
log "Test directories removed."

# --- Mark complete ---
mkdir -p "$(dirname "$SENTINEL")"
touch "$SENTINEL"
log "=== $STAGE_NAME complete ==="

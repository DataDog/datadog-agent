# shellcheck shell=sh
# lib/env.sh — shared environment sourced by every stage script
#
# Usage: . "$SCRIPT_DIR/../lib/env.sh"   (from a stages/NN-name.sh script)
#        . "$SCRIPT_DIR/lib/env.sh"      (from build.sh)
#
# This file is sourced, never executed directly. Callers control set -e/set -u.
# No validation of required variables is done here; each script validates its
# own inputs after sourcing this file.

# ── Python version ────────────────────────────────────────────────────────────
PYTHON_VERSION="3.13.12"
PYTHON_MAJ_MIN="${PYTHON_VERSION%.*}"   # e.g. 3.13
export PYTHON_VERSION PYTHON_MAJ_MIN

# ── Rust SDK version ──────────────────────────────────────────────────────────
# IBM Rust SDK for AIX. The SDK is installed at /opt/freeware/lib/RustSDK/<ver>/bin.
# All stage scripts reference $RUST_VERSION; update only this one line to upgrade.
RUST_VERSION="1.92"
export RUST_VERSION

# ── Build tree layout ─────────────────────────────────────────────────────────

BUILD_DIR=/opt/dd-build
STAGING=$BUILD_DIR/staging

# ── Agent source tree ─────────────────────────────────────────────────────────
# AGENT_SRC is the directory containing the checked-out agent source code. It
# is always derived here by walking up from the caller's script directory
# ($SCRIPT_DIR, set by every stage and by build.sh before sourcing this file)
# until a parent containing .git is found. Failure to find .git anywhere up the tree is a fatal error.
: "${SCRIPT_DIR:?env.sh requires SCRIPT_DIR to be set by the caller before sourcing}"
_dir=$SCRIPT_DIR
while [ "$_dir" != "/" ] && [ ! -e "$_dir/.git" ]; do
    _dir=$(dirname "$_dir")
done
if [ ! -e "$_dir/.git" ]; then
    printf 'ERROR: env.sh could not find a .git ancestor of %s\n' "$SCRIPT_DIR" >&2
    printf '       Run the build from a checkout of the datadog-agent source repo.\n' >&2
    exit 1
fi
AGENT_SRC=$_dir
unset _dir
export AGENT_SRC

# DESTDIR approach (critical — read before modifying):
#   EMBEDDED     = final install path baked into all binaries at configure time
#                  (sys.prefix, _sysconfigdata, XCOFF loader sections)
#   EMBEDDED_DESTDIR = where files actually land during the build (staging tree)
#
# All ./configure calls use --prefix=$EMBEDDED.
# All make install calls use DESTDIR=$STAGING.
# All compiler -I/-L flags point to $EMBEDDED_DESTDIR (where files are during build).
# Never pass $EMBEDDED_DESTDIR to --prefix; never pass $EMBEDDED to -L or -I.
EMBEDDED=/opt/datadog-agent/embedded
EMBEDDED_DESTDIR=$STAGING/opt/datadog-agent/embedded

# Bootstrap $EMBEDDED as a symlink to $EMBEDDED_DESTDIR so that Python
# binaries (pip, ensurepip, etc.) whose shebangs were baked at configure
# time as `#!/opt/datadog-agent/embedded/bin/python3.13` resolve during the build.
if [ ! -L "$EMBEDDED" ] || [ "$(readlink "$EMBEDDED" 2>/dev/null)" != "$EMBEDDED_DESTDIR" ]; then
    if [ -e "$EMBEDDED" ] && [ ! -L "$EMBEDDED" ]; then
        rm -rf "$EMBEDDED"
    fi
    mkdir -p "$(dirname "$EMBEDDED")"
    ln -sf "$EMBEDDED_DESTDIR" "$EMBEDDED"
fi

INTEGRATIONS_CORE=$BUILD_DIR/integrations-core
WHEEL_CACHE=$BUILD_DIR/wheel-cache
LIB_CACHE=$BUILD_DIR/lib-cache

# Number of available CPUs — nproc does not exist on AIX; lsdev is in /usr/sbin
NPROC=$(/usr/sbin/lsdev -Cc processor | wc -l | tr -d ' ')

export BUILD_DIR STAGING EMBEDDED EMBEDDED_DESTDIR INTEGRATIONS_CORE WHEEL_CACHE LIB_CACHE NPROC

# ── Agent version variables ───────────────────────────────────────────────────
# AGENT_VERSION, AGENT_BUILD, and AGENT_VRMF must be set before sourcing this
# file (build.sh sets them; standalone stage invocations must set them too).
# Use ${VAR:-} (no-fail) so env.sh can be sourced under set -u. The individual
# stage scripts validate via : "${AGENT_VERSION:?...}" after sourcing this file.

export AGENT_VERSION AGENT_BUILD AGENT_VRMF

# ── Toolchain ─────────────────────────────────────────────────────────────────

# GCC 8 is required for AIX 7.2 TL2 compatibility.
# GCC 8's libstdc++ does not reference strftime_l (added to AIX libc only at
# TL3+); GCC 10/13 do.  Code compiled by GCC 8 also calls ostringstream
# constructors that GCC 8's libstdc++ actually exports, so the resulting
# binaries run on AIX 7.2 without any compatibility stubs.
# Install on the build host with: yum install -y gcc8 gcc8-c++
if [ ! -x /opt/freeware/bin/gcc-8 ]; then
    printf 'ERROR: gcc-8 not found. Install it with: yum install -y gcc8 gcc8-c++\n' >&2
    exit 1
fi
CC=/opt/freeware/bin/gcc-8
CXX=/opt/freeware/bin/g++-8
NM="/usr/bin/nm -X64"
ARFLAGS="-X64 -cru"
OBJECT_MODE=64
# gcc-8 checks AIX_OBJECT_MODE (not OBJECT_MODE) for startup-file selection.
# Without it, gcc-8 passes 32-bit /lib/crt0.o to ld, which ld running in
# 64-bit mode rejects. AIX_OBJECT_MODE=64 makes gcc-8 use /lib/crt0_64.o.
AIX_OBJECT_MODE=64

export CC CXX NM ARFLAGS OBJECT_MODE AIX_OBJECT_MODE

# ── Compiler/linker flags ─────────────────────────────────────────────────────
# -I and -L always reference $EMBEDDED_DESTDIR (staging), not $EMBEDDED (final path).

CFLAGS="-maix64"
CXXFLAGS="-maix64"
# -Wl,-bbigtoc: remove the 64KB TOC limit (required for large libs like OpenSSL and Python)
# -Wl,-brtl:    enable runtime linking for dlopen support
LDFLAGS="-maix64 -Wl,-brtl -Wl,-bbigtoc -L$EMBEDDED_DESTDIR/lib"
CPPFLAGS="-I$EMBEDDED_DESTDIR/include"

export CFLAGS CXXFLAGS LDFLAGS CPPFLAGS

# ── PATH and Go toolchain ─────────────────────────────────────────────────────

PATH=/opt/go/bin:/opt/freeware/bin:/usr/sbin:/usr/bin:/bin:$PATH
GOPATH=/home/gopath
GOROOT=/opt/go
CGO_ENABLED=1
CGO_CFLAGS="-I/opt/freeware/include"
CGO_LDFLAGS="-L/opt/freeware/lib -L/opt/freeware/lib64"
GOPROXY=https://proxy.golang.org,direct
# Use the locally installed Go toolchain — prevents auto-download of a newer
# toolchain version (go.mod may require a newer patch than is installed).
# Auto-download spawns extra processes and consumes significant memory on AIX.
GOTOOLCHAIN=local
# On hosts with less than 6 GiB of RAM, restrict Go compilation to one package
# at a time and cap the heap to prevent swap thrash. Each compile process can
# use 3-4 GiB; without -p=1 multiple would compete for the same RAM.
# On larger hosts, the default parallelism is fine.
_mem_kb=$(lsattr -El sys0 -a realmem 2>/dev/null | awk '{print $2}')
if [ -n "$_mem_kb" ] && [ "$_mem_kb" -lt 6291456 ]; then
    GOFLAGS="-p=1"
    GOMEMLIMIT=2GiB
    export GOFLAGS GOMEMLIMIT
fi
unset _mem_kb
# Redirect the Go build cache off /tmp (which is only 12 GB) to the larger
# build volume so that large packages like datadogV2 don't exhaust /tmp.
GOCACHE=/opt/dd-build/gocache
mkdir -p "$GOCACHE"

export PATH GOPATH GOROOT CGO_ENABLED CGO_CFLAGS CGO_LDFLAGS GOPROXY GOTOOLCHAIN GOCACHE

# ── Utility functions ─────────────────────────────────────────────────────────

# log MESSAGE ...
#   Print a timestamped log line to stdout.
log() {
    printf '[%s] %s\n' "$(date '+%Y-%m-%dT%H:%M:%S')" "$*"
}

# sentinel_done STAGE_NAME
#   Returns 0 (true) if the stage sentinel file exists, 1 (false) otherwise.
sentinel_done() {
    [ -f "$BUILD_DIR/.done/$1" ]
}

# sentinel_mark STAGE_NAME
#   Create the sentinel file that marks STAGE_NAME as complete.
sentinel_mark() {
    mkdir -p "$BUILD_DIR/.done"
    touch "$BUILD_DIR/.done/$1"
}

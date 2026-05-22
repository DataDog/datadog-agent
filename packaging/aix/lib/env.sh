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

INTEGRATIONS_CORE=$BUILD_DIR/integrations-core
WHEEL_CACHE=$BUILD_DIR/wheel-cache
LIB_CACHE=$BUILD_DIR/lib-cache

# Number of available CPUs — nproc does not exist on AIX; lsdev is in /usr/sbin
NPROC=$(/usr/sbin/lsdev -Cc processor | wc -l | tr -d ' ')

export BUILD_DIR STAGING EMBEDDED EMBEDDED_DESTDIR INTEGRATIONS_CORE WHEEL_CACHE LIB_CACHE NPROC

# ── Agent version variables ───────────────────────────────────────────────────
# AGENT_BRANCH, AGENT_VERSION, and AGENT_BUILD are required inputs.
# They must be set in the caller's environment before sourcing this file.
# AGENT_VRMF is derived here; it is the four-component installp version string.

# Use ${VAR:-} (no-fail) so env.sh can be sourced under set -u before the caller
# validates AGENT_VERSION/AGENT_BUILD. The individual stage scripts call
#   : "${AGENT_VERSION:?AGENT_VERSION must be set}"
# after sourcing this file; that is where the empty-variable error is reported.
# VRMF must be four pure integers (X.Y.Z.N) — strip any .gSHA suffix from AGENT_BUILD.
AGENT_VRMF=$(printf '%s' "${AGENT_VERSION:-}" | sed 's/\([0-9]*\.[0-9]*\.[0-9]*\).*/\1/').$(printf '%s' "${AGENT_BUILD:-}" | sed 's/\..*//')

export AGENT_VERSION AGENT_BUILD AGENT_BRANCH AGENT_VRMF

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

# GNU make is required — AIX's native /usr/bin/make cannot handle Python's
# Makefile (it rejects comment lines as "shell command not associated with a
# dependency"). GNU make installs to /opt/freeware/bin/make via the IBM
# Toolbox RPM and is selected automatically because /opt/freeware/bin is first
# in PATH below.
# Install on the build host with (if yum works):
#   yum install -y make
# Or directly via RPM:
#   curl -fSL -o /tmp/make.rpm https://public.dhe.ibm.com/aix/freeSoftware/aixtoolbox/RPMS/ppc/make/make-4.4.1-1.aix7.1.ppc.rpm
#   rpm -Uvh /tmp/make.rpm
if [ ! -x /opt/freeware/bin/make ]; then
    printf 'ERROR: GNU make not found at /opt/freeware/bin/make.\n' >&2
    printf '       Install with: rpm -Uvh https://public.dhe.ibm.com/aix/freeSoftware/aixtoolbox/RPMS/ppc/make/make-4.4.1-1.aix7.1.ppc.rpm\n' >&2
    exit 1
fi
OBJECT_MODE=64
# AIX_OBJECT_MODE controls startup-file selection in GCC (vs OBJECT_MODE which
# controls ld). However, Go's linker probes the external C compiler with a bare
# invocation (no -maix64) while OBJECT_MODE=64 already puts ld in 64-bit mode.
# This mismatch causes ld to reject the 32-bit /lib/crt0.o that GCC links.
# Fix: wrap gcc-8/g++-8 so every invocation — including Go's probe — gets
# -maix64 and therefore selects the 64-bit /lib/crt0_64.o startup file.
_GCC_WRAP_DIR="$BUILD_DIR/gcc-wrap"
mkdir -p "$_GCC_WRAP_DIR"
printf '#!/bin/sh\nexec /opt/freeware/bin/gcc-8 -maix64 "$@"\n' > "$_GCC_WRAP_DIR/gcc-8"
printf '#!/bin/sh\nexec /opt/freeware/bin/g++-8 -maix64 "$@"\n' > "$_GCC_WRAP_DIR/g++-8"
chmod +x "$_GCC_WRAP_DIR/gcc-8" "$_GCC_WRAP_DIR/g++-8"
AIX_OBJECT_MODE=64
CC="$_GCC_WRAP_DIR/gcc-8"
CXX="$_GCC_WRAP_DIR/g++-8"
NM="/usr/bin/nm -X64"
ARFLAGS="-X64 -cru"

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

# ── TLS / CA bundle ──────────────────────────────────────────────────────────
# AIX's system CA store is outdated. Point curl and git at the freeware CA
# bundle (installed with openssl from AIX Toolbox) so downloads work without
# --insecure.
CURL_CA_BUNDLE=/opt/freeware/etc/ssl/certs/ca-bundle.crt
GIT_SSL_CAINFO=/opt/freeware/etc/ssl/certs/ca-bundle.crt
export CURL_CA_BUNDLE GIT_SSL_CAINFO

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
# -p=1: one package compiled at a time; prevents multiple 3-4 GB Go compiler
# processes from competing for RAM on the 4 GB AIX build host and causing
# thrashing or OOM kills.
GOFLAGS="-p=1"
# Redirect the Go build cache and temp directory off /tmp (only ~3 GB on AIX
# 7.2) to the larger build volume. /tmp fills quickly when compiling large
# packages like datadogV2 or snmptraps; GOCACHE handles the incremental cache
# and GOTMPDIR handles the per-build scratch space (go-buildXXX dirs).
GOCACHE=/opt/dd-build/gocache
GOTMPDIR=/opt/dd-build/gotmp
mkdir -p "$GOCACHE" "$GOTMPDIR"

export PATH GOPATH GOROOT CGO_ENABLED CGO_CFLAGS CGO_LDFLAGS GOPROXY GOTOOLCHAIN GOFLAGS GOCACHE GOTMPDIR

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

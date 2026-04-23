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

CC=/opt/freeware/bin/gcc
CXX=/opt/freeware/bin/g++
NM="/usr/bin/nm -X64"
ARFLAGS="-X64 -cru"
OBJECT_MODE=64

export CC CXX NM ARFLAGS OBJECT_MODE

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

export PATH GOPATH GOROOT CGO_ENABLED CGO_CFLAGS CGO_LDFLAGS GOPROXY GOTOOLCHAIN

# ── Utility functions ─────────────────────────────────────────────────────────

# log MESSAGE ...
#   Print a timestamped log line to stdout.
log() {
    printf '[%s] %s\n' "$(date '+%Y-%m-%dT%H:%M:%S')" "$*"
}

# sha256_file FILE
#   Print the SHA-256 hex digest of FILE to stdout.
sha256_file() {
    _sf_file=$1
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$_sf_file" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$_sf_file" | awk '{print $1}'
    else
        log "ERROR: neither sha256sum nor shasum is available for checksum verification"
        exit 1
    fi
}

# verify_sha256 FILE EXPECTED_HEX NAME
#   Abort with an error if FILE's SHA-256 digest does not match EXPECTED_HEX.
verify_sha256() {
    _vs_file=$1
    _vs_expected=$2
    _vs_name=$3
    _vs_actual=$(sha256_file "$_vs_file")
    if [ "$_vs_actual" != "$_vs_expected" ]; then
        log "ERROR: checksum mismatch for $_vs_name"
        log "       expected: $_vs_expected"
        log "       actual:   $_vs_actual"
        exit 1
    fi
}

# verify_gpg FILE SIG_FILE KEY_FILE NAME
#   Verify FILE against the detached GPG signature SIG_FILE using the armored
#   public key in KEY_FILE.  A temporary GPG homedir is used so the system
#   keyring is never touched.  If neither gpg2 nor gpg is installed the check
#   is skipped with a warning; install gnupg2 (yum install gnupg2) for full
#   supply-chain verification.
verify_gpg() {
    _vg_file=$1
    _vg_sig=$2
    _vg_key=$3
    _vg_name=$4
    if ! command -v gpg2 >/dev/null 2>&1 && ! command -v gpg >/dev/null 2>&1; then
        log "WARNING: gpg/gpg2 not found — skipping signature verification for $_vg_name"
        log "         Install gnupg2 (yum install gnupg2) for full supply-chain verification."
        return 0
    fi
    _gpg=$(command -v gpg2 2>/dev/null || command -v gpg 2>/dev/null)
    _vg_home=$(mktemp -d)
    "$_gpg" --homedir "$_vg_home" --batch --quiet --import "$_vg_key"
    if "$_gpg" --homedir "$_vg_home" --batch --trust-model direct \
               --verify "$_vg_sig" "$_vg_file" 2>/dev/null; then
        log "GPG signature OK for $_vg_name"
    else
        log "ERROR: GPG signature verification failed for $_vg_name"
        rm -rf "$_vg_home"
        exit 1
    fi
    rm -rf "$_vg_home"
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

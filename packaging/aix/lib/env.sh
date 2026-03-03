# lib/env.sh — shared environment sourced by every stage script
#
# Usage: . "$SCRIPT_DIR/../lib/env.sh"   (from a stages/NN-name.sh script)
#        . "$SCRIPT_DIR/lib/env.sh"      (from build.sh)
#
# This file is sourced, never executed directly.  Callers control set -e/set -u.
# No validation of required variables is done here; each script validates its
# own inputs after sourcing this file.

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

# Number of available CPUs — nproc does not exist on AIX; lsdev is in /usr/sbin
NPROC=$(/usr/sbin/lsdev -Cc processor | wc -l | tr -d ' ')

export BUILD_DIR STAGING EMBEDDED EMBEDDED_DESTDIR INTEGRATIONS_CORE WHEEL_CACHE NPROC

# ── Agent version variables ───────────────────────────────────────────────────
# AGENT_BRANCH, AGENT_VERSION, and AGENT_BUILD are required inputs.
# They must be set in the caller's environment before sourcing this file.
# AGENT_VRMF is derived here; it is the four-component installp version string.

# Use ${VAR:-} (no-fail) so env.sh can be sourced under set -u before the caller
# validates AGENT_VERSION/AGENT_BUILD.  The individual stage scripts call
#   : "${AGENT_VERSION:?AGENT_VERSION must be set}"
# after sourcing this file; that is where the empty-variable error is reported.
AGENT_VRMF=$(printf '%s' "${AGENT_VERSION:-}" | sed 's/\([0-9]*\.[0-9]*\.[0-9]*\).*/\1/').${AGENT_BUILD:-}

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

export PATH GOPATH GOROOT CGO_ENABLED CGO_CFLAGS CGO_LDFLAGS GOPROXY

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

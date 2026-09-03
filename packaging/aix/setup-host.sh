#!/bin/sh
# setup-host.sh — setup of an AIX/ppc64 host for running tests/packaging/lint
#
# The end result of this script is that the host should have all required tools
# installed with the expected version. It should be idempotent and succeed quickly
# when the tools are already installed, so that the setup is fast in CI jobs.

set -eu

PATH=/opt/freeware/bin:$PATH
export PATH

BUILD_DIR=/opt/dd-build
DNF=/opt/freeware/bin/dnf-3
AGENT_SRC=$(cd "$(dirname "$0")" && pwd)
while [ "$AGENT_SRC" != "/" ] && [ ! -e "$AGENT_SRC/.git" ]; do
    AGENT_SRC=$(dirname "$AGENT_SRC")
done

GO_VERSION=$(cat "$AGENT_SRC/.go-version")

# /opt is tiny on a fresh SiteOX LPAR; grow it to fit the toolchain + build cache.
OPT_TARGET_BLOCKS=83886080   # 40 GiB (in 512-byte blocks)

log() { printf '[%s] %s\n' "$(date '+%H:%M:%S')" "$*"; }

dnf_install() {
    # --allowerasing: resolve conflicts between monolithic and split subpackages
    # (e.g. cyrus-sasl vs cyrus-sasl-lib) on hosts that have the old form.
    log "dnf install: $*"
    "$DNF" -y --allowerasing install "$@"
}

# ── Preflight ─────────────────────────────────────────────────────────────────
[ "$(id -u)" -eq 0 ] || { echo "ERROR: run as root" >&2; exit 1; }

mkdir -p "$BUILD_DIR/.done" "$BUILD_DIR/bin" "$BUILD_DIR/gocache" "$BUILD_DIR/buildtmp" "$BUILD_DIR/logs"

# ── Step 1: grow /opt ────────────────────────────────────────────────────────
_opt_cur=$(lsfs /opt 2>/dev/null | awk 'NR==2{print $5}')
if [ -n "$_opt_cur" ] && [ "$_opt_cur" -lt "$OPT_TARGET_BLOCKS" ]; then
    log "Growing /opt to 40 GiB"
    chfs -a size="$OPT_TARGET_BLOCKS" /opt
else
    log "/opt already large enough"
fi

# ── Step 2: AIX Toolbox packages ─────────────────────────────────────────────
log "Refreshing dnf metadata"
"$DNF" -y makecache >/dev/null 2>&1 || true

# gcc8 is required by env.sh for shipped-binary compatibility.
# python3.12-pydantic-core is a prebuilt Rust extension (pydantic v2 needs it).
dnf_install \
    cyrus-sasl \
    gcc8 gcc8-c++ \
    git make cmake binutils diffutils \
    protobuf protobuf-compiler \
    zstd xz curl bash \
    libffi-devel ncurses-devel readline-devel libxslt-devel libiconv \
    python3.12-pydantic-core

# ── Step 3: Go toolchain at /opt/go ───────────────────────────────────────────
# The AIX Toolbox golang maxes at 1.26.5 but .go-version requires 1.26.6, so
# install the official aix-ppc64 tarball.
if [ -x /opt/go/bin/go ] && [ "$(/opt/go/bin/go version | awk '{print $3}')" = "go${GO_VERSION}" ]; then
    log "Go ${GO_VERSION} already installed"
else
    log "Installing Go ${GO_VERSION} (aix-ppc64)"
    _tar="/tmp/go${GO_VERSION}.aix-ppc64.tar.gz"
    [ -f "$_tar" ] || curl -fSL -o "$_tar" "https://go.dev/dl/go${GO_VERSION}.aix-ppc64.tar.gz"
    rm -rf /opt/go
    gunzip -c "$_tar" | tar xf - -C /opt
fi

# ── Step 4: Python deps for the `tasks` invoke namespace ─────────────────────
log "Bootstrapping pip"
/opt/freeware/bin/python3.12 -m pip --version >/dev/null 2>&1 || \
    /opt/freeware/bin/python3.12 -m ensurepip --upgrade

# Install the repo's pinned dev deps, plus the transitive imports of the tasks
# namespace that aren't in py_dev_requirements.txt.
log "Installing Python deps"
/opt/freeware/bin/python3.12 -m pip install --upgrade --no-input \
    -r "$AGENT_SRC/deps/py_dev_requirements.txt" \
    requests boto3 botocore semver packaging \
    python-gitlab codeowners termcolor \
    annotated-types typing-extensions typing-inspection
# pydantic v2 needs pydantic-core (Rust); the prebuilt RPM is installed above,
# so install the pure-Python wrapper with --no-deps to reuse it.
/opt/freeware/bin/python3.12 -m pip install --no-input --no-deps "pydantic==2.11.7"

# Source env.sh now that invoke is installed: it provides BUILD_DIR,
# RUST_VERSION, PATH, CC/CXX, and the gcc/g++ symlinks needed below.
# shellcheck source=/dev/null
. "$AGENT_SRC/packaging/aix/lib/env.sh"

# ── Step 5: gotestsum (no Bazel on AIX) ──────────────────────────────────────
# tasks/gotest.py runs gotestsum directly on AIX. Install it the same way
# `dda inv install-tools` does: `go install` from internal/tools/go.mod.
if [ -x "$BUILD_DIR/bin/gotestsum" ]; then
    log "gotestsum already installed"
else
    log "Installing gotestsum"
    ( cd "$AGENT_SRC/internal/tools" && \
        GOBIN="$BUILD_DIR/bin" \
        GOCACHE="$BUILD_DIR/gocache" TMPDIR="$BUILD_DIR/buildtmp" \
        PATH="/opt/go/bin:$PATH" \
        GOPROXY=https://proxy.golang.org,direct GOTOOLCHAIN=local \
        /opt/go/bin/go install gotest.tools/gotestsum )
fi

# ── Step 5b: Rust SDK (build/packaging) ──────────────────────────────────────
# AIX 7.3 ships the IBM Open XL C/C++ Runtime the SDK requires.
if [ -x "/opt/freeware/lib/RustSDK/${RUST_VERSION}/bin/cargo" ]; then
    log "Rust ${RUST_VERSION} already installed"
else
    log "Installing Rust ${RUST_VERSION}"
    "$DNF" -y --allowerasing install \
        "rust${RUST_VERSION}" "cargo${RUST_VERSION}" "rust${RUST_VERSION}-std-static" \
        "rust${RUST_VERSION}-community-license"
fi

# ── Step 5c: IBM MQ Client (build/packaging) ──────────────────────────────────
# Required by the packaging build (pymqi for the ibm_mq check). It's an
# installp fileset from IBM Fix Central (not on the AIX Toolbox); the CI job
# transfers the archive to $BUILD_DIR/mqclient.tar.Z (see .aix_remote).
if lslpp -Lq mqm.base.runtime >/dev/null 2>&1; then
    log "IBM MQ Client already installed"
elif [ -f "$BUILD_DIR/mqclient.tar.Z" ]; then
    log "Installing IBM MQ Client"
    rm -rf "$BUILD_DIR/buildtmp/mqclient"
    mkdir -p "$BUILD_DIR/buildtmp/mqclient"
    ( cd "$BUILD_DIR/buildtmp/mqclient" && zcat "$BUILD_DIR/mqclient.tar.Z" | tar xf - )
    installp -acXg -d "$BUILD_DIR/buildtmp/mqclient/MQClient" -Y mqm.base.runtime mqm.base.sdk mqm.client.rte
    rm -rf "$BUILD_DIR/buildtmp/mqclient"
else
    echo "ERROR: IBM MQ Client not installed and $BUILD_DIR/mqclient.tar.Z not found." >&2
    echo "       The CI job template (.aix_remote) fetches it from MASS and transfers it." >&2
    exit 1
fi

# ── Step 6: verify ────────────────────────────────────────────────────────────
log "=== Verification ==="

verify() {
    _got=$(eval "$2" 2>/dev/null || true)
    if printf '%s' "$_got" | grep -q "$3"; then
        printf '  OK   %-12s %s\n' "$1" "$_got"
    else
        printf '  FAIL %-12s (got: %s, expected: %s)\n' "$1" "$_got" "$3" >&2
        return 1
    fi
}

_rc=0
verify "gcc-8"      "/opt/freeware/bin/gcc-8 --version | head -1" "8\\."            || _rc=1
verify "g++-8"      "/opt/freeware/bin/g++-8 --version | head -1" "8\\."            || _rc=1
verify "go"         "/opt/go/bin/go version" "go${GO_VERSION}"                       || _rc=1
verify "pydantic"   "/opt/freeware/bin/python3.12 -c 'import pydantic;print(pydantic.VERSION)'" "2.11" || _rc=1
verify "git"        "git --version" "git version"                                  || _rc=1
verify "make"       "make --version | head -1" "GNU Make"                         || _rc=1
verify "cmake"      "cmake --version | head -1" "cmake version"                   || _rc=1
verify "protoc"     "protoc --version" "libprotoc"                                || _rc=1
verify "strip"      "strip -V 2>&1 | head -1" "strip"                                  || _rc=1
verify "dump"       "which dump" "dump"                                            || _rc=1
verify "mkinstallp" "which mkinstallp" "/usr/sbin/mkinstallp"                       || _rc=1
verify "zstd"       "zstd --version" "Zstandard"                                     || _rc=1
verify "python3.12" "/opt/freeware/bin/python3.12 --version" "3.12"               || _rc=1
verify "invoke"     "/opt/freeware/bin/python3.12 -m invoke --version" "Invoke"   || _rc=1
verify "gotestsum"  "$BUILD_DIR/bin/gotestsum --version 2>&1 | head -1" "gotestsum"   || _rc=1
verify "cargo"      "/opt/freeware/lib/RustSDK/${RUST_VERSION}/bin/cargo --version" "cargo" || _rc=1
verify "mqm"        "lslpp -Lq mqm.base.runtime" "mqm.base.runtime"                     || _rc=1

[ "$_rc" -eq 0 ] || { echo "ERROR: tool verification failed" >&2; exit 1; }

log "=== setup-host.sh complete ==="

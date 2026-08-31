#!/bin/sh
# setup-host.sh — idempotent provisioning of an AIX/ppc64 host for running
# datadog-agent CI jobs (unit tests, packaging build, lint) over SSH
# (RFC: AIX/ppc64 datadog-agent CI, Phase 1).
#
# This is the single host setup script: it installs the union of dependencies
# for test, build, and packaging jobs. Since the CI runner is long-lived, the
# first run installs everything (slower); subsequent runs are fast (just verify
# tools are present).
#
# What it does:
#   1. Grows /opt from rootvg (AIX Toolbox installs into /opt/freeware, which is
#      tiny on a fresh SiteOX LPAR) and creates the /opt/dd-build build tree.
#   2. Installs the AIX Toolbox packages required by the agent toolchain
#      (gcc8/g++8, git, cmake, protobuf, ...) and the prebuilt pydantic-core
#      RPM. Installs the official Go aix-ppc64 tarball at /opt/go (the AIX
#      Toolbox maxes at 1.26.5, but go.work requires >= 1.26.6).
#   3. Bootstraps pip and installs the Python deps needed to load the `tasks`
#      invoke namespace (so `inv -e test` works).
#   4. Installs gotestsum (from the AIX-fix fork, gotestyourself/gotestsum#567)
#      into /opt/dd-build/bin — AIX has no Bazel, so `inv test` runs gotestsum
#      directly (see the AIX fallback in tasks/gotest.py).
#   5. Installs the Rust SDK (needed by the packaging build for pydantic-core,
#      cryptography, ...). AIX 7.3 ships the IBM Open XL C/C++ Runtime the SDK
#      requires, so it installs cleanly via dnf.
#
# Reuses the toolchain layout/variables from packaging/aix/lib/env.sh: this
# script does NOT source env.sh (it must run before gcc-8/go exist), but it
# materializes the paths env.sh later expects: /opt/go, /opt/dd-build/bin,
# /opt/dd-build/gocache, /opt/dd-build/buildtmp.
#
# Safe to re-run: every step is guarded. Run as root:
#   ./setup-host.sh
#
# NOTE: excludes the `python` build tag story (rtloader, embedded Python, Rust
# extensions) — unit tests here are Go-only without the `python` tag.

set -eu

# AIX Toolbox installs everything under /opt/freeware/bin; the default root
# PATH does not include it, so add it first.
PATH=/opt/freeware/bin:/usr/bin:/etc:/usr/sbin:/usr/ucb:/usr/bin/X11:/sbin
export PATH

# ── Configuration ────────────────────────────────────────────────────────────
# Go: go.work requires `go 1.26.6`; the AIX Toolbox maxes at 1.26.5, so install
# the official aix-ppc64 tarball instead (Go upstream supports aix/ppc64).
GO_VERSION="1.26.7"
GO_SHA256="e5aed98bc83f569a374dbfd79d632b058e5130192fbd4c4ee0d4f6e3d9c101bb"
# pydantic wrapper version matching the prebuilt pydantic-core RPM below.
PYDANTIC_VERSION="2.11.7"
# gotest.tools/gotestsum v1.13.0 doesn't compile on AIX (event.Args undefined
# in cmd/watch.go). Use the AIX fix branch until it's merged and released:
# https://github.com/gotestyourself/gotestsum/pull/567
GOTESTSUM_VERSION="2222dd98bb9fdf07bd834476d0223401b7964fd0"
# Rust SDK version — keep in sync with packaging/aix/lib/env.sh (RUST_VERSION).
# Needed by the packaging build (pydantic-core, cryptography, ...). AIX 7.3 ships
# the IBM Open XL C/C++ Runtime the SDK requires, so it installs via dnf.
RUST_VERSION="1.92"
INVOKE_VERSION="2.2.1"         # deps/py_dev_requirements.txt

# /opt target size in 512-byte blocks (40 GiB). A fresh SiteOX LPAR ships /opt
# at 3 GiB — far too small for golang + git + cmake + protobuf + the build cache.
OPT_TARGET_BLOCKS=83886080     # 40 * 1024 * 1024 * 2

BUILD_DIR=/opt/dd-build
DNF=/opt/freeware/bin/dnf-3

# ── Helpers ──────────────────────────────────────────────────────────────────
log() {
    printf '[%s] %s\n' "$(date '+%H:%M:%S')" "$*"
}

sentinel_done() { [ -f "$BUILD_DIR/.done/$1" ]; }
sentinel_mark() { mkdir -p "$BUILD_DIR/.done"; touch "$BUILD_DIR/.done/$1"; }

# Run dnf (which is a symlink to dnf-3 on AIX Toolbox). dnf metadata is stale on
# these hosts; refresh quietly first.
dnf_install() {
    # --allowerasure: the AIX Toolbox ships some packages both as monolithic and
    # split subpackages (e.g. cyrus-sasl vs cyrus-sasl-lib); on hosts that have
    # the old monolithic form installed, a dependency pull of the new
    # subpackage conflicts. Let dnf remove the conflicting old package.
    log "dnf install: $*"
    "$DNF" -y --allowerasing install "$@"
}

# ── Preflight ─────────────────────────────────────────────────────────────────
if [ "$(id -u)" -ne 0 ]; then
    echo "ERROR: run as root (needed for dnf, chfs, /opt writes)" >&2
    exit 1
fi

mkdir -p "$BUILD_DIR" "$BUILD_DIR/.done" "$BUILD_DIR/bin" \
         "$BUILD_DIR/gocache" "$BUILD_DIR/buildtmp" "$BUILD_DIR/logs"

# ── Step 1: grow /opt ────────────────────────────────────────────────────────
# /opt is on /dev/hd10opt (rootvg). rootvg has ~70 GiB free on a 4-proc SiteOX
# LPAR. Grow once to OPT_TARGET_BLOCKS; chfs is a no-op if already that size or
# larger (we only grow, never shrink).
_opt_cur=$(lsfs /opt 2>/dev/null | awk 'NR==2{print $5}')
if [ -n "$_opt_cur" ] && [ "$_opt_cur" -lt "$OPT_TARGET_BLOCKS" ]; then
    log "Growing /opt from $_opt_cur to $OPT_TARGET_BLOCKS blocks (40 GiB)"
    chfs -a size="$OPT_TARGET_BLOCKS" /opt
else
    log "/opt already large enough ($_opt_cur blocks)"
fi

# ── Step 2: AIX Toolbox packages ─────────────────────────────────────────────
# Toolchain + build tools. Go is installed separately from the official
# aix-ppc64 tarball in Step 3 (the AIX Toolbox golang maxes at 1.26.5, but
# go.work requires >= 1.26.6).
log "Refreshing dnf metadata (may take a moment)"
"$DNF" -y makecache >/dev/null 2>&1 || true

# gcc8/g++8: required by packaging/aix/lib/env.sh (it hard-fails without
#   /opt/freeware/bin/gcc-8) for AIX 7.2 TL2 *shipped-binary* compatibility
#   (gcc-8's libstdc++ avoids strftime_l). On AIX 7.3 gcc-8's include-fixed
#   headers match the system headers, so no gcc-13 workaround is needed.
# python3.12-pydantic-core: prebuilt Rust extension (pydantic v2 needs it).
dnf_install \
    cyrus-sasl \
    gcc8 gcc8-c++ \
    git make cmake binutils diffutils \
    protobuf protobuf-compiler \
    zstd xz curl bash \
    libffi-devel ncurses-devel readline-devel libxslt-devel libiconv \
    python3.12-pydantic-core

# ── Step 3: official Go toolchain at /opt/go ────────────────────────────────
# env.sh sets GOROOT=/opt/go and PATH=.../opt/go/bin.... The AIX Toolbox golang
# package maxes at 1.26.5, but go.work requires >= 1.26.6, so install the
# official aix-ppc64 tarball as a real /opt/go directory (not a symlink).
_go_ok=0
if [ -x /opt/go/bin/go ]; then
    _have=$(/opt/go/bin/go version 2>/dev/null | awk '{print $3}')
    [ "$_have" = "go${GO_VERSION}" ] && _go_ok=1
fi
if [ "$_go_ok" = "0" ]; then
    log "Installing official Go ${GO_VERSION} (aix-ppc64) into /opt/go"
    _tar="/tmp/go${GO_VERSION}.aix-ppc64.tar.gz"
    [ -f "$_tar" ] || curl -fSL -o "$_tar" "https://go.dev/dl/go${GO_VERSION}.aix-ppc64.tar.gz"
    # AIX has no sha256sum; use openssl.
    _got_sha=$(openssl dgst -sha256 "$_tar" 2>/dev/null | awk '{print $NF}')
    if [ "$_got_sha" != "$GO_SHA256" ]; then
        echo "ERROR: Go tarball sha256 mismatch: $_got_sha != $GO_SHA256" >&2
        rm -f "$_tar"
        exit 1
    fi
    rm -rf /opt/go
    gunzip -c "$_tar" | tar xf - -C /opt
else
    log "Go ${GO_VERSION} already installed at /opt/go"
fi

# Private gcc/g++ -> gcc-8 in $BUILD_DIR/bin (env.sh creates these itself on
# source, but create them here too so the tree is usable before env.sh is
# sourced). On AIX 7.3 gcc-8's include-fixed headers match the system headers,
# so no gcc-13 override is needed (unlike the 7.2 TL5 host).
ln -sf /opt/freeware/bin/gcc-8 "$BUILD_DIR/bin/gcc"
ln -sf /opt/freeware/bin/g++-8 "$BUILD_DIR/bin/g++"

# ── Step 4: Python deps for the `tasks` invoke namespace ─────────────────────
# `inv -e test` loads all of tasks/__init__.py, which transitively imports
# invoke, pyyaml, requests, jinja2, boto3, semver, ... Bootstrap pip via
# ensurepip (bundled with CPython 3.12) then install.
log "Bootstrapping pip for python3.12"
if ! /opt/freeware/bin/python3.12 -m pip --version >/dev/null 2>&1; then
    /opt/freeware/bin/python3.12 -m ensurepip --upgrade
fi

# Pinned core deps from deps/py_dev_requirements.txt; the rest are transitive
# imports of the tasks namespace (boto3 via e2e_framework, semver via kmt,
# python-gitlab via gitlab_api, pydantic via e2e_framework/config, ...).
# pydantic v2 needs pydantic-core (Rust); the prebuilt RPM is installed above,
# so install the pure-Python pydantic wrapper with --no-deps (its deps are
# annotated-types / typing-extensions / typing-inspection, all pure Python).
log "Installing Python deps for tasks namespace"
/opt/freeware/bin/python3.12 -m pip install --upgrade --no-input \
    "invoke==${INVOKE_VERSION}" \
    "jinja2==3.1.6" \
    "patch-ng==1.19.1" \
    "pyyaml==6.0.3" \
    requests boto3 botocore semver packaging \
    python-gitlab codeowners termcolor \
    annotated-types typing-extensions typing-inspection
/opt/freeware/bin/python3.12 -m pip install --no-input --no-deps \
    "pydantic==${PYDANTIC_VERSION}"

# ── Step 5: gotestsum (no Bazel on AIX) ──────────────────────────────────────
# AIX has no Bazel, so tasks/gotest.py runs a prebuilt gotestsum directly. Install
# the version pinned in internal/tools/go.mod into $BUILD_DIR/bin (which env.sh
# puts on PATH). GOBIN/GOCACHE/TMPDIR keep the build off the tiny /tmp.
need_gotestsum=1
if [ -x "$BUILD_DIR/bin/gotestsum" ]; then
    # Record the installed commit in a stamp file so re-runs can skip the
    # (slow) go install when the pinned version hasn't changed.
    _stamp="$BUILD_DIR/.done/gotestsum-version"
    if [ -f "$_stamp" ] && [ "$(cat "$_stamp" 2>/dev/null)" = "$GOTESTSUM_VERSION" ]; then
        log "gotestsum $GOTESTSUM_VERSION already installed"
        need_gotestsum=0
    fi
fi
if [ "$need_gotestsum" = "1" ]; then
    log "Installing gotestsum $GOTESTSUM_VERSION into $BUILD_DIR/bin"
    # The AIX fix lives on a PR branch in a fork (pgimalac/gotestsum#567),
    # not a tagged release of gotestyourself/gotestsum, so the module proxy
    # (which maps gotest.tools/gotestsum -> gotestyourself/gotestsum) can't
    # resolve the commit. Clone the fork at the pinned commit and build it.
    _gs_src="$BUILD_DIR/buildtmp/gotestsum-src"
    rm -rf "$_gs_src"
    git clone --quiet --depth=1 https://github.com/pgimalac/gotestsum.git "$_gs_src"
    git -C "$_gs_src" fetch --quiet --depth=1 origin "$GOTESTSUM_VERSION"
    git -C "$_gs_src" checkout --quiet "$GOTESTSUM_VERSION"
    ( cd "$_gs_src" && \
        GOCACHE="$BUILD_DIR/gocache" TMPDIR="$BUILD_DIR/buildtmp" \
        PATH="/opt/go/bin:/opt/freeware/bin:/usr/bin:$PATH" \
        GOPROXY=https://proxy.golang.org,direct \
        /opt/go/bin/go build -o "$BUILD_DIR/bin/gotestsum" . )
    rm -rf "$_gs_src"
    printf '%s' "$GOTESTSUM_VERSION" > "$BUILD_DIR/.done/gotestsum-version"
fi

# ── Step 5b: Rust SDK (build/packaging) ────────────────────────────────────
# AIX 7.3 ships the IBM Open XL C/C++ Runtime (xlC.aix61.rte >= 16.1.0.10)
# that the Rust SDK links against, so it installs cleanly via dnf. Required by
# the packaging build (pydantic-core, cryptography, ...).
if [ -x "/opt/freeware/lib/RustSDK/${RUST_VERSION}/bin/cargo" ]; then
    log "Rust ${RUST_VERSION} already installed"
else
    log "Installing Rust ${RUST_VERSION}"
    "$DNF" -y --allowerasing install \
        "rust${RUST_VERSION}" "cargo${RUST_VERSION}" "rust${RUST_VERSION}-std-static" \
        "rust${RUST_VERSION}-community-license"
fi

# ── Step 6: verify ────────────────────────────────────────────────────────────
log "=== Verification ==="

verify() {
    _name=$1; _cmd=$2; _expect=$3
    _got=$(eval "$_cmd" 2>/dev/null || true)
    if printf '%s' "$_got" | grep -q "$_expect"; then
        printf '  OK   %-12s %s\n' "$_name" "$_got"
    else
        printf '  FAIL %-12s (got: %s, expected: %s)\n' "$_name" "$_got" "$_expect" >&2
        return 1
    fi
}

_rc=0
verify "gcc-8"      "/opt/freeware/bin/gcc-8 --version | head -1" "8\\."            || _rc=1
verify "g++-8"      "/opt/freeware/bin/g++-8 --version | head -1" "8\\."            || _rc=1
verify "go"         "/opt/go/bin/go version" "go${GO_VERSION}"                       || _rc=1
verify "pydantic"   "/opt/freeware/bin/python3.12 -c 'import pydantic;print(pydantic.VERSION)'" "${PYDANTIC_VERSION}" || _rc=1
verify "git"        "git --version" "git version"                                  || _rc=1
verify "make"       "make --version | head -1" "GNU Make"                         || _rc=1
verify "cmake"      "cmake --version | head -1" "cmake version"                   || _rc=1
verify "protoc"     "protoc --version" "libprotoc"                                || _rc=1
verify "strip"      "strip -V 2>&1 | head -1" "strip"                                  || _rc=1
# AIX OS filesets (not rpm) required by the packaging build (build.sh).
verify "dump"       "which dump" "/usr/bin/dump"                                       || _rc=1
verify "mkinstallp" "which mkinstallp" "/usr/sbin/mkinstallp"                       || _rc=1
verify "zstd"       "zstd --version" "Zstandard"                                     || _rc=1
verify "python3.12" "/opt/freeware/bin/python3.12 --version" "3.12"               || _rc=1
verify "invoke"     "/opt/freeware/bin/python3.12 -m invoke --version" "Invoke"   || _rc=1
verify "gotestsum"  "$BUILD_DIR/bin/gotestsum --version 2>&1 | head -1" "gotestsum"   || _rc=1
# Rust is required by the packaging build.
verify "cargo"      "/opt/freeware/lib/RustSDK/${RUST_VERSION}/bin/cargo --version" "cargo" || _rc=1

if [ "$_rc" -ne 0 ]; then
    echo "ERROR: one or more tool verifications failed" >&2
    exit 1
fi

sentinel_mark setup-host
log "=== setup-host.sh complete ==="

#!/usr/bin/env bash
# Build an unsigned *x86_64* macOS Agent .dmg from the current working tree, on an Apple Silicon
# machine. Needed because the shared macOS E2E pool hands out a mac1.metal (x86_64) host, and a
# .pkg declares hostArchitectures from Ohai["kernel"]["machine"] (omnibus-ruby packagers/pkg.rb),
# so an arm64 build is refused outright by macOS installer ("Agent can't be installed on this
# computer") before any script runs.
#
# Two halves, each targeted differently:
#   * The Ruby/omnibus half runs translated under Rosetta, so uname -m -- and therefore the package
#     name and the pkg's hostArchitectures -- is x86_64.
#   * The bazel half cross-compiles natively via --platforms=//bazel/platforms:macos_x86_64, which
#     is faster than translating bazel itself and is what the platform definitions already exist
#     for. Rosetta still matters here: bazel builds CPython from source and any build step that
#     executes a freshly built x86_64 binary relies on translation to run at all.
set -eo pipefail

# Resolved before the cd below, so the re-exec and the goshim path work however the script was invoked.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

cd "$(git rev-parse --show-toplevel)"

# Bundler only learns where the gems live from this untracked file (see below), so stop here rather
# than have it try the system gem directory.
if ! grep -q 'BUNDLE_PATH' omnibus/.bundle/config 2>/dev/null; then
    echo "omnibus/.bundle/config is missing; create it with: BUNDLE_PATH: \"~/.omnibus-gems\" (references/setup.md)" >&2
    exit 1
fi

# Re-exec translated if we are not already. Rosetta gives x86_64 to this whole process tree, which
# is what makes uname/Ohai report x86_64 below.
if [ "$(/usr/bin/uname -m)" != "x86_64" ]; then
    exec arch -x86_64 /bin/bash "$SCRIPT_DIR/$(basename "$0")" "$@"
fi

# PATH must prefer /usr/bin: the Homebrew tools are arm64-only, and Ohai shelling out to an
# arm64-only `uname` under Rosetta fails with "Bad CPU type in executable".
# goshim/ holds a `go` wrapper that forces GOARCH=amd64; see the comment in the shim itself for why
# exporting GOARCH here does not work.
export PATH="$SCRIPT_DIR/goshim:$HOME/.gem/ruby/2.6.0/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"

# Build with the Go release .go-version names, not whatever Homebrew has moved on to: a newer Go can
# stamp pure-Go binaries with a higher minimum macOS than check_macos_version.sh accepts (Go 1.27
# stamps 13.0 against the required 12.0). `go install golang.org/dl/go<ver> && go<ver> download`
# puts that release in ~/sdk.
# Only PATH reaches the build (tasks/libs/common/omnibus.py ENV_PASSHTROUGH), so the order of PATH
# is what selects the Go release; GOTOOLCHAIN would be dropped. Then fail fast like CI's
# .macos_setup_go does, rather than 10 minutes into the build.
GO_SDK_BIN="$HOME/sdk/go$(cat .go-version)/bin"
if [ -x "$GO_SDK_BIN/go" ]; then
    # After goshim/, which runs the next `go` on PATH: this one.
    export PATH="$SCRIPT_DIR/goshim:$GO_SDK_BIN:${PATH#"$SCRIPT_DIR/goshim:"}"
fi
dda inv check-go-version

# omnibazel_flags (omnibus/lib/ostools.rb) is fixed, and omnibus invokes bazel itself, so the target
# platform has to come from a bazelrc. user.bazelrc is gitignored and try-imported last (.bazelrc),
# but it is shared with every other bazel command in the checkout -- an e2e run would otherwise
# build its test binaries for x86_64 too -- so the line is removed again on exit.
BAZELRC_MARK="# --- x86_64 dmg build (temporary) ---"

# Latest versioned compiler-rt dir shipped by the Command Line Tools (see the note below).
CLT_COMPILER_RT_DIR="$(/bin/ls -d /Library/Developer/CommandLineTools/usr/lib/clang/*/lib/darwin 2>/dev/null | /usr/bin/tail -1)"
if [ -z "$CLT_COMPILER_RT_DIR" ] || ! /usr/bin/lipo -archs "$CLT_COMPILER_RT_DIR/libclang_rt.osx.a" | /usr/bin/grep -q x86_64; then
    echo "no x86_64 compiler-rt in the Command Line Tools; install them (xcode-select --install)" >&2
    exit 1
fi

# The block is appended, so everything from the mark to end of file is ours to remove.
cleanup_bazelrc() {
    if [ -f user.bazelrc ]; then
        /usr/bin/sed -i '' "/$(printf '%s' "$BAZELRC_MARK" | /usr/bin/sed 's/[][\.*^$/-]/\\&/g')/,\$d" user.bazelrc
    fi
}
trap cleanup_bazelrc EXIT
cleanup_bazelrc
# The linkopts are not optional: the hermetic LLVM toolchain ships an arm64-only
# libclang_rt.osx.a, which ld64.lld silently ignores when targeting x86_64 (it says so as a
# warning). Go/cgo links fine without it, but CPython uses __builtin_available, so its link fails
# with "undefined symbol: __isPlatformVersionAtLeast". The Command Line Tools' compiler-rt is fat
# (i386 x86_64 x86_64h arm64 arm64e), so pointing the link at that one supplies the builtin.
cat >> user.bazelrc <<EOF
$BAZELRC_MARK
common:macos --platforms=//bazel/platforms:macos_x86_64
common:macos --linkopt=-L$CLT_COMPILER_RT_DIR
common:macos --linkopt=-lclang_rt.osx
EOF

# The build mounts the .dmg it produces; a leftover mount from a previous run makes it fail.
# Deliberately without sudo: this runs unattended and a password prompt would hang it.
hdiutil detach /Volumes/Agent 2>/dev/null || true

unset OMNIBUS_BASE_DIR
export INSTALL_DIR="${TMPDIR}datadog-agent-build-x86/bin"
export CONFIG_DIR="${TMPDIR}datadog-agent-build-x86/config"
export OMNIBUS_DIR="${TMPDIR}omnibus_build_x86"
export OMNIBUS_PACKAGE_DIR="$PWD/omnibus/pkg"

rm -rf "$INSTALL_DIR" "$CONFIG_DIR" "$OMNIBUS_DIR"
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$OMNIBUS_DIR" "$OMNIBUS_PACKAGE_DIR"

GOMODCACHE="$(go env GOMODCACHE)"
export GOMODCACHE

# Rosetta translates only this process: an arm64 `go` binary invoked from here still runs natively
# as arm64, so the non-bazel half of the build (omnibus runs `dda inv agent.build`) would default to
# GOARCH=arm64 and then fail to link the x86_64 rtloader bazel just built ("ignoring file ...: found
# architecture 'x86_64', required architecture 'arm64'"). Exporting GOARCH here only covers this
# script's own `go` calls: the omnibus task drops it (see below), which is why goshim/ exists. cgo
# cross-compiles fine because cmd/go passes -arch x86_64 to clang on darwin.
export GOARCH=amd64
export CGO_ENABLED=1

# The omnibus task runs bundler and omnibus with replace_env=True and an allowlist
# (tasks/libs/common/omnibus.py ENV_PASSHTROUGH): GOARCH, RBENV_VERSION, BUNDLE_PATH and SDKROOT
# never get there, and the task sets MACOSX_DEPLOYMENT_TARGET=12.0 itself. What reaches bundler is
# PATH (the system Ruby pin above -- omnibus-ruby's Builder#delete breaks on Ruby 3.x, and
# /usr/bin/ruby is universal, so it runs x86_64 translated), HOME, and the working directory:
# omnibus/.bundle/config (untracked) is what pins the gems to ~/.omnibus-gems. Bundler keys that
# tree by ruby version, not architecture, so its native extensions (ffi and friends) belong to
# whichever architecture last ran `bundle install`: going back to an arm64 build means re-running
# `bundle install` natively first (references/setup.md).

echo "=== arch check: uname=$(/usr/bin/uname -m) ruby=$(/usr/bin/ruby -e 'print RUBY_PLATFORM') ==="

# Same flags as CI's unsigned branch (.gitlab/build/package_build/build_agent_dmg.sh). --skip-deps
# only skips `dda inv go.deps`; the task still runs `bundle install` in omnibus/ every time.
dda inv -- -e omnibus.build \
    --skip-sign \
    --skip-deps \
    --go-mod-cache="$GOMODCACHE" \
    --config-directory "$CONFIG_DIR" \
    --install-directory "$INSTALL_DIR" \
    --base-dir "$OMNIBUS_DIR"

"$SCRIPT_DIR/check_install_symlinks.sh" "$INSTALL_DIR"

echo "=== built packages ==="
ls -la "$OMNIBUS_PACKAGE_DIR"

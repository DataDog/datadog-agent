#!/usr/bin/env bash
# Build an unsigned *arm64* macOS Agent .dmg from the current working tree, natively on Apple
# Silicon. Sibling of build_local_dmg_x86.sh, which exists only because the
# shared macOS E2E pool hands out x86_64 mac1.metal hosts; when the target is the laptop itself (or
# any arm64 host), none of that machinery is needed:
#   * no `arch -x86_64` re-exec -- uname/Ohai already report arm64, so the .pkg's
#     hostArchitectures comes out right on its own.
#   * no user.bazelrc --platforms override -- the host platform is already the target.
#   * no goshim -- `go` defaults to GOARCH=arm64.
#   * no CLT compiler-rt linkopts -- the hermetic LLVM toolchain's own libclang_rt.osx.a is
#     arm64-only, which is exactly what is wanted here.
# What is still needed is the Ruby side: omnibus-ruby's Builder#delete passes a positional options
# hash to FileUtils.rm_rf, which Ruby 3.x rejects, so the build runs under the system Ruby 2.6.10
# like the macOS CI runners do.
#
# Run it from inside the checkout/worktree you want to build; it locates that itself.
set -eo pipefail

# Resolved before the cd below, so the helper scripts are found however this one was invoked.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

cd "$(git rev-parse --show-toplevel)"

# Bundler only learns where the gems live from this untracked file (see below), so stop here rather
# than have it try the system gem directory.
if ! grep -q 'BUNDLE_PATH' omnibus/.bundle/config 2>/dev/null; then
    echo "omnibus/.bundle/config is missing; create it with: BUNDLE_PATH: \"~/.omnibus-gems\" (references/setup.md)" >&2
    exit 1
fi

if [ "$(/usr/bin/uname -m)" != "arm64" ]; then
    echo "this builds an arm64 dmg and must run natively on Apple Silicon (uname -m: $(/usr/bin/uname -m))" >&2
    echo "for an x86_64 dmg use build_local_dmg_x86.sh next to this script" >&2
    exit 1
fi

# Select the system Ruby (2.6.10, universal so it runs natively) via PATH rather than
# RBENV_VERSION: that variable does not reach the `bundle exec omnibus` that dda inv spawns, so the
# build ends up on rbenv's 3.2.x and dies in omnibus-ruby's Builder#delete with
# "rm_rf: wrong number of arguments (given 2, expected 1)". Putting /usr/bin ahead of the rbenv
# shims is what actually pins it. ~/.gem/ruby/2.6.0/bin supplies a bundler binstub for that Ruby.
# $PATH is kept on the end so dda and the rest of the toolchain still resolve.
export PATH="$HOME/.gem/ruby/2.6.0/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"

# Build with the Go release .go-version names, not whatever Homebrew has moved on to: a newer Go can
# stamp pure-Go binaries with a higher minimum macOS than check_macos_version.sh accepts (Go 1.27
# stamps 13.0 against the required 12.0). `go install golang.org/dl/go<ver> && go<ver> download`
# puts that release in ~/sdk.
# Only PATH reaches the build (tasks/libs/common/omnibus.py ENV_PASSHTROUGH), so the order of PATH
# is what selects the Go release; GOTOOLCHAIN would be dropped. Then fail fast like CI's
# .macos_setup_go does, rather than 10 minutes into the build.
GO_SDK_BIN="$HOME/sdk/go$(cat .go-version)/bin"
if [ -x "$GO_SDK_BIN/go" ]; then
    export PATH="$GO_SDK_BIN:$PATH"
fi
dda inv check-go-version

# The build mounts the .dmg it produces; a leftover mount from a previous run makes it fail.
# Deliberately without sudo: this runs unattended and a password prompt would hang it.
hdiutil detach /Volumes/Agent 2>/dev/null || true

# Same layout as CI (.gitlab/build/package_build/build_agent_dmg.sh), including the /bin leaf of
# INSTALL_DIR, so that a local build exercises the same install-path handling the packaged build
# does -- that leaf is what used to leak into AgentPackageVersion.
unset OMNIBUS_BASE_DIR
export INSTALL_DIR="${TMPDIR}datadog-agent-build/bin"
export CONFIG_DIR="${TMPDIR}datadog-agent-build/config"
export OMNIBUS_DIR="${TMPDIR}omnibus_build"
export OMNIBUS_PACKAGE_DIR="$PWD/omnibus/pkg"

rm -rf "$INSTALL_DIR" "$CONFIG_DIR" "$OMNIBUS_DIR"
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$OMNIBUS_DIR" "$OMNIBUS_PACKAGE_DIR"

GOMODCACHE="$(go env GOMODCACHE)"
export GOMODCACHE

# The omnibus task runs bundler and omnibus with replace_env=True and an allowlist
# (tasks/libs/common/omnibus.py ENV_PASSHTROUGH): RBENV_VERSION, BUNDLE_PATH and SDKROOT never get
# there, and the task sets MACOSX_DEPLOYMENT_TARGET=12.0 itself. What reaches bundler is PATH (the
# system Ruby pin above), HOME, and the working directory: omnibus/.bundle/config (untracked) is
# what pins the gems to ~/.omnibus-gems. Bundler keys that tree by ruby version, not architecture,
# so its native extensions belong to whichever architecture last ran `bundle install` -- see
# references/setup.md before the first arm64 build after an x86_64 one.

echo "=== arch check: uname=$(/usr/bin/uname -m) ruby=$(/usr/bin/ruby -e 'print RUBY_PLATFORM') go=$(go env GOARCH) ==="

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

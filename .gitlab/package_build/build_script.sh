#!/bin/bash -e

# export XCODE_VERSION=15.2
# export XCODE_FULL_VERSION=15.2.0
# export XCODE_VERSION=14.3.1
# export XCODE_FULL_VERSION=14.3.1
export XCODE_VERSION=14.2
export XCODE_FULL_VERSION=14.2

setup_xcode()
{
    (
        cd ~
        echo "=== Setup XCode $XCODE_FULL_VERSION ==="

        # TODO
        # if ! [ -d /Applications/Xcode-${XCODE_FULL_VERSION}.app ]; then
        #     rm -f "Xcode_${XCODE_VERSION}.xip"
        #     aws s3 cp "s3://binaries.ddbuild.io/macos/xcode/Xcode_${XCODE_VERSION}.xip" "Xcode_${XCODE_VERSION}.xip" || true
        #     xcodes install "${XCODE_VERSION}" --experimental-unxip --no-superuser --path "$PWD/Xcode_${XCODE_VERSION}.xip" || true
        # fi

        # TODO: Verify utility
        sudo xcodes select $XCODE_VERSION
        sudo xcodebuild -license accept
        sudo xcodebuild -runFirstLaunch

        # sudo xcodes runtimes install $XCODE_VERSION || true
        echo "=== Ls Xcode Bin ==="
        ls /Applications/Xcode-$XCODE_FULL_VERSION.app/Contents/Developer/usr/bin || true
        echo "=== Ls CLT ==="
        ls /Library/Developer/CommandLineTools/usr/bin || true
        echo "=== Ls SDKs ==="
        ls /Library/Developer/CommandLineTools/SDKs || true
        echo "=== Ls SDKs $XCODE_VERSION ==="
        ls /Library/Developer/CommandLineTools/SDKs/MacOSX$XCODE_FULL_VERSION.sdk || true
        echo "=== SDK settings ==="
        cat /Library/Developer/CommandLineTools/SDKs/MacOSX.sdk/SDKSettings.json || true
        echo "=== Path ==="
        # TODO: Is /Applications/Xcode-15.2.0.app/Contents/Developer
        # TODO A: Add this to path
        # find "$(xcode-select -p)" || true
        xcode-select -p || true
        ls "$(xcode-select -p)" || true
        echo "=== Some other debug ==="
        echo Trying install
        xcode-select --install || true
        echo END DEBUG

    )
}

# TODO A: Remove, this is from the runner
if [ "$1" = SETUP_RUNNER ]; then
    set -euo pipefail

    cd ~

    CMAKE_VERSION=3.22.6
    GIMME_VERSION=1.5.4
    PKG_CONFIG_VERSION=0.29.2
    CI_UPLOADER_VERSION=2.39.0
    CI_UPLOADER_ARM64_SHA=20818eb5dc843d8b87aab98e7fe8f5feb4a108145397b8d07e89afce2ba53b58
    CI_UPLOADER_X64_SHA=7f6ddb731013aba9b7b6d319d89d5f5e6860d0aede6a00d8f244e49d6015ae79
    CODECOV_VERSION=0.6.1
    CODECOV_SHA=62ba56f0f0d62b28e955fcfd4a3524c7c327fcf8f5fcb5124cccf88db358282e

    CI_UPLOADER_FOLDER="/opt/datadog-ci/bin"
    CODECOV_UPLOADER_FOLDER="/opt/codecov/bin"

    ARCH=$(uname -m)
    if [ "$ARCH" = "arm64" ]; then
        CI_UPLOADER_SHA=$CI_UPLOADER_ARM64_SHA
        CI_UPLOADER_BINARY="datadog-ci_darwin-arm64"
    else
        CI_UPLOADER_SHA=$CI_UPLOADER_X64_SHA
        CI_UPLOADER_BINARY="datadog-ci_darwin-x64"
    fi

    # Environment variables passed to the CI
    echo "export DD_ENV=prod" >>~/.zshrc

    # # Custom datadog agent tap
    # brew tap DataDog/datadog-agent-macos-build

    # TODO A: Defined 2 times
    export HOMEBREW_VERSION=4.4.21
    export PKG_CONFIG_VERSION=0.29.2
    export RUBY_VERSION=2.7.4
    export BUNDLER_VERSION=2.3.18
    export PYTHON_VERSION=3.12.6
    export RUST_VERSION=1.74.0
    export RUSTUP_VERSION=1.25.1
    export CMAKE_VERSION=3.30.2
    export GIMME_VERSION=1.5.4
    export GPG_VERSION=1.4.23
    # export CODECOV_VERSION=v0.6.1
    export OPENSSL_VERSION=1.1

    # TODO A: Try block listing?
    # TODO A: Find a better way
    echo Setup env
    mkdir bin
    binaries=("curl" "chmod" "cp" "cut" "date" "mkdir" "readlink" "dirname" "tar" "rm" "mv" "ls" "bash" "make" "xz" "true" "which" "vault" "du" "security" "touch" "cat" "basename" "go" "tr" "uname" "find" "tmutil" "sed" "grep" "git" "tee" "sudo" "xcodes" "xcodebuild" "sort" "uniq" "head")
    for binary in "${binaries[@]}"; do
        echo Using $binary
        ln -s "$(which $binary)" bin/$binary
    done

    # setup_xcode

    echo Setup homebrew
    mkdir homebrew
    curl -L https://github.com/Homebrew/brew/tarball/$HOMEBREW_VERSION | tar xz --strip-components 1 -C homebrew
    # Enable custom env
    export OLDPATH="$PATH"
    export PATH="$PWD/bin"
    eval "$(homebrew/bin/brew shellenv)"
    echo Installed homebrew to "$(brew --prefix)"
    brew update --force
    # TODO A: Necessary?
    chmod -R go-w "$(brew --prefix)/share/zsh"
    brew tap DataDog/datadog-agent-macos-build

    # # TODO A: Add it in the runner
    # echo Install extra packages
    # # TODO: binutils? gmp?
    # extrapackages=(
    #     autoconf@2.72
    #     ca-certificates@2025-02-25
    #     cffi@1.17.1_1
    #     coreutils@9.6
    #     cryptography@44.0.1
    #     gcc@14.2.0_1
    #     gmp@6.3.0
    #     isl@0.27
    #     jq@1.7.1
    #     libmpc@1.3.1
    #     libssh2@1.11.1
    #     libunistring@1.3
    #     libyaml@0.2.5
    #     lz4@1.10.0
    #     m4@1.4.19
    #     mpdecimal@4.0.0
    #     mpfr@4.2.1
    #     oniguruma@6.9.10
    #     pcre2@10.44
    #     pycparser@2.22_1
    #     readline@8.2.13
    #     xz@5.6.2
    #     zstd@1.5.7
    # )
    # for pkg in "${extrapackages[@]}"; do
    #     echo Installing $pkg
    #     brew install $pkg -f || brew install "$(echo $pkg | cut -d@ -f1)" -f || echo "ERROR: Failed to install $pkg"
    # done

    echo Verifying gettext
    echo brew uses gettext --installed
    brew uses gettext --installed || true

    echo brew list --formula --versions
    brew list --formula --versions || true

    echo grep gettext
    brew list --formula --versions | grep gettext || true


    # TODO: xcode select
    echo Install libffi
    brew install libffi -f

    echo Install cmake
    brew install DataDog/datadog-agent-macos-build/cmake@$CMAKE_VERSION -f
    brew link --overwrite cmake@$CMAKE_VERSION

    echo Install pkg-config
    brew install DataDog/datadog-agent-macos-build/pkg-config@$PKG_CONFIG_VERSION -f
    brew link --overwrite pkg-config@$PKG_CONFIG_VERSION

    # TODO A: Not the proper path
    brew install DataDog/datadog-agent-macos-build/gnupg@$GPG_VERSION -f
    brew link --overwrite gnupg@$GPG_VERSION
    # TODO
    echo GPG debug
    # TODO: clean
    # Adding gpgbin to the PATH to be able to call gpg and gpgv
    export PATH="$PWD/homebrew/Cellar/gnupg@$GPG_VERSION/libexec/gpgbin:$PATH"
    export OLDPATH="$PWD/homebrew/Cellar/gnupg@$GPG_VERSION/libexec/gpgbin:$OLDPATH"
    # ls -l "$PWD/homebrew/Cellar/gnupg@$GPG_VERSION/libexec/gpgbin" || true
    # echo "$PWD/homebrew/Cellar/gnupg@$GPG_VERSION/libexec/gpgbin" || true
    # ls "$PWD/homebrew/Cellar/gnupg@$GPG_VERSION" || true

    echo Install openssl
    brew install -v DataDog/datadog-agent-macos-build/openssl@$OPENSSL_VERSION -f
    brew link --overwrite openssl@$OPENSSL_VERSION

    echo Install ruby
    brew install DataDog/datadog-agent-macos-build/ruby@$RUBY_VERSION -f
    brew link --overwrite ruby@$RUBY_VERSION
    gem install bundler -v $BUNDLER_VERSION -f

    echo Install python
    brew install --build-from-source DataDog/datadog-agent-macos-build/python@$PYTHON_VERSION -f
    brew link --overwrite python@$PYTHON_VERSION

    # TODO A: Install in homebrew env
    echo Install rust
    mkdir -p rust/cargo rust/rustup
    export CARGO_HOME="$PWD/rust/cargo"
    export RUSTUP_HOME="$PWD/rust/rustup"
    export RUST_ARCH=x86_64
    if [ "$ARCH" = "arm64" ]; then
        export RUST_ARCH=aarch64
    fi
    curl -sSL -o rustup-init https://static.rust-lang.org/rustup/archive/$RUSTUP_VERSION/$RUST_ARCH-apple-darwin/rustup-init
    chmod +x ./rustup-init
    ./rustup-init -y --profile minimal --default-toolchain $RUST_VERSION
    rm ./rustup-init
    # TODO: Cleanup
    export PATH="$CARGO_HOME/bin:$RUSTUP_HOME/bin:$PATH"
    export OLDPATH="$CARGO_HOME/bin:$RUSTUP_HOME/bin:$OLDPATH"

    echo Install gimme
    brew install DataDog/datadog-agent-macos-build/gimme@$GIMME_VERSION -f
    brew link --overwrite gimme@$GIMME_VERSION

    exit
fi









# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/)
# Copyright 2022-present Datadog, Inc.

# FIXME: Uncomment this once we fix the way we cache the builder setup
# in datadog-agent-macos-build, we have non-critical errors that make
# the script fail with set -e.
# set -e


# Does an omnibus build of the Agent.

# Prerequisites:
# - clone_agent.sh has been run
# - builder_setup.sh has been run
# - $VERSION contains the datadog-agent git ref to target
# - $RELEASE_VERSION contains the release.json version to package. Defaults to $VERSION
# - $AGENT_MAJOR_VERSION contains the major version to release
# - $PYTHON_RUNTIMES contains the included python runtimes
# - $SIGN set to true if signing is enabled
# - if $SIGN is set to true:
#   - $KEYCHAIN_NAME contains the keychain name. Defaults to login.keychain
#   - $KEYCHAIN_PWD contains the keychain password

export RELEASE_VERSION=${RELEASE_VERSION:-$VERSION}
export KEYCHAIN_NAME=${KEYCHAIN_NAME:-"login.keychain"}

# Load build setup vars
source ~/.zshrc

set -x
echo Reset path for custom homebrew

# Homebrew
if [ -n "$CUSTOM_HOMEBREW" ]; then
    paths=("$HOME/bin" "$HOME/homebrew/bin" "$HOME/rust/rustup/bin" "$HOME/rust/cargo/bin" "/Library/Developer/CommandLineTools/usr/bin" "/Applications/Xcode-$XCODE_FULL_VERSION.app/Contents/Developer/usr/bin" "/Applications/Xcode-$XCODE_FULL_VERSION.app/Contents/Developer/CommandLineTools/usr/bin")
    export PATH=
    for path in "${paths[@]}"; do
        export PATH="$PATH:$path"
    done
    # export PATH="$PATH:/opt/codecov/bin:/opt/datadog-ci/bin:/Users/ec2-user/.cargo/bin:/Users/ec2-user/.cargo/bin:/Users/ec2-user/.rbenv/shims:/usr/local/Cellar/pyenv-virtualenv/1.2.4/shims:/Users/ec2-user/.pyenv/shims:/usr/local/bin/:/usr/local/bin:/usr/local/sbin:/usr/local/bin:/System/Cryptexes/App/usr/bin:/usr/bin:/bin:/usr/sbin:/sbin:/Library/Apple/usr/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/local/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/appleinternal/bin"
    export PATH="$PATH:/opt/codecov/bin:/opt/datadog-ci/bin:/Users/ec2-user/.cargo/bin:/Users/ec2-user/.cargo/bin:/usr/local/Cellar/pyenv-virtualenv/1.2.4/shims:/Users/ec2-user/.pyenv/shims:/usr/local/sbin:/System/Cryptexes/App/usr/bin:/usr/bin:/bin:/usr/sbin:/sbin:/Library/Apple/usr/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/local/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/bin:/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/appleinternal/bin" # Removed /usr/local/bin, /Users/ec2-user/.rbenv/shims
    echo "Initial PATH: $PATH"
fi

# Python
echo Setting up venv
python3 -m venv .venv
source .venv/bin/activate
python3 -m pip install -r requirements.txt -r tasks/requirements.txt
echo Python version
python3 --version

# Go
export GO_VERSION="$(cat .go-version)"
eval "$(gimme $GO_VERSION)"
echo "GOROOT: $GOROOT"
echo "GOPATH: $GOPATH"
export GOPATH="$GOROOT" # TODO
export PATH="$PATH:$GOPATH/bin"
echo Go version should be $GO_VERSION
go version

# Debug rust
echo Rust version
rustup --version || true
cargo --version || true

# Xcode
# sudo xcodebuild -license accept
# # TODO A: sudo xcodes select 15.2
# sudo xcodes select 14.2

# setup_xcode
# ls "/Applications/Xcode-15.2.0.app/Contents/Developer"
# ls "/Applications/Xcode-15.2.0.app/Contents/Developer/CommandLineTools"
# ls "/Applications/Xcode-15.2.0.app/Contents/Developer/CommandLineTools/usr/bin"

echo "Full PATH: $PATH"
set +x


# Clean up previous builds
# TODO: rm?
# sudo rm -rf /opt/datadog-agent ./vendor ./vendor-new /var/cache/omnibus/src/* ./omnibus/Gemfile.lock
rm -rf ./opt/datadog-agent ./vendor ./vendor-new ./var/cache/omnibus/src/* ./omnibus/Gemfile.lock

# Create target folders
mkdir -p ./opt/datadog-agent ./var/cache/omnibus # TODO: && chown "$USER" ./opt/datadog-agent ./var/cache/omnibus ? -> seems to break runners

# sudo mkdir -p ./opt/datadog-agent ./var/cache/omnibus && sudo chown "$USER" ./opt/datadog-agent ./var/cache/omnibus

# Set bundler install path to cached folder
pushd omnibus && bundle config set --local path 'vendor/bundle' && popd

inv check-go-version || exit 1

# Update the INTEGRATION_CORE_VERSION if requested
if [ -n "$INTEGRATIONS_CORE_REF" ]; then
    export INTEGRATIONS_CORE_VERSION="$INTEGRATIONS_CORE_REF"
fi

INVOKE_TASK="omnibus.build"
if ! inv --list | grep -qF "$INVOKE_TASK"; then
    echo -e "\033[0;31magent.omnibus-build is deprecated. Please use omnibus.build!\033[0m"
    INVOKE_TASK="agent.omnibus-build"
fi

echo
echo "--- CC ---"
echo "=> ls /usr/local/opt/gettext"
ls /usr/local/opt/gettext || true
echo "=> END ls /usr/local/opt/gettext"
# echo "Old install dir: $INSTALL_DIR"
# echo "Old config dir: $CONFIG_DIR"
export INSTALL_DIR="$PWD/datadog-agent-build/bin"
export CONFIG_DIR="$PWD/datadog-agent-build/config"

# Launch omnibus build
if [ "$SIGN" = "true" ]; then
    # Unlock the keychain to get access to the signing certificates
    security unlock-keychain -p "$KEYCHAIN_PWD" "$KEYCHAIN_NAME"
    inv -e $INVOKE_TASK --hardened-runtime --major-version "$AGENT_MAJOR_VERSION" --release-version "$RELEASE_VERSION" --config-directory "$CONFIG_DIR" --install-directory "$INSTALL_DIR" --base-dir "$OMNIBUS_DIR" || exit 1
    # Lock the keychain once we're done
    security lock-keychain "$KEYCHAIN_NAME"
else
    inv -e $INVOKE_TASK --skip-sign --major-version "$AGENT_MAJOR_VERSION" --release-version "$RELEASE_VERSION" --config-directory "$CONFIG_DIR" --install-directory "$INSTALL_DIR" --base-dir "$OMNIBUS_DIR" || exit 1
fi

echo ls -la "$INSTALL_DIR"
ls -la "$INSTALL_DIR"
echo
echo ls -la "$CONFIG_DIR"
ls -la "$CONFIG_DIR"

echo "--- CC END ---"

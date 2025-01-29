#!/bin/bash

# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/)
# Copyright 2022-present Datadog, Inc.

set -e

# Setups a MacOS builder that can do unsigned builds of the MacOS Agent.
# The .build_setup file is populated with the correct envvar definitions to do the build,
# which are then used by the build script.

# Prerequisites:
# - A MacOS 10.13.6 (High Sierra) box
# - clone_agent.sh has been run


# About brew packages:
# We use a custom homebrew tap (DataDog/datadog-agent-macos-build, hosted in https://github.com/DataDog/homebrew-datadog-agent-macos-build)
# to keep pinned versions of the software we need.

# How to update a version of a brew package:
# 1. See the instructions of the DataDog/homebrew-datadog-agent-macos-build repo
#    to add a formula for the new version you want to use.
# 2. Update here the version of the formula to use.

source ~/.build_setup

export PKG_CONFIG_VERSION=0.29.2
export RUBY_VERSION=2.7.4
export BUNDLER_VERSION=2.3.18
export PYTHON_VERSION=3.12.6
export RUST_VERSION=1.74.0
export RUSTUP_VERSION=1.25.1
export CMAKE_VERSION=3.30.2
export GIMME_VERSION=1.5.4
export GPG_VERSION=1.4.23
export CODECOV_VERSION=v0.6.1
export OPENSSL_VERSION=1.1

export GO_VERSION=$(cat .go-version)

# Helper to run a bash command with retries, with an exponential backoff.
# Returns 1 if the provided command fails every time, 0 otherwise.
function do_with_retries() {
    local command="$1"
    local retries="$2"
    local res=0

    for i in $(seq 0 $retries); do
        res=0
        sleep $((2**$i))
        /bin/bash -c "$command" && break || res=1
    done
    return $res
}

# Install or upgrade brew (will also install Command Line Tools)

# NOTE: The macOS runner has HOMEBREW_NO_INSTALL_FROM_API set, which makes it
# try to clone homebrew-core. At one point, cloning of homebrew-core started
# returning the following error for us in about 50 % of cases:
#     remote: fatal: object 80a071c049c4f2e465e0b1c40cfc6325005ab05b cannot be read
#     remote: aborting due to possible repository corruption on the remote side.
# Unsetting HOMEBREW_NO_INSTALL_FROM_API makes brew use formulas from
# https://formulae.brew.sh/, thus avoiding cloning the repository, hence
# avoiding the error.
brew untap --force homebrew/cask
rm -rf /usr/local/Homebrew/Library/Taps/homebrew/homebrew-core

do_with_retries "CI=1; unset HOMEBREW_NO_INSTALL_FROM_API; $(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install.sh)" 5

# Add our custom repository
echo TAP
brew tap
brew tap --force DataDog/datadog-agent-macos-build
# brew tap DataDog/datadog-agent-macos-build
echo END

brew uninstall python@2 -f || true # Uninstall python 2 if present
brew uninstall python -f || true # Uninstall python 3 if present

# Install cmake
brew install DataDog/datadog-agent-macos-build/cmake@$CMAKE_VERSION -f
brew link --overwrite cmake@$CMAKE_VERSION

# Install pkg-config
brew install DataDog/datadog-agent-macos-build/pkg-config@$PKG_CONFIG_VERSION -f
brew link --overwrite pkg-config@$PKG_CONFIG_VERSION

# Install gpg (depends on pkg-config)
brew install DataDog/datadog-agent-macos-build/gnupg@$GPG_VERSION -f
brew link --overwrite gnupg@$GPG_VERSION
# Adding gpgbin to the PATH to be able to call gpg and gpgv
export PATH="/usr/local/opt/gnupg@1.4.23/libexec/gpgbin:$PATH"

# Install codecov
curl https://uploader.codecov.io/verification.gpg | gpg --no-default-keyring --keyring trustedkeys.gpg --import
curl -Os https://uploader.codecov.io/$CODECOV_VERSION/macos/codecov
curl -Os https://uploader.codecov.io/$CODECOV_VERSION/macos/codecov.SHA256SUM
curl -Os https://uploader.codecov.io/$CODECOV_VERSION/macos/codecov.SHA256SUM.sig
gpgv codecov.SHA256SUM.sig codecov.SHA256SUM
shasum -a 256 -c codecov.SHA256SUM
rm codecov.SHA256SUM.sig codecov.SHA256SUM
mv codecov /usr/local/bin/codecov
chmod +x /usr/local/bin/codecov

# Install openssl
# Homebrew disabled the ability to install openssl@1.1
# so we need to install it from our tap
brew install DataDog/datadog-agent-macos-build/openssl@${OPENSSL_VERSION} -f
brew link --overwrite openssl@${OPENSSL_VERSION}

# Install ruby (depends on pkg-config)
brew install DataDog/datadog-agent-macos-build/ruby@$RUBY_VERSION -f
brew link --overwrite ruby@$RUBY_VERSION

gem install bundler -v $BUNDLER_VERSION -f

# Install python
# "brew link --overwrite" will refuse to overwrite links it doesn't own,
# so we have to make sure these don't exist
# see: https://github.com/actions/setup-python/issues/577
rm -f /usr/local/bin/2to3* \
      /usr/local/bin/idle3* \
      /usr/local/bin/pydoc3* \
      /usr/local/bin/python3* \
      /usr/local/bin/python3*-config
brew install --build-from-source DataDog/datadog-agent-macos-build/python@$PYTHON_VERSION -f
brew link --overwrite python@$PYTHON_VERSION
# Put homebrew Python ahead of system Python
echo 'export PATH="/usr/local/opt/python@'"${PYTHON_VERSION}"'/libexec/bin:$PATH"' >> ~/.build_setup

# Install rust
# Rust may be needed to compile some python libs
curl -sSL -o rustup-init https://static.rust-lang.org/rustup/archive/${RUSTUP_VERSION}/x86_64-apple-darwin/rustup-init \
    && chmod +x ./rustup-init \
    && ./rustup-init -y --profile minimal --default-toolchain ${RUST_VERSION} \
    && rm ./rustup-init

# Install gimme
brew install DataDog/datadog-agent-macos-build/gimme@$GIMME_VERSION -f
brew link --overwrite gimme@$GIMME_VERSION
eval `gimme $GO_VERSION`
echo 'eval `gimme '$GO_VERSION'`' >> ~/.build_setup

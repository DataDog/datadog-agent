#!/bin/zsh
source ~/.zshrc
set -euxo pipefail

BUNDLER_VERSION=2.3.18
CI_UPLOADER_ARM64_SHA=20818eb5dc843d8b87aab98e7fe8f5feb4a108145397b8d07e89afce2ba53b58
CI_UPLOADER_VERSION=2.39.0
CI_UPLOADER_X64_SHA=7f6ddb731013aba9b7b6d319d89d5f5e6860d0aede6a00d8f244e49d6015ae79
CMAKE_VERSION=3.30.2
CODECOV_SHA=62ba56f0f0d62b28e955fcfd4a3524c7c327fcf8f5fcb5124cccf88db358282e
CODECOV_VERSION=0.6.1
GIMME_VERSION=1.5.4
GPG_VERSION=1.4.23
OPENSSL_VERSION=1.1
PKG_CONFIG_VERSION=0.29.2
PYTHON_VERSION=3.12.6
RUBY_VERSION=2.7.4
RUSTUP_VERSION=1.25.1
RUST_VERSION=1.74.0

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

# Helper to run a bash command with retries, with an exponential backoff.
# Returns 1 if the provided command fails every time, 0 otherwise.
function do_with_retries() {
    local command="$1"
    local retries="$2"
    local res=0

    for i in $(seq 0 $retries); do
        res=0
        sleep $((2 ** i))
        /bin/bash -c "$command" && break || res=1
    done
    return $res
}

# The base box ships a few things that can have unwanted effects on the MacOS build.
# For instance, we compile Python in the pipeline. If Python finds some libraries while
# it's being compiled, then it will add a dynamic link to them and add some features.
# In this particular case, Python sees that there is libintl.8.dylib (provided by the gettext brew package)
# in the default include path, thus links to it. However, that's not something we need, so we don't actually
# ship that library in the MacOS package. Since we have a feature to make a build fail if we depend on
# something we don't ship, this made the build fail (see: https://github.com/DataDog/datadog-agent-macos-build/runs/1011733463?check_suite_focus=true).
# In order to avoid such cases in the future where we use things we didn't expect to, we'd rather
# start with a "clean" runner with the bare minimum, and only install the brew packages we require.
echo 'Removing preinstalled environment...'
for dependency in $(brew list --formula); do
    brew remove --force --ignore-dependencies --formula $dependency || echo "Warning: $dependency could not be removed"
done
# TODO A
# # Also completely remove the ruby env, otherwise some files remain after the formula uninstall,
# # possibly causing gem version mismatch issues (eg. bundler).
# rm -rf /usr/local/lib/ruby

# Install or upgrade brew (will also install Command Line Tools)
# NOTE: The macOS runner has HOMEBREW_NO_INSTALL_FROM_API set, which makes it
# try to clone homebrew-core. At one point, cloning of homebrew-core started
# returning the following error for us in about 50 % of cases:
#     remote: fatal: object 80a071c049c4f2e465e0b1c40cfc6325005ab05b cannot be read
#     remote: aborting due to possible repository corruption on the remote side.
# Unsetting HOMEBREW_NO_INSTALL_FROM_API makes brew use formulas from
# https://formulae.brew.sh/, thus avoiding cloning the repository, hence
# avoiding the error.
# TODO A
# echo 'Installing / upgrading brew...'
# brew untap --force homebrew/cask || true
# sudo rm -rf /usr/local/Homebrew/Library/Taps/homebrew/homebrew-core
# do_with_retries "CI=1; unset HOMEBREW_NO_INSTALL_FROM_API; $(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install.sh)" 5

# Environment variables passed to the CI
echo "export DD_ENV=prod" >>~/.zshrc

# Custom datadog agent tap
brew tap DataDog/datadog-agent-macos-build

echo 'Installing cmake...'
brew install DataDog/datadog-agent-macos-build/cmake@$CMAKE_VERSION -f
brew link --overwrite cmake@$CMAKE_VERSION

echo 'Installing pkg-config...'
brew install DataDog/datadog-agent-macos-build/pkg-config@$PKG_CONFIG_VERSION -f
brew link --overwrite pkg-config@$PKG_CONFIG_VERSION

echo 'Installing gpg...'
brew install DataDog/datadog-agent-macos-build/gnupg@$GPG_VERSION -f
brew link --overwrite gnupg@$GPG_VERSION
# Adding gpgbin to the PATH to be able to call gpg and gpgv
echo 'export PATH="/usr/local/opt/gnupg@1.4.23/libexec/gpgbin:$PATH"' >>~/.zshrc

# Homebrew disabled the ability to install openssl@1.1
# so we need to install it from our tap
echo 'Installing openssl...'
brew install DataDog/datadog-agent-macos-build/openssl@$OPENSSL_VERSION -f
brew link --overwrite openssl@$OPENSSL_VERSION

# TODO A
# echo 'Installing ruby...'
# brew install DataDog/datadog-agent-macos-build/ruby@$RUBY_VERSION -f
# brew link --overwrite ruby@$RUBY_VERSION
# gem install bundler -v $BUNDLER_VERSION -f

# "--overwrite" will refuse to overwrite links it doesn't own,
# so we have to make sure these don't exist
# see: https://github.com/actions/setup-python/issues/577
echo 'Installing python...'
# Remove existing Python installation as it may otherwise interfere
brew uninstall python@2 -f || true # Uninstall python 2 if present
brew uninstall python -f || true   # Uninstall python 3 if present
# sudo rm -rf /Library/Frameworks/Python.framework/Versions/*
sudo rm -f /usr/local/bin/2to3* \
    /usr/local/bin/idle3* \
    /usr/local/bin/pydoc3* \
    /usr/local/bin/python3* \
    /usr/local/bin/python3*-config \
    /usr/local/Cellar/python@*
brew install --build-from-source DataDog/datadog-agent-macos-build/python@$PYTHON_VERSION -f
brew link --overwrite python@$PYTHON_VERSION
# Put homebrew Python ahead of system Python
echo 'export PATH="/usr/local/opt/python@'"$PYTHON_VERSION"'/libexec/bin:$PATH"' >>~/.zshrc

# TODO A
# # Rust may be needed to compile some python libs
# echo 'Installing rust...'
# curl -sSL -o rustup-init https://static.rust-lang.org/rustup/archive/$RUSTUP_VERSION/x86_64-apple-darwin/rustup-init &&
#     chmod +x ./rustup-init &&
#     ./rustup-init -y --profile minimal --default-toolchain $RUST_VERSION &&
#     rm ./rustup-init

echo 'Installing gimme...'
brew install DataDog/datadog-agent-macos-build/gimme@$GIMME_VERSION -f
brew link --overwrite gimme@$GIMME_VERSION
eval "$(gimme $GO_VERSION)"

# echo 'Installing datadog-ci...'
# sudo mkdir -p ${CI_UPLOADER_FOLDER}
# sudo chown -R ec2-user ${CI_UPLOADER_FOLDER}
# curl -fsSL https://github.com/DataDog/datadog-ci/releases/download/v${CI_UPLOADER_VERSION}/${CI_UPLOADER_BINARY} --output "${CI_UPLOADER_FOLDER}/datadog-ci"
# echo "${CI_UPLOADER_SHA} *${CI_UPLOADER_FOLDER}/datadog-ci" | shasum -a 256 --check
# chmod +x ${CI_UPLOADER_FOLDER}/datadog-ci
# echo "export PATH=\"${CI_UPLOADER_FOLDER}:\$PATH\"" >>~/.zshrc

# # Codecov uploader is only released on x86_64 macOS
# if [ "$ARCH" = "x86_64" ]; then
#     echo 'Installing Codecov uploader...'
#     sudo mkdir -p ${CODECOV_UPLOADER_FOLDER}
#     sudo chown -R ec2-user ${CODECOV_UPLOADER_FOLDER}
#     curl -fsSL https://uploader.codecov.io/v${CODECOV_VERSION}/macos/codecov --output "${CODECOV_UPLOADER_FOLDER}/codecov"
#     echo "${CODECOV_SHA} *${CODECOV_UPLOADER_FOLDER}/codecov" | shasum -a 256 --check
#     chmod +x ${CODECOV_UPLOADER_FOLDER}/codecov
#     echo "export PATH=\"${CODECOV_UPLOADER_FOLDER}:\$PATH\"" >>~/.zshrc
# fi







# #!/bin/bash

# # Unless explicitly stated otherwise all files in this repository are licensed
# # under the Apache License Version 2.0.
# # This product includes software developed at Datadog (https://www.datadoghq.com/)
# # Copyright 2022-present Datadog, Inc.

# set -e

# # Setups a MacOS builder that can do unsigned builds of the MacOS Agent.
# # The .build_setup file is populated with the correct envvar definitions to do the build,
# # which are then used by the build script.

# # Prerequisites:
# # - A MacOS 10.13.6 (High Sierra) box
# # - clone_agent.sh has been run


# # About brew packages:
# # We use a custom homebrew tap (DataDog/datadog-agent-macos-build, hosted in https://github.com/DataDog/homebrew-datadog-agent-macos-build)
# # to keep pinned versions of the software we need.

# # How to update a version of a brew package:
# # 1. See the instructions of the DataDog/homebrew-datadog-agent-macos-build repo
# #    to add a formula for the new version you want to use.
# # 2. Update here the version of the formula to use.

# source ~/.build_setup

# export PKG_CONFIG_VERSION=0.29.2
# export RUBY_VERSION=2.7.4
# export BUNDLER_VERSION=2.3.18
# export PYTHON_VERSION=3.12.6
# export RUST_VERSION=1.74.0
# export RUSTUP_VERSION=1.25.1
# export CMAKE_VERSION=3.30.2
# export GIMME_VERSION=1.5.4
# export GPG_VERSION=1.4.23
# export CODECOV_VERSION=v0.6.1
# export OPENSSL_VERSION=1.1

# export GO_VERSION=$(cat .go-version)

# # Helper to run a bash command with retries, with an exponential backoff.
# # Returns 1 if the provided command fails every time, 0 otherwise.
# function do_with_retries() {
#     local command="$1"
#     local retries="$2"
#     local res=0

#     for i in $(seq 0 $retries); do
#         res=0
#         sleep $((2**$i))
#         /bin/bash -c "$command" && break || res=1
#     done
#     return $res
# }

# # Install or upgrade brew (will also install Command Line Tools)

# # TODO
# # # NOTE: The macOS runner has HOMEBREW_NO_INSTALL_FROM_API set, which makes it
# # # try to clone homebrew-core. At one point, cloning of homebrew-core started
# # # returning the following error for us in about 50 % of cases:
# # #     remote: fatal: object 80a071c049c4f2e465e0b1c40cfc6325005ab05b cannot be read
# # #     remote: aborting due to possible repository corruption on the remote side.
# # # Unsetting HOMEBREW_NO_INSTALL_FROM_API makes brew use formulas from
# # # https://formulae.brew.sh/, thus avoiding cloning the repository, hence
# # # avoiding the error.
# # brew untap --force homebrew/cask
# # TODO: Need this?
# # rm -rf /usr/local/Homebrew/Library/Taps/homebrew/homebrew-core

# do_with_retries "CI=1; unset HOMEBREW_NO_INSTALL_FROM_API; $(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/master/install.sh)" 5

# # Add our custom repository
# brew tap DataDog/datadog-agent-macos-build

# brew uninstall python@2 -f || true # Uninstall python 2 if present
# brew uninstall python -f || true # Uninstall python 3 if present

# # Install cmake
# brew install DataDog/datadog-agent-macos-build/cmake@$CMAKE_VERSION -f
# brew link --overwrite cmake@$CMAKE_VERSION

# # Install pkg-config
# brew install DataDog/datadog-agent-macos-build/pkg-config@$PKG_CONFIG_VERSION -f
# brew link --overwrite pkg-config@$PKG_CONFIG_VERSION

# # Install gpg (depends on pkg-config)
# brew install DataDog/datadog-agent-macos-build/gnupg@$GPG_VERSION -f
# brew link --overwrite gnupg@$GPG_VERSION
# # Adding gpgbin to the PATH to be able to call gpg and gpgv
# export PATH="/usr/local/opt/gnupg@1.4.23/libexec/gpgbin:$PATH"

# # Install codecov
# curl https://uploader.codecov.io/verification.gpg | gpg --no-default-keyring --keyring trustedkeys.gpg --import
# curl -Os https://uploader.codecov.io/$CODECOV_VERSION/macos/codecov
# curl -Os https://uploader.codecov.io/$CODECOV_VERSION/macos/codecov.SHA256SUM
# curl -Os https://uploader.codecov.io/$CODECOV_VERSION/macos/codecov.SHA256SUM.sig
# gpgv codecov.SHA256SUM.sig codecov.SHA256SUM
# shasum -a 256 -c codecov.SHA256SUM
# rm codecov.SHA256SUM.sig codecov.SHA256SUM
# mv codecov /usr/local/bin/codecov
# chmod +x /usr/local/bin/codecov

# # Install openssl
# # Homebrew disabled the ability to install openssl@1.1
# # so we need to install it from our tap
# brew install DataDog/datadog-agent-macos-build/openssl@${OPENSSL_VERSION} -f
# brew link --overwrite openssl@${OPENSSL_VERSION}

# # Install ruby (depends on pkg-config)
# brew install DataDog/datadog-agent-macos-build/ruby@$RUBY_VERSION -f
# brew link --overwrite ruby@$RUBY_VERSION

# gem install bundler -v $BUNDLER_VERSION -f

# # Install python
# # "brew link --overwrite" will refuse to overwrite links it doesn't own,
# # so we have to make sure these don't exist
# # see: https://github.com/actions/setup-python/issues/577
# rm -f /usr/local/bin/2to3* \
#       /usr/local/bin/idle3* \
#       /usr/local/bin/pydoc3* \
#       /usr/local/bin/python3* \
#       /usr/local/bin/python3*-config
# brew install --build-from-source DataDog/datadog-agent-macos-build/python@$PYTHON_VERSION -f
# brew link --overwrite python@$PYTHON_VERSION
# # Put homebrew Python ahead of system Python
# echo 'export PATH="/usr/local/opt/python@'"${PYTHON_VERSION}"'/libexec/bin:$PATH"' >> ~/.build_setup

# # Install rust
# # Rust may be needed to compile some python libs
# curl -sSL -o rustup-init https://static.rust-lang.org/rustup/archive/${RUSTUP_VERSION}/x86_64-apple-darwin/rustup-init \
#     && chmod +x ./rustup-init \
#     && ./rustup-init -y --profile minimal --default-toolchain ${RUST_VERSION} \
#     && rm ./rustup-init

# # Install gimme
# brew install DataDog/datadog-agent-macos-build/gimme@$GIMME_VERSION -f
# brew link --overwrite gimme@$GIMME_VERSION
# eval `gimme $GO_VERSION`
# echo 'eval `gimme '$GO_VERSION'`' >> ~/.build_setup

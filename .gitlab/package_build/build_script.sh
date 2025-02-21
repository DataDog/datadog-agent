#!/bin/bash -e

# new-agent_dmg-x64-a7:
#   stage: package_build
#   # extends: .macos_gitlab
#   tags: ["macos:ventura-amd64-test", "specific:true"]
#   # tags: ["macos:ventura-arm64-test", "specific:true"] # TODO
#   needs: []
#   timeout: 6h
#   artifacts:
#     expire_in: 2 weeks
#     paths:
#       # - $OMNIBUS_PACKAGE_DIR
#       - /tmp/celian
# !reference [.vault_login]

# Setup
# TODO A: Omnibus cache
unset INTEGRATION_WHEELS_CACHE_BUCKET || true
unset INTEGRATION_WHEELS_SKIP_CACHE_UPLOAD || true
unset S3_OMNIBUS_CACHE_BUCKET || true
unset S3_OMNIBUS_CACHE_ANONYMOUS_ACCESS || true
# export INTEGRATION_WHEELS_CACHE_BUCKET=dd-agent-omnibus
# export INTEGRATION_WHEELS_SKIP_CACHE_UPLOAD="true"
# export S3_OMNIBUS_CACHE_BUCKET="dd-ci-datadog-agent-omnibus-cache-build-stable"
# export S3_OMNIBUS_CACHE_ANONYMOUS_ACCESS="true"
export RELEASE_VERSION=$RELEASE_VERSION_7
export AGENT_MAJOR_VERSION=7
export PYTHON_RUNTIMES=3
export INSTALL_DIR=/tmp/celian/bin
export CONFIG_DIR=/tmp/celian/config
export GOPATH="$GOROOT"
export OMNIBUS_DIR=omnibus_build
rm -f ~/.build_setup; touch ~/.build_setup
rm -rf "$INSTALL_DIR" "$CONFIG_DIR"
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR"
# Omnibus TODO: Useless?
rm -rf "$OMNIBUS_DIR" && mkdir -p "$OMNIBUS_DIR"
echo Ignoring omnibus build cache
# TODO: Omnibus cache
# rm -rf _omnibus_cache_key_files && mkdir -p _omnibus_cache_key_files
# cp ./{release.json,omnibus/Gemfile} _omnibus_cache_key_files
# TODO: Cache go?
mkdir -p $HOME/go
echo 'export GOPATH=$HOME/go' >> ~/.build_setup
echo 'export PATH="$GOPATH/bin:$PATH"' >> ~/.build_setup
export GO_VERSION="$(cat .go-version)"
eval "$(gimme $GO_VERSION)"
. ~/.build_setup
# TODO: xcode 14.2?
# - xcode-select -s /Applications/Xcode_14.3.1.app
# TODO: Cache brew deps
# TODO: Cache _omnibus_cache_key
# TODO: Verify runner - bash .gitlab/package_build/builder_setup.sh
# TODO: Add certificates to the keychain
# TODO: Cache ruby deps
# TODO: Cache...
# - |
#   export GOMODCACHE=~/gomodcache
#   if [ "${USE_CACHING_PROXY_RUBY}" = "true" ]; then export BUNDLE_MIRROR__RUBYGEMS__ORG=https://${ARTIFACTORY_USERNAME}:${ARTIFACTORY_TOKEN}@${ARTIFACTORY_URL}/${ARTIFACTORY_GEMS_PATH}; fi
#   if [ "${USE_CACHING_PROXY_PYTHON}" = "true" ]; then export PIP_INDEX_URL=https://${ARTIFACTORY_USERNAME}:${ARTIFACTORY_TOKEN}@${ARTIFACTORY_URL}/${ARTIFACTORY_PYPI_PATH}; fi
#   mkdir -p $GOMODCACHE


# -------------- Job ----------------

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
source ~/.build_setup

# Install python deps (invoke, etc.)

if [ -d .venv ]; then
    python3 -m venv .venv
fi
source .venv/bin/activate
python3 -m pip install -r requirements.txt

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
# TODO: Remove, do it inside a build dir
rm -rf /tmp/celian
mkdir -p /tmp/celian/bin /tmp/celian/config
# echo "Old install dir: $INSTALL_DIR"
# echo "Old config dir: $CONFIG_DIR"
export INSTALL_DIR=/tmp/celian/bin
export CONFIG_DIR=/tmp/celian/config

# Launch omnibus build
# if [ "$SIGN" = "true" ]; then
# TODO
if false; then
    echo SIGNING

    # Unlock the keychain to get access to the signing certificates
    security unlock-keychain -p "$KEYCHAIN_PWD" "$KEYCHAIN_NAME"
    inv -e $INVOKE_TASK --hardened-runtime --major-version "$AGENT_MAJOR_VERSION" --release-version "$RELEASE_VERSION" --config-directory "$CONFIG_DIR" --install-directory "$INSTALL_DIR" --base-dir "$OMNIBUS_DIR" || exit 1
    # Lock the keychain once we're done
    security lock-keychain "$KEYCHAIN_NAME"
else
    echo NOT SIGNING

    inv -e $INVOKE_TASK --skip-sign --major-version "$AGENT_MAJOR_VERSION" --release-version "$RELEASE_VERSION" --config-directory "$CONFIG_DIR" --install-directory "$INSTALL_DIR" --base-dir "$OMNIBUS_DIR" || exit 1
fi

echo ls -la /tmp/celian/bin
ls -la /tmp/celian/bin
echo ls -la /tmp/celian/config
ls -la /tmp/celian/config
echo du /tmp/celian
du /tmp/celian

echo "--- CC END ---"

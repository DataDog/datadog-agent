#!/bin/bash

set -eo pipefail

if [ "$SIGN" = true ]; then
    echo "Signing enabled"
else
    echo "Signing disabled"
fi

# --- Setup environment ---
unset OMNIBUS_GIT_CACHE_DIR
unset OMNIBUS_BASE_DIR
export INSTALL_DIR="$PWD/datadog-agent-build/bin"
export CONFIG_DIR="$PWD/datadog-agent-build/config"
export OMNIBUS_DIR="$PWD/omnibus_build"
export OMNIBUS_PACKAGE_DIR="$PWD"/omnibus/pkg

# Update the INTEGRATION_CORE_VERSION if requested
if [ -n "$INTEGRATIONS_CORE_REF" ]; then
    export INTEGRATIONS_CORE_VERSION="$INTEGRATIONS_CORE_REF"
fi

# --- Setup Go ---
echo Setting up Go
mkdir -p ~/go
export GO_VERSION="$(cat .go-version)"
eval "$(gimme $GO_VERSION)"
export PATH="$PATH:$GOROOT/bin"
echo Go version should be $GO_VERSION
go version
dda inv check-go-version

# --- Setup signing ---
if [ "$SIGN" = true ]; then
    # Add certificates to temporary keychain
    echo "Setting up signing secrets"

    KEYCHAIN_PWD=$($CI_PROJECT_DIR/tools/ci/fetch_secret.sh $MACOS_KEYCHAIN_PWD password) || exit $?; export KEYCHAIN_PWD
    CODESIGNING_CERT_BASE64=$($CI_PROJECT_DIR/tools/ci/fetch_secret.sh $MACOS_APPLE_APPLICATION_SIGNING certificate) || exit $?; export CODESIGNING_CERT_BASE64
    CODESIGNING_CERT_PASSPHRASE=$($CI_PROJECT_DIR/tools/ci/fetch_secret.sh $MACOS_APPLE_APPLICATION_SIGNING passphrase) || exit $?; export CODESIGNING_CERT_PASSPHRASE
    INSTALLER_CERT_BASE64=$($CI_PROJECT_DIR/tools/ci/fetch_secret.sh $MACOS_APPLE_INSTALLER_SIGNING certificate) || exit $?; export INSTALLER_CERT_BASE64
    INSTALLER_CERT_PASSPHRASE=$($CI_PROJECT_DIR/tools/ci/fetch_secret.sh $MACOS_APPLE_INSTALLER_SIGNING passphrase) || exit $?; export INSTALLER_CERT_PASSPHRASE

    NOTARIZATION_PWD=$($CI_PROJECT_DIR/tools/ci/fetch_secret.sh $MACOS_APPLE_DEVELOPER_ACCOUNT notarization-password) || exit $?; export NOTARIZATION_PWD
    TEAM_ID=$($CI_PROJECT_DIR/tools/ci/fetch_secret.sh $MACOS_APPLE_DEVELOPER_ACCOUNT team-id) || exit $?; export TEAM_ID
    APPLE_ACCOUNT=$($CI_PROJECT_DIR/tools/ci/fetch_secret.sh $MACOS_APPLE_DEVELOPER_ACCOUNT user) || exit $?; export APPLE_ACCOUNT

    # Create temporary build keychain
    security create-keychain -p "$KEYCHAIN_PWD" "$KEYCHAIN_NAME"

    # Let the keychain stay unlocked for 1 hour, otherwise the OS might lock
    # it again after a period of inactivity.
    security set-keychain-settings -lut 3600 "$KEYCHAIN_NAME"

    # Add the build keychain to the list of active keychains
    security list-keychains -d user -s "$KEYCHAIN_NAME" "login.keychain"

    security unlock-keychain -p "$KEYCHAIN_PWD" "$KEYCHAIN_NAME"

    # Apple has two different kinds of certificates:
    # - code signing certificates, to sign binaries.
    # - installer certificates, to sign the .pkg archive.
    # We use both, because having signed binaries & a signed installer is a prerequisite to
    # have an app notarized by Apple.
    echo "$CODESIGNING_CERT_BASE64" | base64 -d > codesigning_cert.p12
    echo "$INSTALLER_CERT_BASE64" | base64 -d > installer_cert.p12

    # Import codesigning cert, only allow codesign to use it without confirmation
    echo Importing codesigning cert
    security import codesigning_cert.p12 -f pkcs12 -P "$CODESIGNING_CERT_PASSPHRASE" -k "build.keychain" -T "/usr/bin/codesign"
    rm -f codesigning_cert.p12

    # Import installer cert, only allow productbuild to use it without confirmation
    echo Importing installer cert
    security import installer_cert.p12 -f pkcs12 -P "$INSTALLER_CERT_PASSPHRASE" -k "build.keychain" -T "/usr/bin/productbuild"
    rm -f installer_cert.p12

    # Update the key partition list
    # Since MacOS Sierra, this line is needed to "apply" the security import changes above
    # (namely the changes that allow using codesign and productbuild without user prompts)
    # See: https://stackoverflow.com/questions/39868578/security-codesign-in-sierra-keychain-ignores-access-control-settings-and-ui-p
    #      https://stackoverflow.com/questions/43002579/after-set-key-partition-list-codesign-still-prompts-for-key-access/43002580
    # for reference.
    # Note: this feature is badly documented (and doesn't even appear in the command list if you run security --help...).
    # Note: we silence the output of this command because it contains metadata about the certificates.
    security set-key-partition-list -S apple-tool:,apple: -s -k "$KEYCHAIN_PWD" "$KEYCHAIN_NAME" &>/dev/null
fi

# --- Build ---
echo Launching omnibus build
rm -rf "$INSTALL_DIR" "$CONFIG_DIR"
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR"
rm -rf "$OMNIBUS_DIR" && mkdir -p "$OMNIBUS_DIR"
if [ "$SIGN" = "true" ]; then
    # Unlock the keychain to get access to the signing certificates
    security unlock-keychain -p "$KEYCHAIN_PWD" "$KEYCHAIN_NAME"
    dda inv -- -e omnibus.build --hardened-runtime --release-version "$RELEASE_VERSION" --config-directory "$CONFIG_DIR" --install-directory "$INSTALL_DIR" --base-dir "$OMNIBUS_DIR" || exit 1
    # Lock the keychain once we're done
    security lock-keychain "$KEYCHAIN_NAME"
else
    dda inv -- -e omnibus.build --skip-sign --release-version "$RELEASE_VERSION" --config-directory "$CONFIG_DIR" --install-directory "$INSTALL_DIR" --base-dir "$OMNIBUS_DIR" || exit 1
fi
echo Built packages using omnibus

# --- Notarization ---
if [ "$SIGN" = true ]; then
    echo -e "\e[0Ksection_start:`date +%s`:notarization\r\e[0KDoing notarization"
    export RELEASE_VERSION=${RELEASE_VERSION:-$VERSION}
    unset LATEST_DMG

    # Find latest .dmg file in $GOPATH/src/github.com/Datadog/datadog-agent/omnibus/pkg
    for file in "$PWD/omnibus/pkg"/*.dmg; do
    if [[ -z "$LATEST_DMG" || "$file" -nt "$LATEST_DMG" ]]; then LATEST_DMG="$file"; fi
    done

    echo "File to upload: $LATEST_DMG"

    # Send package for notarization; retrieve REQUEST_UUID
    echo "Sending notarization request."

    # Apply timeout / retry
    for attempt in $(seq 1 $NOTARIZATION_ATTEMPTS); do
    RESULT=$(timeout "$NOTARIZATION_TIMEOUT" xcrun notarytool submit --apple-id "$APPLE_ACCOUNT" --team-id "$TEAM_ID" --password "$NOTARIZATION_PWD" "$LATEST_DMG" --wait) || EXIT_CODE=$?
    echo "Results: $RESULT"
    SUBMISSION_ID=$(echo "$RESULT" | awk '$1 == "id:"{print $2; exit}')
    echo "Submission ID: $SUBMISSION_ID"
    echo "Submission logs:"
    xcrun notarytool log --apple-id "$APPLE_ACCOUNT" --team-id "$TEAM_ID" --password "$NOTARIZATION_PWD" "$SUBMISSION_ID"
    if [ -n "$EXIT_CODE" ]; then
        echo "Notarization attempt #$attempt/$NOTARIZATION_ATTEMPTS failed, retrying in $NOTARIZATION_WAIT_TIME"
        sleep "$NOTARIZATION_WAIT_TIME"
    else
        echo "Successfully notarized the package"
        break
    fi
    done
    echo -e "\e[0Ksection_end:`date +%s`:notarization\r\e[0K"
fi

if [ "$SIGN" = true ]; then
    echo Built signed package
else
    echo Built unsigned package
fi

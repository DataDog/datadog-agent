#!/usr/bin/env bash

set -eo pipefail

if [ "${SIGN:-false}" = true ]; then
    echo "Signing enabled"
else
    echo "Signing disabled"
fi

# --- Setup environment ---
unset OMNIBUS_BASE_DIR
export INSTALL_DIR="$TMPDIR/datadog-agent-build/bin"
export CONFIG_DIR="$TMPDIR/datadog-agent-build/config"
export OMNIBUS_DIR="$TMPDIR/omnibus_build"
export OMNIBUS_PACKAGE_DIR="$PWD"/omnibus/pkg

rm -rf "$INSTALL_DIR" "$CONFIG_DIR" "$OMNIBUS_DIR"
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$OMNIBUS_DIR"

# Update the INTEGRATION_CORE_VERSION if requested
if [ -n "$INTEGRATIONS_CORE_REF" ]; then
    export INTEGRATIONS_CORE_VERSION="$INTEGRATIONS_CORE_REF"
fi

# --- Setup signing ---
if [ "${SIGN:-false}" = true ]; then
    # Add certificates to temporary keychain
    echo "Setting up signing secrets"

    KEYCHAIN_PWD=$("$CI_PROJECT_DIR/tools/ci/fetch_secret.sh" "$MACOS_KEYCHAIN_PWD" password) || exit $?; export KEYCHAIN_PWD
    CODESIGNING_CERT_BASE64=$("$CI_PROJECT_DIR/tools/ci/fetch_secret.sh" "$MACOS_APPLE_APPLICATION_SIGNING" certificate) || exit $?; export CODESIGNING_CERT_BASE64
    CODESIGNING_CERT_PASSPHRASE=$("$CI_PROJECT_DIR/tools/ci/fetch_secret.sh" "$MACOS_APPLE_APPLICATION_SIGNING" passphrase) || exit $?; export CODESIGNING_CERT_PASSPHRASE
    INSTALLER_CERT_BASE64=$("$CI_PROJECT_DIR/tools/ci/fetch_secret.sh" "$MACOS_APPLE_INSTALLER_SIGNING" certificate) || exit $?; export INSTALLER_CERT_BASE64
    INSTALLER_CERT_PASSPHRASE=$("$CI_PROJECT_DIR/tools/ci/fetch_secret.sh" "$MACOS_APPLE_INSTALLER_SIGNING" passphrase) || exit $?; export INSTALLER_CERT_PASSPHRASE

    NOTARIZATION_PWD=$("$CI_PROJECT_DIR/tools/ci/fetch_secret.sh" "$MACOS_APPLE_DEVELOPER_ACCOUNT" notarization-password) || exit $?; export NOTARIZATION_PWD
    TEAM_ID=$("$CI_PROJECT_DIR/tools/ci/fetch_secret.sh" "$MACOS_APPLE_DEVELOPER_ACCOUNT" team-id) || exit $?; export TEAM_ID
    APPLE_ACCOUNT=$("$CI_PROJECT_DIR/tools/ci/fetch_secret.sh" "$MACOS_APPLE_DEVELOPER_ACCOUNT" user) || exit $?; export APPLE_ACCOUNT

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
if [ "${SIGN:-false}" = true ]; then
    # Unlock the keychain to get access to the signing certificates
    security unlock-keychain -p "$KEYCHAIN_PWD" "$KEYCHAIN_NAME"
    dda inv -- -e omnibus.build --hardened-runtime --config-directory "$CONFIG_DIR" --install-directory "$INSTALL_DIR" --base-dir "$OMNIBUS_DIR" || exit 1
    # Lock the keychain once we're done
    security lock-keychain "$KEYCHAIN_NAME"
else
    dda inv -- -e omnibus.build --skip-sign --config-directory "$CONFIG_DIR" --install-directory "$INSTALL_DIR" --base-dir "$OMNIBUS_DIR" || exit 1
fi

#TODO(regis): consider moving the following check to `DataDog/omnibus-ruby` to benefit other OSes
declare -i dangling=0
real_install=$(readlink -f "$INSTALL_DIR")
while read -r link; do
    target=$(readlink "$link")
    if [ ! -e "$link" ]; then
        dangling+=1
        echo >&2 "Dangling symlink: $link -❌> $target (must resolve to an existing target)"
        continue
    fi
    real_target=$(readlink -f "$link")
    if [[ ! $real_target = $real_install/* ]]; then
        dangling+=1
        echo >&2 "Outbound symlink: $link -❌> $target (must resolve inside install prefix)"
        continue
    fi
    if [[ $target = /* ]]; then
        dangling+=1
        echo >&2 "Absolute symlink: $link -❌> $target (must be relative to symlink's directory)"
        continue
    fi
done < <(find "$INSTALL_DIR" -type l)
if [ $dangling -gt 0 ]; then
    exit $dangling
fi

echo Built packages using omnibus

# --- Notarization ---
if [ "${SIGN:-false}" = true ]; then
    printf "\033[0Ksection_start:%s:notarization\r\033[0KDoing notarization\n" "$(date +%s)"
    unset LATEST_DMG

    # Find latest .dmg file in $GOPATH/src/github.com/Datadog/datadog-agent/omnibus/pkg
    for file in "$PWD/omnibus/pkg"/*.dmg; do
    if [[ -z "$LATEST_DMG" || "$file" -nt "$LATEST_DMG" ]]; then LATEST_DMG="$file"; fi
    done

    echo "File to upload: $LATEST_DMG"

    # Send package for notarization; retrieve REQUEST_UUID
    echo "Sending notarization request."
    submit_for_notarization() {
        set -euo pipefail
        local dmg_file="$1"
        xcrun notarytool submit \
            --apple-id "$APPLE_ACCOUNT" \
            --password "$NOTARIZATION_PWD" \
            --team-id "$TEAM_ID" \
            "$dmg_file" | tee /dev/stderr | awk '$1 == "id:" {id=$2} END{if (id) print id; else exit 2}'
    }
    export -f submit_for_notarization
    SUBMISSION_ID=$(tools/ci/retry.sh -n "$NOTARIZATION_ATTEMPTS" submit_for_notarization "$LATEST_DMG" | tail -n1)
    echo "Submission ID: $SUBMISSION_ID"

    wait_for_notarization() {
        set -euo pipefail
        local submission_id="$1"
        xcrun notarytool wait \
            --apple-id "$APPLE_ACCOUNT" \
            --password "$NOTARIZATION_PWD" \
            --team-id "$TEAM_ID" \
            --timeout "$NOTARIZATION_TIMEOUT" \
            "$submission_id"
    }
    export -f wait_for_notarization
    tools/ci/retry.sh -n "$NOTARIZATION_ATTEMPTS" wait_for_notarization "$SUBMISSION_ID"

    check_notarization_status() {
        set -euo pipefail
        local submission_id="$1"
        xcrun notarytool log \
            --apple-id "$APPLE_ACCOUNT" \
            --password "$NOTARIZATION_PWD" \
            --team-id "$TEAM_ID" \
            "$submission_id" | tee /dev/stderr | jq --exit-status '.status == "Accepted"'
    }
    export -f check_notarization_status
    tools/ci/retry.sh -n "$NOTARIZATION_ATTEMPTS" check_notarization_status "$SUBMISSION_ID"
    printf "\033[0Ksection_end:%s:notarization\r\033[0K\n" "$(date +%s)"
fi

if [ "${SIGN:-false}" = true ]; then
    echo Built signed package
else
    echo Built unsigned package
fi

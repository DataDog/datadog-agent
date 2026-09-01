#!/usr/bin/env bash
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.
#
# Build the per-version macOS Fleet package.
#
# This is the artifact a version experiment installs. It differs from the
# package inside the .dmg in three ways, each of which is the reason it cannot
# simply be that package:
#
#   1. It installs to /opt/datadog-packages/datadog-agent/<version>, not to
#      /opt/datadog-agent. installer(8)'s -target names a volume rather than a
#      directory, so the destination is baked in at build time. One package per
#      version is the direct consequence: there is no way to redirect one.
#
#   2. It carries no scripts. Not empty scripts -- none at all, so there is
#      nothing for the installer to run as root. Installing a version and
#      putting it into service are separate events, minutes or weeks apart, and
#      everything that happens at the second belongs to the installer's hooks.
#
#   3. The payload is non-relocatable and not version-checked, via the component
#      property list, so it lands where it is told even on a host that already
#      has an Agent, and a downgrade is not silently skipped.
#
# Signing and notarization are here rather than in a Bazel rule because neither
# is hermetic: signing needs a keychain the build does not own, and notarization
# is a network round trip to Apple with a wait of unbounded length. A Bazel
# action that did either would be a non-reproducible action pretending to be a
# reproducible one. The templates it consumes are Bazel outputs; the parts that
# reach outside the build are not.
#
# Usage:
#   build_fleet_pkg.sh --payload <dir> --version <v> --out <file>
#                      [--component-plist <file>] [--distribution <file>]
#
# Environment (all optional; signing is skipped when SIGN is not true):
#   SIGN                   "true" to sign, notarize and staple
#   INSTALLER_SIGNING_ID   Developer ID Installer identity
#   APPLE_ACCOUNT, NOTARIZATION_PWD, TEAM_ID
#   NOTARIZATION_TIMEOUT   default 30m
#   NOTARIZATION_ATTEMPTS  default 3

set -euo pipefail

readonly IDENTIFIER="com.datadoghq.agent.fleet"
readonly FRIENDLY_NAME="Datadog Agent"
readonly POOL_ROOT="/opt/datadog-packages/datadog-agent"
# 11.0 is the oldest macOS the Agent supports; asserting it here is the last
# point at which a too-old host refuses cleanly rather than accepting a payload
# that will not run.
readonly MIN_OS_VERSION="11.0"

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

payload=""
version=""
out=""
component_plist="$script_dir/component.plist.in"
distribution_template="$script_dir/distribution.xml.in"

while [ $# -gt 0 ]; do
    case "$1" in
        --payload) payload="$2"; shift 2 ;;
        --version) version="$2"; shift 2 ;;
        --out) out="$2"; shift 2 ;;
        --component-plist) component_plist="$2"; shift 2 ;;
        --distribution) distribution_template="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 2 ;;
    esac
done

for required in payload version out; do
    if [ -z "${!required}" ]; then
        echo "--$required is required" >&2
        exit 2
    fi
done
if [ ! -d "$payload" ]; then
    echo "payload directory does not exist: $payload" >&2
    exit 2
fi

# The paths the installer's completeness check will demand of the version
# directory. Checking them here means a payload assembled wrong fails the build
# rather than a host: an incomplete version installs successfully, fails the
# check, and is never named -- which is safe, but it is a wasted download and a
# failed experiment for a reason that was knowable at build time.
#
# Keep in step with requiredPayloadPaths in
# pkg/fleet/installer/macpkg/payload.go.
required_payload_paths=(
    "bin/agent/agent"
    "embedded/bin/installer"
    "embedded/bin/system-probe"
    "embedded/bin/agent-data-plane"
)
missing=0
for path in "${required_payload_paths[@]}"; do
    if [ ! -e "$payload/$path" ]; then
        echo "payload is missing $path, which the installer requires before it will name this version" >&2
        missing=$((missing + 1))
    fi
done
if [ "$missing" -gt 0 ]; then
    exit 1
fi

staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT

host_architecture=$(uname -m)
component_pkg="$staging/datadog-agent-core.pkg"

# --- Component package ---
#
# --scripts is deliberately absent, not pointed at an empty directory: an empty
# --scripts is still a scripts directory, and the point is that this package has
# no code in it at all.
echo "Building component package for $version"
pkgbuild \
    --identifier "$IDENTIFIER" \
    --version "$version" \
    --root "$payload" \
    --install-location "$POOL_ROOT/$version" \
    --component-plist "$component_plist" \
    "$component_pkg"

# --- Distribution ---
distribution="$staging/Distribution"
sed \
    -e "s|{identifier}|$IDENTIFIER|g" \
    -e "s|{friendly_name}|$FRIENDLY_NAME|g" \
    -e "s|{build_version}|$version|g" \
    -e "s|{host_architecture}|$host_architecture|g" \
    -e "s|{min_os_version}|$MIN_OS_VERSION|g" \
    -e "s|{component_pkg}|$(basename "$component_pkg")|g" \
    "$distribution_template" > "$distribution"

# --- Product package ---
#
# --resources is absent for the same reason the panes are: there is no UI to
# carry a background or a license into.
echo "Building product package"
productbuild_args=(
    --distribution "$distribution"
    --package-path "$staging"
)
if [ "${SIGN:-false}" = true ]; then
    productbuild_args+=(--sign "${INSTALLER_SIGNING_ID:?INSTALLER_SIGNING_ID is required when SIGN=true}")
fi
mkdir -p "$(dirname "$out")"
productbuild "${productbuild_args[@]}" "$out"

if [ "${SIGN:-false}" != true ]; then
    echo "Built unsigned package at $out"
    # An unsigned package is refused by the installer's verifier, which requires
    # a Datadog Developer ID signature and Gatekeeper acceptance. Saying so here
    # keeps an unsigned local build from looking like a shippable artifact.
    echo "NOTE: unsigned; the installer will refuse this package on a real host" >&2
    exit 0
fi

# --- Notarization ---
#
# The .pkg is notarized in its own right rather than relying on the .dmg's
# ticket. Nothing wraps this package: the installer downloads it and hands it
# straight to installer(8), so a ticket that covered only some outer container
# would not exist. Stapling matters for the same reason -- the host may have no
# route to Apple when the experiment starts.
: "${APPLE_ACCOUNT:?required for notarization}"
: "${NOTARIZATION_PWD:?required for notarization}"
: "${TEAM_ID:?required for notarization}"
notarization_timeout="${NOTARIZATION_TIMEOUT:-30m}"
notarization_attempts="${NOTARIZATION_ATTEMPTS:-3}"

echo "Submitting $out for notarization"
submission_id=""
for attempt in $(seq 1 "$notarization_attempts"); do
    if submission_id=$(xcrun notarytool submit \
            --apple-id "$APPLE_ACCOUNT" \
            --password "$NOTARIZATION_PWD" \
            --team-id "$TEAM_ID" \
            "$out" | tee /dev/stderr | awk '$1 == "id:" {id=$2} END{if (id) print id; else exit 2}'); then
        break
    fi
    if [ "$attempt" = "$notarization_attempts" ]; then
        echo "could not submit $out for notarization" >&2
        exit 1
    fi
    sleep 5
done
echo "Submission ID: $submission_id"

xcrun notarytool wait \
    --apple-id "$APPLE_ACCOUNT" \
    --password "$NOTARIZATION_PWD" \
    --team-id "$TEAM_ID" \
    --timeout "$notarization_timeout" \
    "$submission_id"

# notarytool wait exits zero for a rejected submission as well as an accepted
# one -- it reports that the wait finished, not that the answer was yes -- so the
# log is what the build decides on.
xcrun notarytool log \
    --apple-id "$APPLE_ACCOUNT" \
    --password "$NOTARIZATION_PWD" \
    --team-id "$TEAM_ID" \
    "$submission_id" | tee /dev/stderr | jq --exit-status '.status == "Accepted"' > /dev/null

xcrun stapler staple "$out"

# The two checks the installer's verifier will make on the host, made here while
# there is a person to read the failure.
pkgutil --check-signature "$out"
spctl --assess -vv --type install "$out"

echo "Built signed, notarized and stapled package at $out"

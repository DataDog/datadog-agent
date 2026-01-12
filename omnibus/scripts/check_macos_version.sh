#! /usr/bin/env bash
# Check that all binaries are compatible with the minimum macOS version we support
# https://docs.datadoghq.com/agent/supported_platforms/?tab=macos
# The script will return an error code and print information on the violations or errors found
# Inputs (via environment):
# INSTALL_DIR: directory with binaries to check for compatibility
# ARCH: arm64 / x86_64
# MIN_ACCEPTABLE_VERSION: minimum version that we accept
# ALLOW_PATTERN: a regex to match files that are "allowed to fail"

set -euo pipefail

satisfies_min_version() {
    local min_acceptable_version="$MIN_ACCEPTABLE_VERSION"
    local arch="$ARCH"

    local vtool_output="$(vtool -arch "${arch}" -show-build "$1")"
    local cmd=$(echo "$vtool_output" | awk '/cmd / {print $2}')

    case $cmd in
        "LC_VERSION_MIN_MACOSX")
            local min_version=$(echo "$vtool_output" | awk '/version / {print $2}')
            ;;
        "LC_BUILD_VERSION")
            local min_version=$(echo "$vtool_output" | awk '/minos / {print $2}')
            ;;
    esac

    if [ -z "${min_version:-}" ]; then
        echo "ERROR: failed to detect minimum version of $1" >&2
        echo "vtool output: $(vtool -show-build "$1")" >&2
        return 1
    fi
    local highest_version=$(printf '%s\n%s\n' "$min_version" "$min_acceptable_version" | sort -V | tail -1)
    if [[ "$highest_version" != "$min_acceptable_version" ]]; then
        if [[ "$1" =~ ${ALLOW_PATTERN} ]]; then
            echo "WARNING: minimum version for $1 (${min_version}) is higher than the minimimum acceptable (${min_acceptable_version}) (explicitly allowed)" >&2
        else
            echo "ERROR: minimum version for $1 (${min_version}) is higher than the minimimum acceptable (${min_acceptable_version})" >&2
            return 1
        fi
    fi
}

export -f satisfies_min_version
export ALLOW_PATTERN="${ALLOW_PATTERN:-}"

echo "Checking $INSTALL_DIR for compatibility with version >= $MIN_ACCEPTABLE_VERSION and architecture $ARCH..."

find "$INSTALL_DIR" -type f -print0 |
    xargs -0 -n1000 -P9 file -n --mime-type |
    awk -F: '/[^)]:[[:space:]]*application\/x-mach-binary/ { printf "%s%c", $1, 0 }' |
    xargs -0 -n1 -P9 -I {} bash -euo pipefail -c 'satisfies_min_version "$@"' _ {}

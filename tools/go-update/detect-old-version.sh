#!/bin/bash

set -euo pipefail

# This script is used to display files which might contain a reference to an old Go version.
#
# It can be explicitely given a Go version as argument to look for.
# Otherwise, it assumes that the current branch is the one which contains the new Go version,
# and compares it to $GITHUB_BASE_REF if it is defined, or "main" otherwise

if [ $# -gt 2 ]; then
    echo "This script takes at most two arguments, the old Go version we want to look for and the new one" 1>&2
    echo "If no argument is given, they are fetched from the '.go-version' file, respectively of the branch from GITHUB_BASE_REF, or main if it is not defined." 1>&2
    echo "" 1>&2
    echo "If only one version is given, it is looked for without any particular checking." 1>&2
    echo "If zero or two versions are given, the script checks whether the minor changed or only the bugfix, and uses either accordingly." 1>&2
    exit 1
fi

if [ $# -eq 1 ]; then
    # use the version given as argument
    PATTERN_GO_VERSION="$1"
else
    if [ $# -eq 0 ]; then
        # use the version from the .go-version file, and compare it to the one from GITHUB_BASE_REF, or main
        GO_VERSION_PREV_BUGFIX=$(git show "${GITHUB_BASE_REF:-main}":.go-version)
        GO_VERSION_NEW_BUGFIX=$(cat .go-version)
    else
        GO_VERSION_PREV_BUGFIX="$1"
        GO_VERSION_NEW_BUGFIX="$2"
    fi

    GO_VERSION_PREV_MINOR="${GO_VERSION_PREV_BUGFIX%.*}"
    echo "Former bugfix version: $GO_VERSION_PREV_BUGFIX" 1>&2
    echo "Former minor version: $GO_VERSION_PREV_MINOR" 1>&2

    GO_VERSION_NEW_MINOR="${GO_VERSION_NEW_BUGFIX%.*}"
    echo "New bugfix version: $GO_VERSION_NEW_BUGFIX" 1>&2
    echo "New minor version: $GO_VERSION_NEW_MINOR" 1>&2

    # if the old bugfix is the same as the new one, return
    if [ "$GO_VERSION_PREV_BUGFIX" == "$GO_VERSION_NEW_BUGFIX" ]; then
        echo "This branch doesn't change the Go version" 1>&2
        exit 1
    fi

    if [ "$GO_VERSION_PREV_MINOR" != "$GO_VERSION_NEW_MINOR" ]; then
        # minor update
        PATTERN_GO_VERSION="$GO_VERSION_PREV_MINOR"
    else
        # bugfix update
        PATTERN_GO_VERSION="$GO_VERSION_PREV_BUGFIX"
    fi
fi

echo "Looking for Go version: $PATTERN_GO_VERSION" 1>&2

# Go versions can be preceded by a 'g' (golang), 'o' (go), 'v', or a non-alphanumerical character
# Prevent matching when preceded by a dot, as it is likely not a Go version in that case
PATTERN_PREFIX='(^|[^.a-fh-np-uw-z0-9])'
# Go versions in go.dev/dl URLs are followed by a dot, so we need to allow it in the regex
PATTERN_SUFFIX='($|[^a-z0-9])'

# Go versions contain dots, which are special characters in regexes, so we need to escape them
# Safely assume that the version only contains numbers and dots
PATTERN_GO_VERSION_ESCAPED="$(echo "$PATTERN_GO_VERSION" | sed 's/\./\\./g')"
# The regex is not perfect, but it should be good enough
PATTERN="${PATTERN_PREFIX}${PATTERN_GO_VERSION_ESCAPED}${PATTERN_SUFFIX}"
echo "Using pattern: $PATTERN" 1>&2

# grep returns 1 when no match is found, which would cause the script to fail, wrap it to return 0 in that case.
# It returns a non-zero value if grep returned >1.
function safegrep() {
    grep "$@" || [ $? -eq 1 ]
}

# -r: recursive
# -I: ignore binary files
# -i: ignore case
# -n: print line number
# -E: extended regexp pattern to match
# --exclude-dir: exclude directories
# --exclude: exclude file name patterns
safegrep -r -I -i -n -E "$PATTERN" . \
    --exclude-dir fixtures --exclude-dir .git --exclude-dir releasenotes \
    --exclude-dir omnibus --exclude-dir snmp --exclude-dir testdata \
    --exclude '*.rst' --exclude '*.sum' --exclude '*generated*.go' --exclude '*.svg' | \
# -v: invert match
# exclude matches in go.mod files that are dependency versions (note the 'v' before the version)
safegrep -v -E "go\.mod:.*v${PATTERN_GO_VERSION_ESCAPED}" | \
# grep outputs paths starting with ./, so we remove that
sed -e 's|^./||'

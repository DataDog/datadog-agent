#!/bin/bash
# This script checks if any non linux-only files in omnibus/config/software/ have changed
# Returns 0 if non-linux files changed (should trigger jobs)
# Returns 1 if only linux only files changed (should skip jobs)

set -e

# Define excluded files (Linux-specific dependencies not needed for Windows)
LINUX_ONLY=(
    # openscap
    "acl.rb"
    "attr.rb"
    "dbus.rb"
    "libacl.rb"
    "libgcrypt.rb"
    "libselinux.rb"
    "libsepol.rb"
    "libxml2.rb"
    "libxslt.rb"
    "lua.rb"
    "openscap.rb"
    "pcre2.rb"
    "popt.rb"
    "rpm.rb"
    "util-linux.rb"
    "xmlsec.rb"
    "zstd.rb"

    # other
    "init-scripts-agent.rb"
    "init-scripts-ddot.rb"
    "init-scripts-iot.rb"
)

# Determine the comparison base
COMPARE_BASE="${CI_MERGE_REQUEST_DIFF_BASE_SHA:-${COMPARE_TO_BRANCH:-main}}"

# Get all changed files in omnibus/config/software/
CHANGED_FILES=$(git diff --name-only "$COMPARE_BASE"...HEAD -- omnibus/config/software/ 2>/dev/null || true)

# If no files changed in this directory, return 1 (skip)
if [ -z "$CHANGED_FILES" ]; then
    echo "No changes detected in omnibus/config/software/"
    exit 1
fi

echo "Changed files in omnibus/config/software/:"
echo "$CHANGED_FILES"

# Check each changed file
NON_LINUX_FOUND=false
for file in $CHANGED_FILES; do
    basename=$(basename "$file")

    # Check if this file is in the linux specific list
    is_linux_only=false
    for filename in "${LINUX_ONLY[@]}"; do
        if [ "$basename" = "$filename" ]; then
            is_linux_only=true
            echo "  - $basename (linux only)"
            break
        fi
    done

    if [ "$is_linux_only" = "false" ]; then
        echo "  - $basename (TRIGGERS BUILD)"
        NON_LINUX_FOUND=true
    fi
done

if [ "$NON_LINUX_FOUND" = "true" ]; then
    echo "$0: Non-excluded files changed - Windows installer jobs should run"
    exit 1
else
    echo "$0: Only linux omnibus files changed - Skipping Windows installer jobs"
    exit 0
fi

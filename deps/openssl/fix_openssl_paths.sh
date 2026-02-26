#!/usr/bin/env bash
#
# fix_openssl_paths.sh - Fix hardcoded OpenSSL paths in binaries
#
# This script replaces the placeholder directory in OpenSSL libraries
# with the target destination directory.
#
# The placeholder is inserted by the build process and is:
#   ##...PLACEHOLDERDIRECTORY...## (200 chars total)
#
# Usage: fix_openssl_paths.sh --destdir <destination_prefix> <library_files...>
#

set -euo pipefail

DESTDIR=""

while [ "$#" -gt 0 ]; do
    case "$1" in
        --destdir | -d)
            shift
            DESTDIR="$1"
            ;;
        *)
            break
    esac
    shift
done

if [ -z "$DESTDIR" ]; then
    echo "Usage: fix_openssl_paths.sh --destdir <destination_prefix> <library_files...>"
    echo ""
    echo "Example: fix_openssl_paths.sh --destdir /opt/datadog-agent/embedded libssl.so libcrypto.so"
    exit 1
fi

if [ "$#" -eq 0 ]; then
    echo "Error: No library files specified"
    exit 1
fi

PLACEHOLDER='##########################################################################################PLACEHOLDERDIRECTORY##########################################################################################'

# Do all replacements in a single perl pass to avoid file corruption
# from multiple read/write cycles
for lib in "$@"; do
    if [ ! -f "$lib" ]; then
        echo "Error: $lib not found"
        exit 1
    fi

    # Resolve symlinks to get the actual file
    REAL_LIB=$(perl -e 'use Cwd "abs_path"; print abs_path($ARGV[0])' "$lib")

    echo "Patching: $REAL_LIB"

    PLACEHOLDER="$PLACEHOLDER" DESTDIR="$DESTDIR" perl -0777 -pi -e '
        my $placeholder = $ENV{PLACEHOLDER};
        my $destdir = $ENV{DESTDIR};

        # All suffixes to replace (order matters - longer suffixes first to avoid partial matches)
        # OpenSSL stores paths in two forms:
        #   1. Display strings for `openssl version -a` ending with " (e.g., MODULESDIR: "/path")
        #   2. Internal runtime paths without quotes used for actually loading modules/engines
        # Both forms must be replaced for FIPS to work correctly.
        my @suffixes = (
            # Longer paths first to avoid partial matches
            "/ssl/private",
            "/ssl/cert.pem",
            "/ssl/certs",
            # Display strings (with trailing quote)
            "/lib/engines-3\"",
            "/lib/ossl-modules\"",
            "/ssl\"",
            # Internal runtime paths (no trailing quote) - CRITICAL for FIPS provider loading!
            "/lib/engines-3",
            "/lib/ossl-modules",
            "/ssl",
        );

        for my $suffix (@suffixes) {
            my $old_path = $placeholder . $suffix;
            my $new_path = $destdir . $suffix;
            my $padding = "\x00" x (length($old_path) - length($new_path));
            s/\Q$old_path\E/$new_path . $padding/ge;
        }
    ' "$REAL_LIB"

    # Re-sign the binary on macOS (modifying invalidates the code signature)
    if [[ "$(uname)" == "Darwin" ]]; then
        codesign -f -s - "$REAL_LIB" 2>/dev/null || true
    fi
done

echo "Fixed OpenSSL paths in: $*"
echo "New prefix: $DESTDIR"

#!/bin/bash

set -euo pipefail

DESTDIR=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --destdir)
            DESTDIR="$2"
            shift 2
            ;;
        --destdir=*)
            DESTDIR="${1#*=}"
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 --destdir <directory>"
            exit 1
            ;;
    esac
done

if [[ -z "$DESTDIR" ]]; then
    echo "Error: --destdir is required"
    echo "Usage: $0 --destdir <directory>"
    exit 1
fi

OPENSSL_CNF="${DESTDIR}/ssl/openssl.cnf"
FIPSINSTALL_SH="${DESTDIR}/embedded/bin/fipsinstall.sh"

# Replace {{install_dir}} with DESTDIR in openssl.cnf
if [[ -f "$OPENSSL_CNF" ]]; then
    sed -i.bak "s|{{install_dir}}|${DESTDIR}|g" "$OPENSSL_CNF"
    rm -f "${OPENSSL_CNF}.bak"
    echo "Updated: $OPENSSL_CNF"
else
    echo "Warning: $OPENSSL_CNF not found"
fi

# Replace {{install_dir}} with DESTDIR in fipsinstall.sh
if [[ -f "$FIPSINSTALL_SH" ]]; then
    sed -i.bak "s|{{install_dir}}|${DESTDIR}|g" "$FIPSINSTALL_SH"
    rm -f "${FIPSINSTALL_SH}.bak"
    echo "Updated: $FIPSINSTALL_SH"
else
    echo "Warning: $FIPSINSTALL_SH not found"
fi

echo "Configuration complete."


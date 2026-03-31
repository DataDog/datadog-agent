#!/bin/bash
# Builds the C filtertest binary from libpcap sources.
# Usage: ./build_filtertest.sh <libpcap-source-dir> <output-binary>
#
# Example:
#   ./build_filtertest.sh /path/to/libpcap-1.10.6 ./filtertest

set -euo pipefail

LIBPCAP_SRC="${1:?Usage: $0 <libpcap-source-dir> <output-binary>}"
OUTPUT="${2:?Usage: $0 <libpcap-source-dir> <output-binary>}"

if [ ! -f "$LIBPCAP_SRC/pcap.h" ]; then
    echo "Error: $LIBPCAP_SRC does not look like a libpcap source directory" >&2
    exit 1
fi

# Build libpcap if not already built
if [ ! -f "$LIBPCAP_SRC/libpcap.a" ]; then
    echo "Building libpcap..."
    (cd "$LIBPCAP_SRC" && ./configure --quiet --disable-shared && make -j"$(nproc)" -s)
fi

# Build filtertest
echo "Building filtertest..."
(cd "$LIBPCAP_SRC" && make -j"$(nproc)" -s testprogs)

cp "$LIBPCAP_SRC/testprogs/filtertest" "$OUTPUT"
echo "filtertest built: $OUTPUT"

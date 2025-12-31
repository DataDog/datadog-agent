#!/bin/bash
# Transparent decompression wrapper stub
# This script decompresses and executes the actual binary

STUB_PATH="$(readlink -f "$0")"
STUB_NAME="$(basename "$STUB_PATH")"
COMPRESSED="${STUB_PATH}.gz"
CACHE_DIR="/tmp/.bin-cache"
CACHED_BIN="${CACHE_DIR}/${STUB_NAME}"

# Create cache directory
mkdir -p "$CACHE_DIR"

# Decompress to cache if not already there
if [ ! -f "$CACHED_BIN" ]; then
    gzip -dc "$COMPRESSED" > "$CACHED_BIN"
    chmod +x "$CACHED_BIN"
fi

# Execute the actual binary with all arguments
exec "$CACHED_BIN" "$@"


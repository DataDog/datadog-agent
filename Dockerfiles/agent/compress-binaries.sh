#!/bin/bash
# Compresses all binaries in a directory and replaces them with stubs

set -e

BIN_DIR="$1"
STUB_TEMPLATE="/tmp/stub-wrapper.sh"

if [ -z "$BIN_DIR" ] || [ ! -d "$BIN_DIR" ]; then
    echo "Usage: $0 <directory>"
    exit 1
fi

echo "Processing binaries in $BIN_DIR..."

# Find all executable files (binaries)
find "$BIN_DIR" -maxdepth 1 -type f -executable | while read -r binary; do
    # Skip symlinks
    if [ -L "$binary" ]; then
        continue
    fi
    
    # Skip if already processed
    if grep -q "Transparent decompression wrapper" "$binary" 2>/dev/null; then
        echo "Skipping stub: $(basename "$binary")"
        continue
    fi
    
    # Skip if not an ELF binary (only compress compiled binaries, not scripts)
    if ! file "$binary" | grep -q "ELF"; then
        continue
    fi
    
    # Skip if compressed version exists
    if [ -f "${binary}.gz" ]; then
        echo "Already compressed: $(basename "$binary")"
        continue
    fi
    
    echo "Compressing: $(basename "$binary")"
    
    # Get original size (Linux stat format)
    ORIG_SIZE=$(stat -c%s "$binary")
    
    # Compress binary (gzip -9 for best compression)
    gzip -9 -c "$binary" > "${binary}.gz"
    
    # Get compressed size
    COMP_SIZE=$(stat -c%s "${binary}.gz")
    
    # Replace binary with stub
    cp "$STUB_TEMPLATE" "$binary"
    chmod +x "$binary"
    
    # Calculate savings
    SAVED=$((ORIG_SIZE - COMP_SIZE))
    PERCENT=$((SAVED * 100 / ORIG_SIZE))
    
    echo "  Original: ${ORIG_SIZE} bytes"
    echo "  Compressed: ${COMP_SIZE} bytes"
    echo "  Saved: ${SAVED} bytes (${PERCENT}%)"
done

echo "Compression complete!"


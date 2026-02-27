#!/usr/bin/env bash

set -euo pipefail

OTOOL=$1
PREFIX=$2
INPUT=$3
OUTPUT=$4

cp "$INPUT" "$OUTPUT"
install_name_tool -add_rpath "$PREFIX/lib" "$OUTPUT" 2>/dev/null || true
# Get the old install name/ID
dylib_name=$(basename "$OUTPUT")
new_id="$PREFIX/lib/$dylib_name"

# Change the dylib's own ID
install_name_tool -id "$new_id" "$OUTPUT"

# Update all dependency paths that point to sandbox locations
${OTOOL} -L "$OUTPUT" | tail -n +2 | awk '{print $1}' | while read -r dep; do
    if [[ "$dep" == *"sandbox"* ]] || [[ "$dep" == *"bazel-out"* ]]; then
        dep_name=$(basename "$dep")
        new_dep="$PREFIX/lib/$dep_name"
        install_name_tool -change "$dep" "$new_dep" "$OUTPUT" 2>/dev/null || true
        install_name_tool -add_rpath "$PREFIX/lib" "$dep" 2>/dev/null || true
    fi
done

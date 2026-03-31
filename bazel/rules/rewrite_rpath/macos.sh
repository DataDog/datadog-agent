#!/usr/bin/env bash

set -euo pipefail

OTOOL=$1
PREFIX=$2
INPUT=$3
OUTPUT=$4

# PREFIX is the full rpath (e.g. {install_dir}/embedded/lib); use as-is, do not append /lib
cp "$INPUT" "$OUTPUT"
install_name_tool -add_rpath "$PREFIX" "$OUTPUT" 2>/dev/null || true
dylib_name=$(basename "$OUTPUT")
new_id="$PREFIX/$dylib_name"

install_name_tool -id "$new_id" "$OUTPUT"

# Update all dependency paths that point to sandbox locations
${OTOOL} -L "$OUTPUT" | tail -n +2 | awk '{print $1}' | while read -r dep; do
    if [[ "$dep" == *"sandbox"* ]] || [[ "$dep" == *"bazel-out"* ]]; then
        dep_name=$(basename "$dep")
        new_dep="$PREFIX/$dep_name"
        install_name_tool -change "$dep" "$new_dep" "$OUTPUT" 2>/dev/null || true
        install_name_tool -add_rpath "$PREFIX" "$dep" 2>/dev/null || true
    fi
done

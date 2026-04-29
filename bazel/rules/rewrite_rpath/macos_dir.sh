#!/usr/bin/env bash

set -euo pipefail

MACOS_SH=$1
OTOOL=$2
PREFIX=$3
INPUT_DIR=$4
OUTPUT_DIR=$5

# Copy the full directory (handles non-dylib files such as headers and pkgconfig).
cp -rL "$INPUT_DIR" "$OUTPUT_DIR"
# Bazel sandbox files are read-only; make the copy writable so macos.sh can
# overwrite each dylib in place.
chmod -R u+w "$OUTPUT_DIR"

# Delegate per-file patching to macos.sh, which rewrites install names, rpaths,
# and re-signs each dylib.
find "$INPUT_DIR" -type f -name "*.dylib" | while read -r input_f; do
    rel_path="${input_f#"$INPUT_DIR"/}"
    "$MACOS_SH" "$OTOOL" "$PREFIX" "$input_f" "$OUTPUT_DIR/$rel_path"
done

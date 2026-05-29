#!/usr/bin/env bash
set -euo pipefail

MACOS_SH="$(find "$TEST_SRCDIR" -path "*/rewrite_rpath/macos.sh" | head -1)"
MACOS_DIR_SH="$(find "$TEST_SRCDIR" -path "*/rewrite_rpath/macos_dir.sh" | head -1)"

LIBS=()
while IFS= read -r f; do LIBS+=("$f"); done < <(find "$TEST_SRCDIR" \( -name "*.dylib" -o -name "*.so" \))

[ "${#LIBS[@]}" -ge 3 ] || { echo "FAIL: expected at least 3 libs (.dylib + .so), got ${#LIBS[@]}"; exit 1; }

INPUT_DIR="$(mktemp -d)"
OUTPUT_DIR="${INPUT_DIR}_out"
trap 'rm -rf "$INPUT_DIR" "${OUTPUT_DIR:-}"' EXIT

for lib in "${LIBS[@]}"; do
    cp "$lib" "$INPUT_DIR/"
done

PREFIX="@loader_path/../../embedded/lib"
"$MACOS_DIR_SH" "$MACOS_SH" /usr/bin/otool "$PREFIX" "$INPUT_DIR" "$OUTPUT_DIR"

FAILED=0
while IFS= read -r outfile; do
    if /usr/bin/otool -l "$outfile" | grep -q "$PREFIX"; then
        echo "OK: $(basename "$outfile") has rpath $PREFIX"
    else
        echo "FAIL: $(basename "$outfile") missing rpath $PREFIX"
        FAILED=1
    fi
done < <(find "$OUTPUT_DIR" \( -name "*.dylib" -o -name "*.so" \))

exit $FAILED

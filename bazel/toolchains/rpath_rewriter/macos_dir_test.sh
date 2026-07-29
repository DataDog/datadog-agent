#!/usr/bin/env bash
set -euo pipefail

MACOS_SH="$(find "$TEST_SRCDIR" -path "*/rpath_rewriter/rewrite_with_install_name_tool.sh" | head -1)"
MACOS_DIR_SH="$(find "$TEST_SRCDIR" -path "*/rpath_rewriter/rewrite_tree_with_install_name_tool.sh" | head -1)"
HELPER="$(find "$TEST_SRCDIR" -path "*/rpath_rewriter/rpath_helpers.sh" | head -1)"
INSTALL_NAME_TOOL="${1:?usage: $0 <install-name-tool>}"

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
"$MACOS_DIR_SH" "$HELPER" "$MACOS_SH" "$INSTALL_NAME_TOOL" /usr/bin/otool "$PREFIX" "$INPUT_DIR" "$OUTPUT_DIR"

FAILED=0
while IFS= read -r outfile; do
    rpaths=()
    while IFS= read -r rpath; do
        rpaths+=("$rpath")
    done < <(/usr/bin/otool -l "$outfile" | awk '
        $1 == "cmd" && $2 == "LC_RPATH" { in_rpath = 1; next }
        in_rpath && $1 == "path" { print $2; in_rpath = 0 }
    ')
    if [ "${#rpaths[@]}" -eq 1 ] && [ "${rpaths[0]}" = "$PREFIX" ]; then
        echo "OK: $(basename "$outfile") has only rpath $PREFIX"
    else
        echo "FAIL: $(basename "$outfile") rpaths: ${rpaths[*]:-<none>}"
        FAILED=1
    fi
done < <(find "$OUTPUT_DIR" \( -name "*.dylib" -o -name "*.so" \))

exit $FAILED

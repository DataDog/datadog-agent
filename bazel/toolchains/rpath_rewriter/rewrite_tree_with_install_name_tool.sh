#!/usr/bin/env bash

set -euo pipefail

HELPER=$1
MACOS_SH=$2
INSTALL_NAME_TOOL=$3
OTOOL=$4
PREFIX=$5
INPUT_DIR=$6
OUTPUT_DIR=$7

source "$HELPER"

# Walk the input tree once: route Mach-O libraries through rewrite_with_install_name_tool.sh
# (which writes a fresh output file with rewritten install names, rpaths, and ad-hoc signature)
# and copy everything else verbatim with cp -p to preserve the original mode.
find -L "$INPUT_DIR" -type f | while read -r input_f; do
    rel_path="${input_f#"$INPUT_DIR"/}"
    output_f="$OUTPUT_DIR/$rel_path"
    mkdir -p "$(dirname "$output_f")"
    case "$input_f" in
        *.dylib | *.so)
            file_prefix=$(origin_rpath_for_tree_file "@loader_path" "$OUTPUT_DIR" "$output_f" "$PREFIX")
            "$MACOS_SH" "$HELPER" "$INSTALL_NAME_TOOL" "$OTOOL" "$file_prefix" "$input_f" "$output_f"
            ;;
        *)
            cp -p "$input_f" "$output_f"
            ;;
    esac
done

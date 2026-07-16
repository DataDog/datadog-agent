#!/usr/bin/env bash

set -euo pipefail

MACOS_SH=$1
INSTALL_NAME_TOOL=$2
OTOOL=$3
PREFIX=$4
INPUT_DIR=$5
OUTPUT_DIR=$6

# Walk the input tree once: route Mach-O libraries through rewrite_with_install_name_tool.sh
# (which writes a fresh output file with rewritten install names, rpaths, and ad-hoc signature)
# and copy everything else verbatim with cp -p to preserve the original mode.
find -L "$INPUT_DIR" -type f | while read -r input_f; do
    rel_path="${input_f#"$INPUT_DIR"/}"
    output_f="$OUTPUT_DIR/$rel_path"
    mkdir -p "$(dirname "$output_f")"
    case "$input_f" in
        *.dylib | *.so)
            "$MACOS_SH" "$INSTALL_NAME_TOOL" "$OTOOL" "$PREFIX" "$input_f" "$output_f"
            ;;
        *)
            cp -p "$input_f" "$output_f"
            ;;
    esac
done

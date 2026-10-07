#!/usr/bin/env bash
# llvm-windres preprocesses .rc files with clang, which needs the mingw-w64
# headers that the cc toolchain only passes to compile actions.
set -euo pipefail
exec "$LLVM_WINDRES" \
    -I"$MINGW_INCLUDE_DIR" \
    -I"$MINGW_CRT_DIR" \
    -I"$(dirname "$MINGW_GENERATED_CRT_H")" \
    "$@"

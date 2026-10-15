#!/usr/bin/env bash
set -euo pipefail

python="$1"
test_script="$2"
modules_file="$3"
shift 3

# The modules are copied out of their Bazel output directory, which breaks their
# relative rpaths, so point the loader at the shared libraries passed as arguments.
lib_dirs=""
for lib in "$@"; do
    case "${lib}" in
        *.so | *.so.* | *.dylib) lib_dirs="${lib_dirs:+${lib_dirs}:}$(cd "$(dirname "${lib}")" && pwd)" ;;
    esac
done

LD_LIBRARY_PATH="${lib_dirs}" DYLD_LIBRARY_PATH="${lib_dirs}" exec "${python}" -I "${test_script}" "${modules_file}"

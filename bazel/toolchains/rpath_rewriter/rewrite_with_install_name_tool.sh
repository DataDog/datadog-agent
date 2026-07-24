#!/usr/bin/env bash

set -euo pipefail

HELPER=$1
INSTALL_NAME_TOOL=$2
OTOOL=$3
PREFIX=$4
INPUT=$5
OUTPUT=$6

source "$HELPER"

# PREFIX is the full rpath, or a relative path signaled as ./<suffix>.#rewrite_with_install_name_tool.sh
# In the relative case, ./../lib becomes @loader_path/../lib.
PREFIX=$(origin_rpath "@loader_path" "$PREFIX")

cp "$INPUT" "$OUTPUT"
# Restore owner-write so install_name_tool can modify dylibs installed as
# read-only by their build system (e.g. Python lib-dynload modules).
chmod u+w "$OUTPUT"

# Match Linux patchelf --set-rpath semantics: the packaged output should have
# exactly the packaging rpath, to avoid leaving in existing ones that may
# point to build-time paths.
"$INSTALL_NAME_TOOL" -delete_all_rpaths -add_rpath "$PREFIX" "$OUTPUT"
dylib_name=$(basename "$OUTPUT")
new_id="@rpath/$dylib_name"

"$INSTALL_NAME_TOOL" -id "$new_id" "$OUTPUT"

# Dylibs built in the Bazel sandbox record their dependencies with absolute
# sandbox paths as install names (e.g. bazel-out/.../libfoo.dylib). Those paths
# vanish after the build, so rewrite them to @rpath/<basename> so the dynamic
# linker can find them via the rpath we just added. Leave everything else
# (system libraries, @rpath/... references) untouched.
"$OTOOL" -L "$OUTPUT" | tail -n +2 | awk '{print $1}' | while read -r dep; do
    if [[ "$dep" == *"sandbox"* ]] || [[ "$dep" == *"bazel-out"* ]]; then
        dep_name=$(basename "$dep")
        new_dep="@rpath/$dep_name"
        "$INSTALL_NAME_TOOL" -change "$dep" "$new_dep" "$OUTPUT" 2>/dev/null || true
    fi
done

# Re-sign with an ad-hoc signature after modification as install_name_tool invalidates
# any existing code signature.
/usr/bin/codesign --sign - --force "$OUTPUT"

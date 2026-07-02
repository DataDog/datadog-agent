#!/usr/bin/env bash

set -euo pipefail

OTOOL=$1
PREFIX=$2
INPUT=$3
OUTPUT=$4

# PREFIX is the full rpath (e.g. {install_dir}/embedded/lib); use as-is, do not append /lib
cp "$INPUT" "$OUTPUT"
# Restore owner-write so install_name_tool can modify dylibs installed as
# read-only by their build system (e.g. Python lib-dynload modules).
chmod u+w "$OUTPUT"

# Match Linux patchelf --set-rpath semantics: the packaged output should have
# exactly the packaging rpath, to avoid leaving in existing ones that may
# point to build-time paths.
${OTOOL} -l "$OUTPUT" | awk '
    $1 == "cmd" && $2 == "LC_RPATH" { in_rpath = 1; next }
    in_rpath && $1 == "path" { print $2; in_rpath = 0 }
' | while read -r rpath; do
    install_name_tool -delete_rpath "$rpath" "$OUTPUT" 2>/dev/null || true
done
install_name_tool -add_rpath "$PREFIX" "$OUTPUT" 2>/dev/null || true
dylib_name=$(basename "$OUTPUT")
new_id="$PREFIX/$dylib_name"

install_name_tool -id "$new_id" "$OUTPUT"

# Dylibs built in the Bazel sandbox record their dependencies with absolute
# sandbox paths as install names (e.g. bazel-out/.../libfoo.dylib). Those paths
# vanish after the build, so rewrite them to $PREFIX/<basename> so the dynamic
# linker can find them via the rpath we just added. Leave everything else
# (system libraries, @rpath/... references) untouched.
${OTOOL} -L "$OUTPUT" | tail -n +2 | awk '{print $1}' | while read -r dep; do
    if [[ "$dep" == *"sandbox"* ]] || [[ "$dep" == *"bazel-out"* ]]; then
        dep_name=$(basename "$dep")
        new_dep="$PREFIX/$dep_name"
        install_name_tool -change "$dep" "$new_dep" "$OUTPUT" 2>/dev/null || true
        install_name_tool -add_rpath "$PREFIX" "$dep" 2>/dev/null || true
    fi
done

# Re-sign with an ad-hoc signature after modification as install_name_tool invalidates
# any existing code signature.
codesign --sign - --force "$OUTPUT"

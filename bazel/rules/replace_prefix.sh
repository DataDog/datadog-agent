#!/usr/bin/env bash

set -euo pipefail

while [ "$#" -gt 0 ]; do
    case "$1" in
        --prefix | -p)
            shift
            PREFIX="$1"
            ;;
        --patchelf)
            shift
            PATCHELF="$1"
            ;;
        *)
            break
    esac
    shift
done

if [ -z "$PREFIX" ]; then
    echo "Usage: replace_prefix.sh -p <new prefix> <input file>"
    exit 1
fi

patch_text_file() {
    f="$1"
    sed -ibak -e "s|^prefix=.*|prefix=$PREFIX|" -e "s|##PREFIX##|$PREFIX|" -e "s|\${EXT_BUILD_DEPS}|$PREFIX|" "$f" && rm -f "${f}bak"
}

for f in "$@"; do
    if [ ! -f "$f" ]; then
        echo "$f: file not found"
        exit 2
    fi
    # We don't want to process symlinks but rather the actual file it's pointing to
    # Otherwise `file $f` would return that it's a symlink, not an elf/mach-o file
    if [ -L "$f" ]; then
        f=$(realpath "$f")
    fi

    case $f in
        *.pc)
            patch_text_file "$f"
            ;;
        *)
            if file "$f" | grep -q ELF; then
                ${PATCHELF} --force-rpath --set-rpath "$PREFIX"/lib "$f"
            elif file "$f" | grep -q "Mach-O"; then
                # Handle macOS binaries (executables and other Mach-O files)
                install_name_tool -add_rpath "$PREFIX/lib" "$f" 2>/dev/null || true
                # Get the old install name/ID
                dylib_name=$(basename "$f")
                new_id="$PREFIX/lib/$dylib_name"

                # Change the dylib's own ID
                install_name_tool -id "$new_id" "$f"

                # Update all dependency paths that point to sandbox locations
                otool -L "$f" | tail -n +2 | awk '{print $1}' | while read -r dep; do
                    if [[ "$dep" == *"sandbox"* ]] || [[ "$dep" == *"bazel-out"* ]]; then
                        dep_name=$(basename "$dep")
                        new_dep="$PREFIX/lib/$dep_name"
                        install_name_tool -change "$dep" "$new_dep" "$f" 2>/dev/null || true
                        install_name_tool -add_rpath "$PREFIX/lib" "$dep" 2>/dev/null || true
                    fi
                done
            elif file "$f" | grep -q "ASCII text executable"; then
                patch_text_file "$f"
            else
                >&2 echo "Ignoring $f"
            fi
    esac
done

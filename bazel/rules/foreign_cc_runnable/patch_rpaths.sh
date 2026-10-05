#!/bin/bash
# Materialize a runnable copy of $INPUT in $OUTPUT, rewriting rpaths on the
# files listed in $MANIFEST. On Linux each rpath entry is prefixed with
# `$ORIGIN/<ups>`; on macOS with `@loader_path/<ups>`.
#
# Manifest format (tab-separated, one instruction per line):
#   FILE\t<tree-relative path>
#   GLOB\t<tree-relative pattern>
#
# A FILE must exist; a GLOB must match at least one file.
set -euo pipefail

PATCHELF=""
INSTALL_NAME_TOOL=""

while [ "$#" -gt 0 ]; do
    case "$1" in
        --patchelf)
            shift
            PATCHELF="$1"
            ;;
        --install-name-tool)
            shift
            INSTALL_NAME_TOOL="$1"
            ;;
        *)
            break
            ;;
    esac
    shift
done

if [ "$#" -lt 4 ]; then
    echo "Usage: patch_rpaths.sh [--patchelf <path>|--install-name-tool <path>] <platform> <input> <output> <manifest> [rpath dirs...]" >&2
    exit 1
fi

PLATFORM="$1"
INPUT="$2"
OUTPUT="$3"
MANIFEST="$4"
shift 4
RPATH_DIRS=("$@")

# `cp -rL` materializes a real copy and dereferences any symlinks that
# rules_foreign_cc may have left in the install tree. The `/.` on $INPUT
# copies its contents rather than the directory itself.
cp -rL "$INPUT/." "$OUTPUT/"

patch_file() {
    local f="$1"
    local rel="${f#"$OUTPUT"/}"
    # Number of parent directories between $f and $OUTPUT (= number of slashes)
    local slashes="${rel//[^\/]/}"
    local depth=${#slashes}

    local ups=""
    for ((i = 0; i < depth; i++)); do
        ups="${ups}../"
    done

    if [[ "$PLATFORM" == "darwin" ]]; then
        "$INSTALL_NAME_TOOL" -delete_all_rpaths "$f"
        for dir in "${RPATH_DIRS[@]}"; do
            "$INSTALL_NAME_TOOL" -add_rpath "@loader_path/${ups}${dir}" "$f"
        done
        # Re-sign with an ad-hoc signature; install_name_tool invalidates any existing code signature.
        codesign --sign - --force "$f" 2>/dev/null
    else
        local rpath=""
        for dir in "${RPATH_DIRS[@]}"; do
            rpath+="${rpath:+:}\$ORIGIN/${ups}${dir}"
        done
        "$PATCHELF" --set-rpath "$rpath" --force-rpath "$f"
    fi
}

while IFS=$'\t' read -r kind path; do
    case "$kind" in
        FILE)
            target="$OUTPUT/$path"
            if [[ ! -e "$target" ]]; then
                echo "patch_rpaths: FILE '$path' not found in tree" >&2
                exit 1
            fi
            patch_file "$target"
            ;;
        GLOB)
            count=0
            while IFS= read -r -d '' f; do
                patch_file "$f"
                count=$((count + 1))
            done < <(find "$OUTPUT" -path "$OUTPUT/$path" -type f -print0)
            if [[ $count -eq 0 ]]; then
                echo "patch_rpaths: GLOB '$path' matched no files" >&2
                exit 1
            fi
            ;;
        *)
            echo "Unknown manifest instruction kind: $kind" >&2
            exit 1
            ;;
    esac
done < "$MANIFEST"

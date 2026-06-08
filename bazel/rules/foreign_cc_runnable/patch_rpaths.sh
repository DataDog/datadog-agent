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

PLATFORM="$1"
PATCHELF="$2"
INPUT="$3"
OUTPUT="$4"
MANIFEST="$5"
shift 5
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
        for dir in "${RPATH_DIRS[@]}"; do
            install_name_tool -add_rpath "@loader_path/${ups}${dir}" "$f"
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

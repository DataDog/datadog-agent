#!/usr/bin/env bash

# Shared helpers for rpath rewriter toolchains. RPATH values beginning with
# `./` are relative placeholders produced by rewrite_rpaths; the platform-specific
# origin token is substituted here. Same-directory rpaths are represented as `./`.

origin_rpath() {
    local origin="$1"
    local rpath="$2"

    if [[ "$rpath" != ./* ]]; then
        printf '%s\n' "$rpath"
        return
    fi

    local suffix="${rpath#./}"
    printf '%s\n' "$origin${suffix:+/$suffix}"
}

origin_rpath_for_tree_file() {
    local origin="$1"
    local output_root="$2"
    local file="$3"
    local rpath="$4"

    if [[ "$rpath" != ./* ]]; then
        printf '%s\n' "$rpath"
        return
    fi

    local suffix="${rpath#./}"

    local rel="${file#"$output_root"/}"
    local slashes="${rel//[^\/]/}"
    local depth=${#slashes}
    local ups=""
    for ((i = 0; i < depth; i++)); do
        ups="${ups}../"
    done

    local relative="${ups}${suffix}"
    relative="${relative%/}"
    printf '%s\n' "$origin${relative:+/$relative}"
}

# Whether an rpath can affect what the file loads: it already has one, depends
# on a library that isn't system provided, or calls dlopen.
needs_rpath() {
    local patchelf="$1"
    local readelf="$2"
    local file="$3"

    local rpath needed symbols
    rpath="$("$patchelf" --print-rpath "$file")" || exit 1
    needed="$("$patchelf" --print-needed "$file")" || exit 1
    symbols="$("$readelf" --dyn-syms --wide "$file")" || exit 1

    if [ -n "$rpath" ]; then
        return 0
    fi

    # glibc and GCC runtime libraries, always found in the default search path.
    if [ -n "$needed" ] && printf '%s\n' "$needed" | grep -vE '^((libc|libm|libdl|libpthread|librt|libutil|libgcc_s)\.so\.[0-9]+|libstdc\+\+\.so\.[0-9]+|ld-linux.*)$' > /dev/null; then
        return 0
    fi

    printf '%s\n' "$symbols" | grep -E ' UND (dlopen|dlmopen)(@|$)' > /dev/null
}

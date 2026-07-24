#!/usr/bin/env bash

# Shared helpers for rpath rewriter toolchains. RPATH values beginning with
# `.` are relative placeholders produced by rewrite_rpaths; the platform-specific
# origin token is substituted here.

origin_rpath() {
    local origin="$1"
    local rpath="$2"

    if [[ "$rpath" != ./* ]]; then
        printf '%s\n' "$rpath"
        return
    fi

    local suffix="${rpath#./}"
    suffix="${suffix%/}"
    if [[ "$suffix" == "." ]]; then
        suffix=""
    fi

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
    suffix="${suffix%/}"
    if [[ "$suffix" == "." ]]; then
        suffix=""
    fi

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

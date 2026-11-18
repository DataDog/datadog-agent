#!/usr/bin/env bash
# Source this file to select changed-file arguments for the calling lint task.
# The caller consumes the array in the same shell as before_script.
# shellcheck disable=SC2034

CHANGED_FILES_ARGS=(--all-files)
if [ "${CI_COMMIT_BRANCH:-}" = "$COMPARE_TO_BRANCH" ]; then
    changed_files_base="${CI_COMMIT_BEFORE_SHA:-HEAD^}"
    if [ "$changed_files_base" = "0000000000000000000000000000000000000000" ]; then
        changed_files_base=HEAD^
    fi
    # A previous tip can be unreachable from every ref after a force-push.
    if git cat-file -e "${changed_files_base}^{commit}" 2>/dev/null || {
        [ "$changed_files_base" != 'HEAD^' ] &&
            git fetch --no-tags origin "$changed_files_base" &&
            git cat-file -e "${changed_files_base}^{commit}" 2>/dev/null
    }; then
        CHANGED_FILES_ARGS=(--base "$changed_files_base")
    elif git cat-file -e 'HEAD^' 2>/dev/null; then
        printf '%s\n' 'The previous pipeline tip is unavailable; comparing only HEAD^ to HEAD, which may omit earlier changes in the push.' >&2
        CHANGED_FILES_ARGS=(--base 'HEAD^')
    else
        printf '%s\n' 'HEAD has no parent; checking all eligible files introduced by the root commit.' >&2
    fi
else
    git fetch --no-tags origin "$COMPARE_TO_BRANCH"
    changed_files_base=$(git merge-base FETCH_HEAD HEAD)
    CHANGED_FILES_ARGS=(--base "$changed_files_base")
fi
unset changed_files_base

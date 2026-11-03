#!/usr/bin/env bash
# Usage: pre-push-changes.sh REGEX
# Exits 0 when the pushed commit changes a file matching REGEX since it forked from
# the release.json base branch, 1 otherwise. Diffing against the base branch instead
# of the remote tip keeps files brought in by merging the base branch out of the list.
# Any failure exits 0 so the calling hook still runs; callers must skip only on 1.
ref=${PRE_COMMIT_TO_REF:-HEAD}
base=$(git show "$ref:release.json" | sed -n 's/.*"base_branch": *"\([^"]*\)".*/\1/p') || exit 0
[ -n "$base" ] || exit 0
# Explicit refspec: single-branch clones would otherwise only update FETCH_HEAD.
git rev-parse -q --verify "origin/$base" >/dev/null ||
    git fetch -q origin "refs/heads/$base:refs/remotes/origin/$base" || exit 0
changed=$(git diff --name-only "origin/$base...$ref") || exit 0
grep -qE "$1" <<<"$changed"

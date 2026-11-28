#!/usr/bin/env bash
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.
#
# Copy the working tree into a throwaway detached worktree under .worktrees/ and print its absolute path: HEAD, plus the
# staged and unstaged changes to tracked files (modes and binary files included), plus the untracked files that aren't
# ignored. Nothing is written to the repository's object store, its index or the original files. Exits non-zero,
# printing nothing on stdout and removing the partial copy, when any step fails.
set -euo pipefail

top=$(git rev-parse --show-toplevel)
cd "$top"
dir=".worktrees/write-test-$(date +%Y%m%d%H%M%S)-$$"
patch=$(mktemp)
index=$(mktemp)
cleanup() {
    rm -f "$patch" "$index"
}
trap cleanup EXIT

# The trap comes first, so a failure inside worktree add removes what it registered. Hooks are off for the copy: a
# post-checkout hook would run the contributor's tooling in it, and its failure would fail the copy.
# Only this run's copy is removed: a repository-wide prune would drop other worktrees' registrations too.
trap 'git worktree remove --force "$dir" >/dev/null 2>&1 || echo "copy_worktree: remove $top/$dir by hand" >&2; cleanup' ERR
git -c core.hooksPath=/dev/null worktree add --quiet --detach "$dir" HEAD

# The diff reads a copy of the index: even with --no-optional-locks, it can refresh the stat cache of the index it reads.
# Explicit prefixes, context and no external diff or color: diff.noprefix, diff.context=0, diff.external or
# color.diff=always in the contributor's configuration would otherwise produce a patch git apply rejects
cp "$(git rev-parse --git-path index)" "$index"
GIT_INDEX_FILE=$index git --no-optional-locks diff --binary --no-color --no-ext-diff --unified=3 --src-prefix=a/ --dst-prefix=b/ \
    HEAD > "$patch"
if [[ -s $patch ]]; then
    git -C "$dir" apply "$patch"
fi
git ls-files -z --others --exclude-standard | tar --null -T - -cf - | tar -C "$dir" -xf -
echo "$top/$dir"

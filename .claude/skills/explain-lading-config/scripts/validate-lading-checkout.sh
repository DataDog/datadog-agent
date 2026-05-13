#!/usr/bin/env bash
# Validate that ~/dd/lading is a usable lading checkout.
#
# Exits 0 with the checkout's current branch printed on stdout when usable.
# Exits 1 with a suggested `git clone` command on stderr when the checkout
# is missing or not a git repo.
#
# Callers should print the stdout line to the user so they know which branch
# explanations will be grounded in, and warn if the branch is not `main`.

set -euo pipefail

LADING_DIR="${LADING_DIR:-$HOME/dd/lading}"

if [[ ! -d "$LADING_DIR" ]]; then
    cat >&2 <<EOF
lading checkout not found at $LADING_DIR

Clone it with:
  git clone git@github.com:DataDog/lading.git "$LADING_DIR"
EOF
    exit 1
fi

if ! git -C "$LADING_DIR" rev-parse --git-dir >/dev/null 2>&1; then
    echo "lading checkout at $LADING_DIR is not a git repo" >&2
    exit 1
fi

branch="$(git -C "$LADING_DIR" branch --show-current)"
echo "$branch"

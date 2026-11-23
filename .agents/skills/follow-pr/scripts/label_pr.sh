#!/usr/bin/env bash
# Usage: check_pr.sh <ref>
# Adds the `follow-pr` label to the open PR for <ref>, trying every authenticated gh account.
# Prints a single `ok:` or `skipped:` line and exits 0 unless called incorrectly.
set -euo pipefail

ref="${1:?usage: check_pr.sh <ref>}"
label="follow-pr"
case "$ref" in
    main | master | [0-9]*.[0-9]*.x)
        echo "skipped: $ref is a shared branch"
        exit 0
        ;;
esac
repo=$(git remote get-url origin | sed -E 's#^(git@|https://)github\.com[:/]##; s#\.git$##')

run_gh() {
    if [ -n "$token" ]; then GH_TOKEN="$token" gh "$@"; else gh "$@"; fi
}

accounts=$(gh auth status --json hosts --jq '.hosts["github.com"][].login' 2>/dev/null || true)
for account in "" $accounts; do
    token=""
    if [ -n "$account" ]; then
        token=$(gh auth token --user "$account" 2>/dev/null) || continue
    fi
    pr=$(run_gh pr view "$ref" --repo "$repo" --json number,state,labels \
        --jq "\"\(.number) \(.state) \(any(.labels[]; .name == \"$label\"))\"" 2>/dev/null) || continue
    read -r number state labeled <<<"$pr"

    if [ "$state" != "OPEN" ]; then
        echo "skipped: PR #$number is $state"
    elif [ "$labeled" = "true" ]; then
        echo "ok: PR #$number already has the $label label"
    elif run_gh pr edit "$number" --repo "$repo" --add-label "$label" >/dev/null 2>&1; then
        echo "ok: added the $label label to PR #$number"
    else
        echo "skipped: could not add the $label label to PR #$number"
    fi
    exit 0
done

echo "skipped: no PR found for $ref in $repo with any gh account"

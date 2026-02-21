#!/usr/bin/env bash
set -euo pipefail

# Check that a commit message (and optionally its associated PR) do not contain
# CI-skip directives.  When run in CI the script expects GITHUB_TOKEN to be set
# so it can query the GitHub API for PR metadata.
#
# Usage:
#   check_skip_ci.sh <commit_sha>          # in CI (uses GITHUB_TOKEN)
#   GITHUB_TOKEN=ghp_... check_skip_ci.sh <commit_sha>  # locally

COMMIT_SHA="${1:-HEAD}"
REPO="DataDog/datadog-agent"
SKIP_CI_PATTERN='\[(ci skip|skip ci|actions skip|skip actions|no ci)\]|skip-checks:\s*true'

# --- 1. Check the commit message -------------------------------------------

commit_message="$(git log --format=%B -n 1 "$COMMIT_SHA")"

if echo "$commit_message" | grep -qEi "$SKIP_CI_PATTERN"; then
    echo "error: The commit message contains ci skip tags." >&2
    echo "Do not skip checks when merging PRs, please change the description of the merge commit." >&2
    exit 1
fi

# --- 2. Check the PR title and body ----------------------------------------

# Extract PR number from the commit subject line (format: "title (#1234)")
PR_NUMBER=$(echo "$commit_message" | head -1 | grep -oE '\(#[0-9]+\)' | grep -oE '[0-9]+' | tail -n 1)

if [ -z "$PR_NUMBER" ]; then
    echo "info: No PR number found in commit message, skipping PR metadata check."
    echo "success: No ci skip tags found."
    exit 0
fi

if [ -z "${GITHUB_TOKEN:-}" ]; then
    echo "warning: GITHUB_TOKEN is not set, skipping PR metadata check." >&2
    echo "success: No ci skip tags found in commit message (PR metadata not checked)."
    exit 0
fi

echo "info: Checking PR #${PR_NUMBER} title and body for ci skip tags..."

pr_json=$(curl -sf -H "Authorization: token ${GITHUB_TOKEN}" \
    "https://api.github.com/repos/${REPO}/pulls/${PR_NUMBER}" || true)

if [ -z "$pr_json" ]; then
    echo "warning: Could not fetch PR #${PR_NUMBER} from GitHub API, skipping PR metadata check." >&2
    echo "success: No ci skip tags found in commit message (PR metadata not checked)."
    exit 0
fi

pr_title=$(printf '%s' "$pr_json" | python3 -c "import sys,json; print(json.load(sys.stdin).get('title',''))" 2>/dev/null || true)
pr_body=$(printf '%s' "$pr_json" | python3 -c "import sys,json; print(json.load(sys.stdin).get('body','') or '')" 2>/dev/null || true)

if echo "$pr_title" | grep -qEi "$SKIP_CI_PATTERN"; then
    echo "error: The PR title (#${PR_NUMBER}) contains ci skip tags. Please update the PR title before merging." >&2
    exit 1
fi

if echo "$pr_body" | grep -qEi "$SKIP_CI_PATTERN"; then
    echo "error: The PR description (#${PR_NUMBER}) contains ci skip tags. Please update the PR description before merging." >&2
    exit 1
fi

echo "success: No ci skip tags found."

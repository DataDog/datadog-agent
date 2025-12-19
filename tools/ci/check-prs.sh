#!/bin/bash
# check-prs.sh - Triage open PRs for unanswered human comments
#
# Usage: ./tools/ci/check-prs.sh [PR_NUMBER...]
#
# If no PR numbers provided, fetches all non-draft open PRs for the current user.
# Filters out bot comments and automated reports, showing only human activity.

set -euo pipefail

# Bot accounts to filter out
BOTS="agent-platform-auto-pr|cit-pr-commenter|datadog-official|dd-octo-sts"

# Patterns in comment bodies to filter out (case-insensitive)
NOISE_PATTERNS="Go Package Import Differences|Static quality checks|GitLab CI Configuration Changes|Regression Detector|bits_ai_status|graphite.dev"

# Get current GitHub username
ME=$(gh api user --jq '.login')

get_pr_info() {
    local pr_number=$1
    gh pr view "$pr_number" --json number,title,url,reviewDecision,reviewRequests,comments,reviews | jq --arg me "$ME" --arg bots "$BOTS" --arg noise "$NOISE_PATTERNS" '
    {
        pr: .number,
        title: .title,
        url: .url,
        reviewDecision: .reviewDecision,
        pendingReviewers: [.reviewRequests[] | if .name then .name else .login end],
        comments: [
            .comments[] |
            select(
                (.author.login | test($bots)) | not
            ) |
            select(
                .author.login != $me
            ) |
            select(
                (.author.login | startswith("graphite")) | not
            ) |
            select(
                (.body | test($noise; "i")) | not
            ) |
            {author: .author.login, body: .body[0:200]}
        ],
        reviews: [
            .reviews[] |
            select(.state == "CHANGES_REQUESTED" or .state == "APPROVED" or (.body | length > 0)) |
            select((.author.login | test($bots)) | not) |
            select(.author.login != $me) |
            {author: .author.login, state: .state, body: .body[0:200]}
        ]
    }'
}

# If PR numbers provided as arguments, use those; otherwise fetch all non-draft PRs
if [ $# -gt 0 ]; then
    for pr in "$@"; do
        get_pr_info "$pr"
    done
else
    PR_NUMBERS=$(gh pr list --author @me --state open --json number,isDraft --jq '.[] | select(.isDraft == false) | .number')

    if [ -z "$PR_NUMBERS" ]; then
        echo "No open non-draft PRs found."
        exit 0
    fi

    echo "$PR_NUMBERS" | while read -r pr; do
        get_pr_info "$pr"
    done
fi

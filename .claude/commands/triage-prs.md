# PR Triage - Find Unanswered Human Comments

Find human comments on my open PRs that may need a response, filtering out automated bot noise.

## Command

```bash
./tools/ci/check-prs.sh
```

Or for specific PRs:
```bash
./tools/ci/check-prs.sh 44174 44088
```

## What it filters out

### Bot accounts:
- `agent-platform-auto-pr`
- `cit-pr-commenter`
- `datadog-official`
- `dd-octo-sts`
- Any account starting with `graphite`
- Your own comments

### Comment patterns (case-insensitive):
- "Go Package Import Differences"
- "Static quality checks"
- "GitLab CI Configuration Changes"
- "Regression Detector"
- "bits_ai_status"
- "graphite.dev"

## Output format

For each PR, report:
- PR number, title, and URL
- Review status: APPROVED, CHANGES_REQUESTED, or REVIEW_REQUIRED
- Pending reviewers (teams or individuals still needed)
- Any human comments needing response (author + truncated body)
- Any reviews with state CHANGES_REQUESTED or APPROVED (author + state + truncated body)

### Priority order:
1. PRs with CHANGES_REQUESTED - need action from you
2. PRs with unanswered human comments - need response
3. PRs with REVIEW_REQUIRED - waiting on others
4. PRs with APPROVED and no pending reviewers - ready to merge

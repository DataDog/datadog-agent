---
name: review-pr-comments
description: Review and triage PR review comments for the current branch using gh CLI. Fetches review comments, groups them into threads, separates human from bot feedback, and walks through each unresolved comment interactively. Use when the user asks to check PR comments, review feedback, address review comments, or triage PR reviews.
---

# Review PR Comments

Fetch, summarize, and interactively triage review comments on the PR for the current git branch.

## Prerequisites

- `gh` CLI authenticated with access to the repository
- Current branch must have an open PR (not `main` or a release branch like `N.N.x`)

## Workflow

### Step 1: Identify the PR

```bash
BRANCH=$(git branch --show-current)
```

Guard: skip if branch is `main`, `master`, or matches release pattern `^\d+\.\d+\.x$`.

Find the PR:

```bash
gh pr list --head "$BRANCH" --json number,title,url,state --limit 5
```

If no PR found, inform the user and stop.

### Step 2: Fetch all review data

Run these three `gh api` calls in parallel:

```bash
# Review comments (inline code comments)
gh api repos/{owner}/{repo}/pulls/{PR_NUMBER}/comments --paginate \
  --jq '.[] | {id, path, line, body, user: .user.login, created_at, in_reply_to_id, diff_hunk}'

# Top-level reviews (approval/request-changes status + body)
gh api repos/{owner}/{repo}/pulls/{PR_NUMBER}/reviews --paginate \
  --jq '.[] | {id, user: .user.login, state, body}'

# Issue-level comments (general PR discussion, CI bot reports)
gh api repos/{owner}/{repo}/issues/{PR_NUMBER}/comments --paginate \
  --jq '.[] | {id, user: .user.login, body, created_at}'
```

Extract `{owner}` and `{repo}` from `gh repo view --json owner,name`.

### Step 3: Organize into threads

Group review comments into conversation threads using `in_reply_to_id`:
- A comment with `in_reply_to_id: null` starts a new thread
- Replies link to their parent thread

For each thread, track:
- **File & line** from the root comment
- **Author** of the root comment
- **All replies** in chronological order
- **Resolution status**: resolved if the PR author replied last

### Step 4: Classify comments

Separate into two groups:

**Bot comments** (user login ends with `[bot]` or is a known CI bot):
- Summarize briefly (e.g., "CI quality gates passed", "Regression detector: no regressions")
- Only highlight actionable items (failures, warnings)

**Human reviewer comments**:
- These get full treatment in Step 5

### Step 5: Present and triage each human comment

For each unresolved human reviewer comment thread:

1. **Show context**: file, line, diff hunk, the comment body
2. **Read the current file** at the referenced location to understand current state
3. **Summarize** the reviewer's concern in one sentence
4. **Propose** a concrete action: code fix, reply text, or explanation
5. **Ask the user** what to do using AskQuestion:

```
Options:
- "Fix" → Apply the proposed code change
- "Reply" → Post a reply comment on the PR via gh api
- "Reply with custom message" → Ask user for reply text, then post
- "Add TODO" → Add a TODO item to track for later
- "Skip" → Move to next comment
```

For already-resolved threads (PR author replied last), present them as a summary group:
- Show the comment + your reply, note it's resolved
- Ask if user wants to revisit any

### Step 6: Execute chosen actions

**Fix**: Edit the file, show the diff to the user. Do NOT commit automatically.

**Reply**: Post via gh API:

```bash
gh api repos/{owner}/{repo}/pulls/{PR_NUMBER}/comments \
  -f body="$REPLY_TEXT" -f in_reply_to=$COMMENT_ID
```

**TODO**: Use the TodoWrite tool to track it.

### Step 7: Summary

After processing all comments, output a summary table:

```
| # | File | Reviewer | Action Taken |
|---|------|----------|--------------|
| 1 | path/to/file.go:42 | reviewer1 | Fixed |
| 2 | path/to/other.py:10 | reviewer2 | Replied |
| 3 | path/to/build.bzl:5 | reviewer3 | Skipped (already resolved) |
```

## Edge Cases

- **No unresolved comments**: Report "All review comments are resolved" with approval status summary
- **PR not found**: Suggest the user push the branch or create a PR first
- **Outdated diff hunks**: Note that the comment may reference old code; read the current file to verify
- **Multiple PRs for branch**: Use the most recent open one

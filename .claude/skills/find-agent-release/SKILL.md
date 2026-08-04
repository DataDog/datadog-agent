---
name: find-agent-release
description: Find which Agent release(s) contain a given commit or PR. Use when the user asks whether a fix/change is in a release, which version ships a commit, or whether a PR was backported.
allowed-tools: Bash
argument-hint: "<commit-sha | PR-number | PR-URL>"
---

# Find Release for Commit or PR

Determine which Agent release tags and release branches contain a given commit or PR.

## Input

`$ARGUMENTS` is one of:
- A full or short commit SHA (e.g. `c0d4450e5`, `c0d4450e57ecb9cf781866c4e908856142461263`)
- A PR number (e.g. `49109`)
- A GitHub PR URL (e.g. `https://github.com/DataDog/datadog-agent/pull/49109`)

## Steps

### Step 1 — Resolve to a commit SHA

If `$ARGUMENTS` looks like a PR number or URL, extract the PR number and fetch the merge commit:

```bash
gh pr view <PR_NUMBER> --repo DataDog/datadog-agent \
  --json number,title,state,mergedAt,mergeCommit,baseRefName
```

- If state is not `MERGED`, report "PR #N is not merged yet — it is not in any release" and stop.
- The commit SHA to use is `mergeCommit.oid`.

If `$ARGUMENTS` looks like a commit SHA (hex string, 7–40 chars), use it directly. Verify it exists:

```bash
git cat-file -t <SHA>
```

If not found, report "Commit not found in local repo — try `git fetch --all` first" and stop.

### Step 2 — Find containing release tags (parallel)

Run both lookups at the same time:

```bash
# All release tags containing the commit (vX.Y.Z and X.Y.Z patterns)
git tag --contains <SHA> | grep -E '^[0-9]+\.[0-9]+\.[0-9]+' | sort -V

# All remote release branches containing the commit
git branch -r --contains <SHA> | grep -E 'origin/[0-9]+\.[0-9]+\.x$' | sed 's|.*origin/||' | sort -V
```

### Step 3 — Check for backport PRs (if no release branch found)

If Step 2 found no matching release branches, search for a backport PR:

```bash
gh pr list --repo DataDog/datadog-agent \
  --search "backport-<PR_NUMBER>" \
  --json number,title,state,baseRefName,mergedAt \
  --limit 20
```

Also try searching by the original commit message or title if the PR title is known.

### Step 4 — Check current milestone

Read the current milestone from `release.json` to understand what's upcoming:

```bash
python3 -c "import json; d=json.load(open('release.json')); print('current_milestone:', d['current_milestone']); print('last_stable:', d['last_stable'])"
```

### Step 5 — Report results

Present a clear summary:

#### If found in release tags:

```
PR #49109 / commit c0d4450e5 is included in these releases:
  Tags:     7.79.0-rc.1, 7.79.0-rc.2, 7.79.0 (stable)
  Branches: 7.79.x

First release: 7.79.0-rc.1
Latest stable: 7.79.0
```

#### If found in main only (not yet released):

```
Commit c0d4450e5 is on `main` but has NOT been released yet.
  Current milestone: 7.80.0
  Last stable:       7.78.1 (agent 7), 6.53.1 (agent 6)

  No backport PRs found for #49109.
  → This fix will ship in 7.80.0 unless a backport is created.
```

#### If backport PRs exist but not yet merged:

```
Commit c0d4450e5 is on `main` but NOT yet in a release.
  Backport PRs found:
    #50123 — [Backport 7.79.x] ... (OPEN, base: 7.79.x)

  → It will ship in 7.79.x once the backport merges.
```

#### If not on main either:

```
Commit <SHA> was not found in main or any release branch.
  It may be on an unmerged PR or a branch that was rebased.
  Try: gh pr list --search "<SHA>"
```

## Edge Cases

- **RC tags vs stable**: Distinguish RCs (`7.79.0-rc.1`) from stable (`7.79.0`). Clearly note if the commit is only in RCs so far.
- **Multiple majors**: The repo ships both agent 6 and 7. Note if only one major line has the commit (rare, but possible for backports).
- **No network**: If `gh` commands fail, fall back to git-only checks and note results may be incomplete.
- **Short SHA ambiguity**: If `git cat-file` fails on a short SHA, suggest using the full SHA.
- **Fetch reminder**: If `git branch -r --contains` returns nothing but the commit is on main, remind the user that local remote-tracking refs may be stale (`git fetch --all`).

## Example invocations

```
/find-agent-release 49109
/find-agent-release c0d4450e57ecb9cf781866c4e908856142461263
/find-agent-release https://github.com/DataDog/datadog-agent/pull/49109
```

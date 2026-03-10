# PR Workflow

Guide for implementing a ticket and creating a draft pull request.

**Read this when:** you are about to implement a ticket and open a PR.

---

## Implementation

Run the `/jira-ticket-solver` skill from the [Claude Marketplace](https://github.com/DataDog/claude-marketplace-gpt):

```
/jira-ticket-solver <JIRA-KEY> --headless
```

This skill handles: fetching full ticket context, codebase analysis, branching, implementation, lint, tests, commit, and PR creation.

### datadog-agent-specific constraints for jira-ticket-solver

**Branch naming:** `auto-jira/<KEY>-brief-description` (lowercase, hyphens only)

**PR title:** `[auto-jira][<KEY>] <description>`, e.g. `[auto-jira][AGENTCFG-456] handle nil pointer on shutdown`

**PR must be a draft** (`--draft` flag).

**Linter:** `dda inv linter.go --targets=./path/to/changed/package` must pass before the PR is created.

**Tests:** `dda inv test --targets=./path/to/changed/package` must pass.

**Reno release notes:** if the change touches Agent binary code, run `dda inv releasenotes.new-note` and include the note in the commit.

**PR body** must follow `.github/PULL_REQUEST_TEMPLATE.md` and include:
- What is changed
- Motivation (link to the Jira ticket: `https://datadoghq.atlassian.net/browse/<KEY>`)
- How changes were validated
- Attribution footer: `_Created by [Auto-JIRA](https://github.com/DataDog/datadog-agent/blob/main/.claude/skills/auto-jira/SKILL.md)._`

**Labels:**

| Situation | Labels |
|-----------|--------|
| Test/doc/CI-only | `changelog/no-changelog`, `qa/no-code-change` |
| Bug fix | `changelog/bug-fix`, `qa/done` |
| New feature | `changelog/new-feature`, `qa/done` |

**One focused commit per ticket.** Stage specific files only, never `git add -A`.

---

## After the PR is created

Once the draft PR exists with title and description:

1. POST-ACTION verify: `gh pr view <number> --json title,state,isDraft` — confirm `isDraft: true`
2. Add a comment on the Jira ticket with the PR link (see [ticket-workflow.md](ticket-workflow.md))
3. **Move on to the next ticket.** CI, review, and merge are left for humans.

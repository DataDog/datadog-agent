# auto-jira

Autonomously scans a Jira board backlog for eligible tickets and implements fixes as draft PRs. Once a draft PR is open, the skill moves on to the next ticket — CI, review, and merge are left for humans.

## Usage

```
/auto-jira <BOARD-KEY> [flags]
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `BOARD-KEY` | yes | — | Jira project key (e.g. `ACTP`, `AGENTCFG`, `CONTINT`) |
| `--max-cards N` | no | `3` | Maximum number of PRs to create in this run |
| `--before-date YYYY-MM-DD` | no | 30 days ago | Only consider tickets created before this date |
| `--exclude word1,word2` | no | none | Skip tickets whose title contains any of these words (case-insensitive, comma-separated) |

## Examples

```
# Scan the AGENTCFG board, up to 3 PRs (defaults)
/auto-jira AGENTCFG

# Up to 5 PRs
/auto-jira AGENTCFG --max-cards 5

# Only older tickets
/auto-jira AGENTCFG --before-date 2026-02-12

# Skip planning/spike tickets
/auto-jira CONTINT --max-cards 2 --exclude RFC,spike,design

# Full example
/auto-jira AGENTCFG --max-cards 5 --before-date 2026-02-12 --exclude nodetreemodel,viper
```

## What it does

For each eligible ticket (up to `--max-cards`), the skill:

1. Queries the board backlog for unassigned `To Do` tickets with no parent issue
2. Evaluates each ticket for feasibility — skips vague, docs-only, or already-fixed issues
3. Claims the ticket (assigns to you)
4. Implements the fix, runs lint and tests, creates a single commit
5. Opens a **draft PR** against `main` titled `[auto-jira][KEY] ...`
6. Posts the PR link as a comment on the Jira ticket

Progress is tracked locally in `AUTO_JIRA.md` (gitignored, never committed).

## Ticket eligibility

A ticket is eligible only if ALL of the following are true:

- Status is `To Do`
- Assignee is empty
- No parent issue
- Created before `--before-date`
- Label `do-not-autosolve` is NOT set
- Title contains none of the `--exclude` words
- Solvable by a code PR (not docs-only, not infra, not cross-team)
- Not already fixed in the codebase

To permanently exclude a ticket from auto-jira, add the `do-not-autosolve` label on the Jira issue.

## Running headlessly via clauded

For unattended/background operation, pass `ARGUMENTS` as an env var:

```bash
ARGUMENTS="AGENTCFG --max-cards 5 --before-date 2026-02-12" clauded -p .claude/skills/auto-jira/SKILL.md
```

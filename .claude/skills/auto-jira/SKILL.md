---
name: auto-jira
description: Autonomously work on Jira backlog tickets, creating PRs and shepherding them to merge
argument-hint: "<BOARD-KEY> [--max-cards N] [--before-date YYYY-MM-DD] [--exclude word1,word2]"
---

## Overview

This skill autonomously identifies Jira backlog tickets for a given board that can be worked on without human intervention, implements the code change, and creates draft PRs. Once a draft PR is open, the skill moves on to the next ticket.

**Arguments:** $ARGUMENTS

---

## CRITICAL: Serial Operations with Verification

For every write action:

1. **PRE-CHECK with Critic** — get APPROVED before proceeding
2. **Execute the action** — wait for completion
3. **POST-CHECK with Critic** — verify it actually happened
4. **Only then** — move to the next action

Write actions include: creating PRs, posting Jira comments, pushing commits, transitioning ticket status, assigning tickets.

---

## Work Tracking

**Track all work in `AUTO_JIRA.md`** (gitignored, local only):
- Current ticket being worked on
- Current PR (if any)
- CI status and what is blocking
- Skipped tickets with reasons

**NEVER commit this file.** Update it as you work, removing notes for completed work and keeping focus on in-progress state and next steps.

---

## Workflow Overview

```
1. PARSE ARGS        -> Extract board, max-cards, filters from $ARGUMENTS
2. SELECT TICKET     -> Find eligible ticket from board backlog
3. CLAIM TICKET      -> Assign to user + add to current sprint
4. IMPLEMENT         -> Code change, lint, tests, commit (via /jira-ticket-solver)
5. CREATE PR (DRAFT) -> Open draft PR against main
6. FIX PR TITLE      -> Rename to [auto-jira][<KEY>] <description> using gh pr edit
8. LINK & COMMENT    -> Post PR link on Jira ticket
9. NEXT              -> Go to step 2 until max-cards reached or no eligible tickets
```

---

## Supporting Documents

| When you need to...                              | Read this                                       |
|--------------------------------------------------|-------------------------------------------------|
| Select or claim a Jira ticket                    | [ticket-workflow.md](ticket-workflow.md)        |
| Create a PR                                      | [pr-workflow.md](pr-workflow.md)                |
| Evaluate a ticket that links to CI logs          | [ci-validation.md](ci-validation.md)            |
| Debug common issues                              | [troubleshooting.md](troubleshooting.md)        |

**Do NOT load all docs at once.** Only read what is relevant to the current task.

---

## Parse Arguments

Parse `$ARGUMENTS`:
- **BOARD** (required): Jira project key, e.g. `ACTP`, `AGENTCFG`, `CONTINT`
- **--max-cards N**: max PRs to create this run (default: `3`)
- **--before-date YYYY-MM-DD**: only tickets created before this date (default: 30 days ago)
- **--exclude word1,word2**: skip tickets whose title contains any of these words (case-insensitive)

If BOARD is missing, stop and ask the user.

Initialize `AUTO_JIRA.md`:
```markdown
# Auto-JIRA Run — <BOARD> — <TODAY>

## Configuration
- Board: <BOARD>
- Max cards: <N>
- Before date: <BEFORE_DATE>
- Exclude: <EXCLUDED_WORDS or "none">

## In Progress
_none_

## Skipped
_none_
```

---

## Jira Access

Use the **Atlassian MCP** tools as the primary method:

```
mcp__atlassian__searchJiraIssuesUsingJql   — query tickets
mcp__atlassian__getJiraIssue               — fetch ticket details
mcp__atlassian__editJiraIssue              — assign, update fields
mcp__atlassian__transitionJiraIssue        — change status
mcp__atlassian__addCommentToJiraIssue      — add comments
mcp__atlassian__getTransitionsForJiraIssue — list available transitions
mcp__atlassian__atlassianUserInfo          — get your account ID
```

All calls use `cloudId: datadoghq.atlassian.net`.

**If MCP authentication fails for any reason, STOP.** Mark the ticket HOLD and report the failure.

---

## Repository Location

Discover the repository root from the current working tree:
```bash
git rev-parse --show-toplevel
```

Always work from this directory. If this command fails (not in a git repo), stop and inform the user.

---

## Local Environment Setup

Local checks catch problems before CI. Do not skip or work around them:
- Linter failures → fix the issue, do not use `--no-verify`
- Missing tools → `dda inv install-tools`
- Pre-commit hook failures → fix the root cause

---

## Ticket Selection Criteria

**DO work on tickets that:**
- Status is `To Do`
- Assignee is empty
- No parent issue (epics as parents are okay; sub-tasks with a parent ticket are not)
- Created before `<BEFORE_DATE>`
- Label `do-not-autosolve` is NOT set
- Title contains none of the excluded words
- Clear, self-contained description with sufficient context to implement
- Solvable by a code PR to this repository
- Not already fixed in the codebase

**DO NOT work on tickets that:**
- Are assigned to someone else
- Require design decisions or cross-team coordination
- Touch authentication, authorization, cryptography, or secrets handling
- Are docs-only, infra/config, or process changes with no code change
- Have vague or missing acceptance criteria
- Are tracking/umbrella/epic-level tickets

**When uncertain → skip.** The goal is reliable autonomous completion, not ambitious attempts that get stuck.

**Check eligibility quickly** — do a codebase search to confirm the issue is not already fixed before starting any implementation work.

---

## Attribution

Every interaction must be clearly attributed as coming from Auto-JIRA.

| Platform             | Format |
|----------------------|--------|
| **Jira comment**     | End with: `— Auto-JIRA (https://github.com/DataDog/datadog-agent/blob/main/.claude/skills/auto-jira/SKILL.md)` |
| **GitHub PR body**   | Include: `_Created by [Auto-JIRA](https://github.com/DataDog/datadog-agent/blob/main/.claude/skills/auto-jira/SKILL.md)._` |
| **GitHub comment**   | End with: `_— [Auto-JIRA](https://github.com/DataDog/datadog-agent/blob/main/.claude/skills/auto-jira/SKILL.md)_` |
| **Commit**           | Co-author line: `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` |

---

## Human Intervention

When something requires human intervention:

1. Transition the Jira ticket to HOLD (do NOT change other fields)
2. Add a NEW comment on the ticket explaining exactly what is blocked
3. Update `AUTO_JIRA.md` with the blocker
4. Move to the next ticket (if any)

**Triggers:**
- Authentication failures (Jira MCP, GitHub)
- Unclear requirements that cannot be resolved from ticket context
- Security-sensitive changes discovered during implementation

---

## Completion Signals

**Print `TASK COMPLETED` only when ALL are true:**
- No more eligible tickets in the board backlog, OR
- `--max-cards` limit has been reached

**Print `TASK FAILED` only when:**
- Authentication is permanently broken and cannot be recovered

After completing one ticket, always check for more work before stopping.

---

## Critic Agent

Every write action needs PRE-CHECK and POST-CHECK.

**PRE-ACTION:**
```
Agent(
  prompt="""You are a Critic agent.

MODE: PRE-ACTION

PROPOSED ACTION:
[What you are about to do]

PROPOSED CONTENT:
[The exact message / comment / command]

Verify all factual claims and links. Respond APPROVED or REJECTED with evidence.""",
  subagent_type="general-purpose"
)
```

**POST-ACTION:**
```
Agent(
  prompt="""You are a Critic agent.

MODE: POST-ACTION

ACTION TAKEN:
[What was supposed to happen]

EXPECTED RESULT:
[What should be visible now]

Verify the action actually succeeded. Respond APPROVED or REJECTED with evidence.""",
  subagent_type="general-purpose"
)
```

If Critic rejects: fix the issues, try again, get a clean APPROVED before continuing.

---

## Important Constraints

1. **NEVER take a ticket assigned to someone else.** Check Assignee before claiming.
2. **One focused commit per ticket.** Do not create "fix CI" commits.
3. **Never skip hooks** (`--no-verify`) or force-push to `main`.
4. **Never use `git add -A` or `git add .`** — stage specific files by name.
5. **Always draft** — PRs are created as drafts and left for humans to take from there.
6. **Never commit `AUTO_JIRA.md`** — local tracking only.

---

## Quick Reference

| State       | When to use                             |
|-------------|-----------------------------------------|
| In Progress | Actively working on implementation      |
| HOLD        | Blocked, needs human intervention       |

Branch naming: `auto-jira/<KEY>-brief-description` (lowercase, hyphens only)

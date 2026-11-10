# Ticket Workflow

Guide for selecting, claiming, and managing Jira tickets for autonomous work.

**Read this when:** you are about to select or claim a ticket.

---

## Querying the Backlog

**First, look up the initial status for this board from `.ddqa/config.toml`:**

```bash
# Find the team section whose jira_project matches <BOARD>, then read jira_statuses[0]
# Example: for BOARD=NPM the initial status is "Backlog", not "To Do"
```

Read `.ddqa/config.toml`, find the `[teams."..."]` block where `jira_project = "<BOARD>"`, and use the first element of its `jira_statuses` array as `<INITIAL_STATUS>`.

If the board is not found in `.ddqa/config.toml`, default to `"To Do"` and log a warning in `AUTO_JIRA.md`.

Use `mcp__atlassian__search_issues` with:
- `jql`: the template below
- `max_results`: 50

The tool has no field selection; each result carries the server's default issue fields.

### JQL template

```
project = <BOARD>
  AND status = "<INITIAL_STATUS>"
  AND assignee is EMPTY
  AND "Parent" is EMPTY
  AND created <= "<BEFORE_DATE>"
  AND (labels is EMPTY OR labels != "do-not-autosolve")
ORDER BY created ASC
```

The JQL gets you candidates. You still must evaluate each one before claiming.

---

## Evaluating a Candidate

For each candidate, fetch full details:
```
mcp__atlassian__get_issue(issue_key="<KEY>")
```

Then answer:
1. **Is it already fixed?** Do a quick codebase search (Grep, Glob) before going further.
2. **Is the description clear enough?** Can you implement this without guessing?
3. **Is it self-contained?** No cross-team coordination, no design decisions, no security-sensitive areas?
4. **Is it a code change?** Not docs-only, not config/infra, not process.

**When skipping, record in `AUTO_JIRA.md`:**
```markdown
### Skipped
- **<KEY>**: <reason> (checked <date>)
```

---

## Claiming a Ticket

Once a ticket is selected, do BOTH of these steps:

### Step 1: Get your account ID

There is no "who am I" tool. Find an issue assigned to you:

```
mcp__atlassian__search_issues(
  jql="assignee = currentUser() ORDER BY updated DESC",
  max_results=1
)
```

Read the assignee's `accountId` from the result. If there is no result, or it does not include an account ID, STOP and ask the user for their Atlassian account ID: it is the last segment of their Jira profile URL (`https://datadoghq.atlassian.net/jira/people/<accountId>`).

Save your `accountId` in `AUTO_JIRA.md` for subsequent calls.

### Step 2: Assign to yourself

```
mcp__atlassian__update_issue(
  issue_key="<KEY>",
  fields={"assignee": {"accountId": "<YOUR_ACCOUNT_ID>"}}
)
```

This returns a preview. Check it, then repeat the call with `confirmed=true`.

POST-ACTION verify: this search must return the ticket:

```
mcp__atlassian__search_issues(jql="key = <KEY> AND assignee = currentUser()")
```

### Step 3: Add to the current sprint

Fetch your active sprint ID and move the ticket into it:

```
mcp__atlassian__search_issues(
  jql='project = <BOARD> AND sprint in openSprints() AND sprint not in futureSprints()',
  max_results=1
)
```

Extract the sprint ID from the result's sprint field (`customfield_10020`), then:

```
mcp__atlassian__update_issue(
  issue_key="<KEY>",
  fields={"customfield_10020": <SPRINT_ID>}
)
```

Check the preview, then repeat the call with `confirmed=true`.

**IMPORTANT:** Pass `<SPRINT_ID>` as a bare integer (e.g. `50028`), NOT as an object (`{"id": 50028}`). The API returns "Number value expected" if you pass an object.

POST-ACTION verify: this search must return the ticket:

```
mcp__atlassian__search_issues(jql="key = <KEY> AND sprint in openSprints()")
```

If no open sprint is found, the result does not include the sprint field, or the edit fails, log a warning in `AUTO_JIRA.md` and continue — this is non-blocking.

### Step 4: Add a start comment

```
mcp__atlassian__add_comment(
  issue_key="<KEY>",
  body="Starting autonomous implementation.\n\n— Auto-JIRA (https://github.com/DataDog/datadog-agent/blob/main/.agents/skills/auto-jira/SKILL.md)"
)
```

POST-ACTION verify: re-fetch the issue with `mcp__atlassian__get_issue` and confirm the comment appears.

Update `AUTO_JIRA.md`:
```markdown
## In Progress
- **<KEY>**: <summary>
  - Status: claimed, starting implementation
```

---

## Reading Ticket Context

Before implementing, thoroughly read:
1. **Description** — full requirements
2. **Acceptance Criteria** — what defines "done"
3. **Comments** — any clarifications or context from the team
4. **Linked issues** — related tickets that provide context

Fetch any linked URLs in the description:
- GitHub PRs: `gh pr view <number> --repo DataDog/datadog-agent`
- Other URLs: `WebFetch`

**If requirements are unclear after reading all context:**
1. Add a comment asking for clarification with specific questions
2. Skip and move to the next ticket

---

## Completing a Ticket

### When the draft PR is open

Add a comment on the Jira ticket with the PR link and move on:

```
PR opened (draft): <URL>

— Auto-JIRA (https://github.com/DataDog/datadog-agent/blob/main/.agents/skills/auto-jira/SKILL.md)
```

### When blocked

1. Transition ticket to HOLD
2. Add a NEW comment (do NOT edit description) explaining:
   - What is blocked
   - What human action is needed
   - Relevant links (PR, CI failure)

```
Blocked: <reason>

<details>

PR (if any): <URL>

Needs: <specific human action required>

— Auto-JIRA (https://github.com/DataDog/datadog-agent/blob/main/.agents/skills/auto-jira/SKILL.md)
```

---

## CRITICAL: Do NOT Take Other People's Tickets

Before claiming any ticket, check the Assignee field — it must be empty. If it has anyone's name, do not touch the ticket.

Even if a ticket looks abandoned, skip it. We do not step on other developers' toes.

# Ticket Workflow

Guide for selecting, claiming, and managing Jira tickets for autonomous work.

**Read this when:** you are about to select or claim a ticket.

---

## Querying the Backlog

Use `mcp__atlassian__searchJiraIssuesUsingJql` with:
- `cloudId`: `datadoghq.atlassian.net`
- `fields`: `["summary", "description", "status", "assignee", "labels", "issuetype", "created", "priority", "parent"]`
- `maxResults`: 50

### JQL template

```
project = <BOARD>
  AND status = "To Do"
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
mcp__atlassian__getJiraIssue(
  cloudId="datadoghq.atlassian.net",
  issueIdOrKey="<KEY>",
  fields=["summary", "description", "status", "assignee", "labels",
          "comment", "priority", "issuetype", "parent", "created"]
)
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

```
mcp__atlassian__atlassianUserInfo()
```

Save your `accountId` for subsequent calls.

### Step 2: Assign to yourself

```
mcp__atlassian__editJiraIssue(
  cloudId="datadoghq.atlassian.net",
  issueIdOrKey="<KEY>",
  fields={"assignee": {"accountId": "<YOUR_ACCOUNT_ID>"}}
)
```

POST-ACTION verify: re-fetch the issue and confirm `assignee.accountId` matches yours.

### Step 3: Add to the current sprint

Fetch your active sprint ID and move the ticket into it:

```
mcp__atlassian__searchJiraIssuesUsingJql(
  cloudId="datadoghq.atlassian.net",
  jql='project = <BOARD> AND sprint in openSprints() AND sprint not in futureSprints()',
  fields=["summary", "sprint"],
  maxResults=1
)
```

Extract the `sprint.id` from the result, then:

```
mcp__atlassian__editJiraIssue(
  cloudId="datadoghq.atlassian.net",
  issueIdOrKey="<KEY>",
  fields={"customfield_10020": {"id": <SPRINT_ID>}}
)
```

POST-ACTION verify: re-fetch the issue and confirm the sprint field is set.

If no open sprint is found or the edit fails, log a warning in `AUTO_JIRA.md` and continue — this is non-blocking.

### Step 4: Add a start comment

```
mcp__atlassian__addCommentToJiraIssue(
  cloudId="datadoghq.atlassian.net",
  issueIdOrKey="<KEY>",
  body="Starting autonomous implementation.\n\n— Auto-JIRA (https://github.com/DataDog/datadog-agent/blob/main/.claude/skills/auto-jira/SKILL.md)"
)
```

POST-ACTION verify: fetch the issue comments and confirm the comment appears.

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

— Auto-JIRA (https://github.com/DataDog/datadog-agent/blob/main/.claude/skills/auto-jira/SKILL.md)
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

— Auto-JIRA (https://github.com/DataDog/datadog-agent/blob/main/.claude/skills/auto-jira/SKILL.md)
```

---

## CRITICAL: Do NOT Take Other People's Tickets

Before claiming any ticket, check the Assignee field — it must be empty. If it has anyone's name, do not touch the ticket.

Even if a ticket looks abandoned, skip it. We do not step on other developers' toes.

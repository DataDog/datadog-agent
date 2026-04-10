---
name: newsletter
description: Generate the Agent Supply Chain newsletter by researching team activity on GitHub and Confluence, then creating a Confluence draft and Gmail draft
argument-hint: "<period e.g. 'February-March 2026'>"
---

Generate the Agent Supply Chain newsletter for the period **$ARGUMENTS** by researching what team members accomplished on GitHub and Confluence, then producing both a Confluence blog post draft and a Gmail draft.

## Step 1: Gather team members

Fetch the current list of agent-supply-chain team members from GitHub:

```bash
gh api orgs/DataDog/teams/agent-supply-chain/members --paginate --jq '.[].login'
```

Ask the user if any members should be excluded (e.g. people who moved teams).

## Step 2: Read past newsletters for format reference

1. Search for the latest blog posts in the ASC1 Confluence space:
   - `mcp__claude_ai_Atlassian__searchConfluenceUsingCql` with `cql`: `type = "blogpost" AND space = "ASC1" ORDER BY created DESC` (limit 3)
2. Read the most recent newsletter with `mcp__claude_ai_Atlassian__getConfluencePage` (contentFormat: `markdown`, contentType: `blog`) to match its structure and tone.

The newsletter format is:
- **Header**: greeting, team links (ASC1, ABLD, BARX, ADX spaces), OKR link, support channel link
- **"What's new?"** section organized by sub-team (Agent Developer Experience, Agent Build, Agent Delivery)
- Each item has: a short title, **quantified impact** (time saved, percentage improvement, count), and **links to PRs or docs**
- **"Did you know?"** section with one fun/useful tip
- **Footer**: link to all newsletters, support channel reminder

## Step 3: Research GitHub activity

For each team member, launch background agents (use `run_in_background: true`) to search merged PRs during the period. Split into batches of ~8 members per agent to parallelize.

Each agent should run, for every user:

```bash
# PRs in datadog-agent
gh pr list --repo DataDog/datadog-agent --author USERNAME --state merged --search "merged:START_DATE..END_DATE" --limit 50 --json title,url,mergedAt,labels

# PRs across the DataDog org (catches buildimages, k8s-ops, integrations-core, etc.)
gh search prs --author USERNAME --owner DataDog --merged --merged-after START_DATE --merged-before END_DATE --limit 20 --json repository,title,url
```

Each agent should return a summary per user, grouped thematically (build improvements, CI/CD, new features, bug fixes, etc.). Skip trivial PRs (version bumps, dependency updates). Focus on items that **impact teams outside Agent Supply Chain**.

## Step 4: Research Confluence activity

Launch a background agent to search for relevant documentation created during the period:

```
mcp__claude_ai_Atlassian__searchConfluenceUsingCql
```

With CQL queries:
- `space = "ASC1" AND lastModified >= "START_DATE" AND lastModified <= "END_DATE" ORDER BY lastModified DESC` (limit 25)
- `space = "ADX" AND lastModified >= "START_DATE" AND lastModified <= "END_DATE" ORDER BY lastModified DESC` (limit 25)

Identify RFCs, design docs, operational reports, and guides that are newsletter-worthy.

## Step 5: Synthesize and write the newsletter

Apply the newsletter guide's filter: **"Is this information impacting a team outside of the Agent Supply Chain group?"** Only include items where the answer is yes.

For each item:
- Provide a **quantifiable improvement** (time saved, percentage change, cost reduction) when available
- Link to the relevant **PR, Confluence page, or documentation**
- Keep descriptions concise (2-4 sentences max per item)

Group items under:
1. **Agent Developer Experience** (CI speed, developer tools, workflows, open source)
2. **Agent Build** (Bazel migration, build system, platform support)
3. **Agent Delivery** (releases, deployments, registries, security)

End with a **"Did you know?"** section highlighting one interesting tool, feature, or tip.

## Step 6: Create outputs

### 6a. Confluence blog post draft

Use `mcp__claude_ai_Atlassian__createConfluencePage` with:
- `cloudId`: `datadoghq.atlassian.net`
- `spaceId`: `4662624793` (ASC1 space)
- `title`: `<Period> - Agent Supply Chain Monthly Update`
- `contentType`: `blog`
- `status`: `draft`
- `contentFormat`: `markdown`

### 6b. Gmail draft

Use `mcp__claude_ai_Gmail__gmail_create_draft` with:
- `to`: `agent-all-teams@datadoghq.com`
- `subject`: `<Period> - Agent Supply Chain Monthly Update`
- `contentType`: `text/html`
- Rich HTML body matching the Confluence content

## Step 7: Present results

Return to the user:
1. Links to both the Confluence draft and Gmail draft
2. A bullet-point summary of the sections covered
3. Remind them to:
   - Review both outputs for accuracy
   - Send the Gmail to themselves first to verify formatting
   - Schedule the final send between **2-4pm CET**, not on a Friday
   - Ask for review in `#agent-devx-private` / from Damien Desmarets before publishing

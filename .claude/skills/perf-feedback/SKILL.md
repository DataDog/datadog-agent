---
name: perf-feedback
description: Generate a performance self-evaluation by gathering work data from GitHub, Jira, Confluence, and Datadog
argument-hint: "[github-username] [--months N]"
---

# Performance Self-Evaluation Generator

Generate a comprehensive performance self-evaluation following the Datadog SBI (Situation, Behavior, Impact) framework by gathering data from multiple sources.

## What this skill does

1. **Identifies the user** via git config (email, name) and GitHub CLI (username), and Atlassian account
2. **Reads the official self-evaluation guidelines** from the Confluence page [How to Write Your Self Evaluation](https://datadoghq.atlassian.net/wiki/spaces/peoplesolutions/pages/2535424703)
3. **Gathers work data** from the last 6 months (configurable via `--months N`) across:
   - **GitHub**: All merged PRs, grouped by repository
   - **Jira**: All assigned issues, grouped by project/theme with status breakdown
   - **Confluence**: All pages created or updated
   - **Datadog**: Dashboards, monitors, and notebooks authored
4. **Generates a draft self-evaluation** answering the 4 Workday questions using the SBI framework, with links to relevant sources

## Usage

- `/perf-feedback` -- Generate feedback for the current git user, last 6 months
- `/perf-feedback chouetz` -- Generate feedback for a specific GitHub username
- `/perf-feedback --months 12` -- Generate feedback covering the last 12 months

## Instructions

### Step 1: Identify the user

Determine the user's identity from multiple sources. Run these in parallel:
- `git config user.email` and `git config user.name` via Bash
- `gh api user --jq '.login'` via Bash to get the GitHub username (override with the argument if provided)
- Use the `mcp__atlassian__atlassianUserInfo` tool to get the Atlassian account ID and job title

If a GitHub username is passed as argument, use that instead of the auto-detected one.

### Step 2: Determine the time range

Default to the last 6 months from today. If `--months N` is provided in the arguments, use N months instead. Compute the start date as `YYYY-MM-DD`.

### Step 3: Read the self-evaluation guidelines

Fetch the Confluence page with ID `2535424703` from `datadoghq.atlassian.net` using the `mcp__atlassian__getConfluencePage` tool with `contentFormat: markdown`. This contains the SBI framework and the 4 questions to answer.

### Step 4: Gather work data in parallel

Run ALL of the following in parallel to maximize speed:

#### 4a. GitHub PRs
Use Bash to run:
```
gh search prs --author=<github_username> --merged-at=">=<start_date>" --limit=100 --json title,url,closedAt,repository
```
Then run a second query to group by repository:
```
gh search prs --author=<github_username> --merged-at=">=<start_date>" --limit=100 --json title,url,closedAt,repository --jq 'group_by(.repository.nameWithOwner) | map({repo: .[0].repository.nameWithOwner, count: length, prs: map(.title)}) | sort_by(-.count)'
```

#### 4b. Confluence pages
Use `mcp__atlassian__searchConfluenceUsingCql` with:
- cloudId: `datadoghq.atlassian.net`
- cql: `contributor = currentUser() AND lastmodified >= "<start_date>" ORDER BY lastmodified DESC`
- limit: 50

If the result is too large and saved to a file, use a subagent (Task tool with general-purpose type) to parse it and extract: title, space, last modified date, URL. Group by topic/space.

#### 4c. Jira issues
Use `mcp__atlassian__searchJiraIssuesUsingJql` with:
- cloudId: `datadoghq.atlassian.net`
- jql: `assignee = currentUser() AND updated >= "<start_date>" ORDER BY updated DESC`
- fields: `["summary", "status", "issuetype", "priority", "created", "updated", "resolution", "labels"]`
- maxResults: 100

If the result is too large and saved to a file, use a subagent (Task tool with general-purpose type) to parse it and extract: key, summary, status, type, priority, labels. Group by project/theme. Count totals and breakdown by status.

#### 4d. Datadog dashboards
Use `mcp__datadog-mcp__search_datadog_dashboards` with:
- query: `author.handle:<user_email>`
- sort_by: `-modified_at`

#### 4e. Datadog monitors
Use `mcp__datadog-mcp__search_datadog_monitors` with:
- query: `creator:<user_email>`

#### 4f. Datadog notebooks
Use `mcp__datadog-mcp__search_datadog_notebooks` with:
- filter: `author.handle:<user_email>`
- query: `*`
- sort_by: `-modified_at`

### Step 5: Present data summary

Before writing the evaluation, present a structured summary of all gathered data:
- GitHub: total PRs, breakdown by repository (table)
- Jira: total issues, breakdown by status, key themes
- Confluence: total pages, grouped by space/topic
- Datadog: dashboards, monitors, notebooks created

### Step 6: Write the self-evaluation draft

Generate the draft answering the 4 Workday questions using the SBI framework. Use **bullet lists** (not tables) for easy copy/paste into Workday. Include **hyperlinks** to the most relevant sources (PRs, Jira issues, Confluence pages, notebooks).

#### Question 1: What are you doing really well? Pick 2-3 strengths you should continue to leverage.

For each strength, use this format:
```
**Strength N: <Title>**

- **Situation:** <What you did well -- 1 sentence>
- **Behavior:** <Specific examples with links -- 1-2 paragraphs>
- **Impact:** <How this behavior impacts success -- 1 sentence>
```

Identify 2-3 strengths by analyzing the themes across GitHub PRs, Jira issues, and Confluence pages. Look for:
- Recurring patterns (e.g., CI improvements, security work, automation)
- High-impact contributions (incidents resolved, features shipped, processes improved)
- Cross-team work (contributions to multiple repos/teams)

#### Question 2: What area(s) of development should you focus on? Pick 1-2 areas for development and give examples.

For each area, use the same SBI bullet format. Look for signals like:
- PRs that required multiple reverts/re-applications
- Jira issues that stayed blocked or in progress for a long time
- Draft documents that were never finalized
- Frame development areas positively: "I want to focus on improving X so that Y"

#### Question 3: Select an indicator

Suggest an indicator (Needs Development / On Track / Sets a New Standard) with a brief justification based on the data. Add a note that the user should calibrate against their career path document.

#### Question 4: Determine one action you can take toward growth

Suggest a concrete, actionable next step that addresses the development areas identified in Question 2. Reference the [3E Opportunity Bank](https://datadoghq.atlassian.net/wiki/spaces/peoplesolutions/pages/3668771235) and link to specific upcoming Jira issues where the action can be applied.

### Output format

The final output should be plain text with markdown formatting (bullet lists, bold, hyperlinks) that can be directly copy/pasted into Workday. Do NOT use tables anywhere in the final output.

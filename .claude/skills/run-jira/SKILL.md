---
name: run-jira
description: Fetch a Jira issue and propose an implementation plan based on codebase analysis
argument-hint: "<JIRA-KEY e.g. ACTP-1234>"
---

Fetch the Jira issue **$ARGUMENTS** from the Datadog Atlassian instance and use it as the basis for a codebase analysis and implementation proposal.

## Step 1: Gather Jira issue data

Use the Atlassian MCP tools to fetch the issue. **Only request the fields you need** to avoid huge responses:

1. Call `mcp__atlassian__getJiraIssue` with:
   - `cloudId`: `datadoghq.atlassian.net`
   - `issueIdOrKey`: `$ARGUMENTS`
   - `fields`: `["summary", "description", "status", "assignee", "issuetype", "comment", "priority"]`
2. Extract the **title** (summary), **description**, **status**, **assignee**, and **comments** from the response.

If the issue cannot be found, stop and inform the user.

## Step 2: Summarize the issue

Present a clear summary of the Jira issue:
- **Key**: $ARGUMENTS
- **Title**: the issue summary
- **Status**: current status
- **Assignee**: who is assigned
- **Description**: the full description
- **Comments**: any relevant context or discussion from comments
- **Link**: https://datadoghq.atlassian.net/browse/$ARGUMENTS

## Step 3: Fetch linked resources

Scan the issue description and comments for links to external resources and fetch them for additional context:

- **Datadog notebooks / postmortems** (`app.datadoghq.com/notebook/<id>`): use `mcp__datadog-mcp__get_datadog_notebook` with the notebook ID
- **GitHub PRs** (`github.com/.../pull/<number>`): use `gh pr view <number>` via Bash
- **GitLab commits/pipelines** or other URLs: use `WebFetch` if accessible

This step is critical — linked resources often contain the root cause analysis, timelines, and technical details that the Jira description alone does not capture.

## Step 4: Analyze the codebase

Based on the issue requirements and linked resources, explore the codebase to understand:
- Which files and packages are relevant
- Existing patterns and conventions that should be followed
- Dependencies and potential impacts

Use Glob, Grep, and Read tools extensively. For broad exploration, use the Task tool with `subagent_type=Explore`.

## Step 5: Propose an implementation

Enter plan mode with `EnterPlanMode` and write a detailed implementation plan that includes:
- A breakdown of the changes needed, organized by file
- Any new files that need to be created
- Test strategy
- Potential risks or open questions

Wait for user approval before implementing.

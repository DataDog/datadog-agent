# Runtime & MCP tooling

This skill runs in **two environments** with different Atlassian MCP servers. Detect which one you have at the start of every run, then use the matching column in the tool mapping. Never assume one runtime's tool names exist in the other.

## Detection

- If `jira_get_issue` / `jira_get_issue_development_info` is callable → **Cursor** path (community `mcp-atlassian`, the `user-atlassian` server).
- If `mcp__atlassian__getJiraIssue` is callable → **Claude Code** path (official Atlassian Rovo server).
- If neither, stop and ask the user to connect/authenticate an Atlassian MCP server.

## Tool mapping

| Logical operation | Cursor (`mcp-atlassian`) | Claude Code (Atlassian Rovo) |
|---|---|---|
| Fetch issue | `jira_get_issue` — `issue_key`, `fields` (CSV), `comment_limit` | `mcp__atlassian__getJiraIssue` — `cloudId`, `issueIdOrKey`, `fields` (array), `responseContentFormat:"markdown"` |
| Search children | `jira_search` — `jql`, `fields` (CSV), `limit` | `mcp__atlassian__searchJiraIssuesUsingJql` — `cloudId`, `jql`, `fields` (array), `maxResults` |
| PR detail / Tier 0 | `jira_get_issue_development_info` — `issue_key`, `application_type:"GitHub"`, `data_type:"pullrequest"` | **NOT AVAILABLE** — skip Phase A1 entirely (see *Claude Code capability gap* in `references/pr-discovery.md`) |
| Post comment | `jira_add_comment` — `issue_key`, `body` | `mcp__atlassian__addCommentToJiraIssue` — `cloudId`, `issueIdOrKey`, `commentBody`, `contentFormat:"markdown"` |

## Key runtime differences

- **Cursor** — no `cloudId` needed; `fields` is CSV; has full Jira Development panel access (Tier 0 PRs).
- **Claude Code** — `cloudId` required on every call; `fields` is a JSON array; **no dev-status endpoint** — Phase A1/Tier 0 is unavailable, all PR URLs come from GitHub search (Phase B).

## JQL for Epic children

Use `parent = <EPIC-KEY>` — works on both runtimes and modern Jira hierarchies. Fall back to `"Epic Link" = <EPIC-KEY>` (double quotes exactly as shown) only if `parent =` is rejected on a classic project.

## cloudId (Claude Code only)

Pass `cloudId: "datadoghq.atlassian.net"` directly — the site hostname works. If a call rejects it as invalid/unknown, call `mcp__atlassian__getAccessibleAtlassianResources` once and use the returned `id` (UUID) for the `datadoghq` site.

## Large responses (Claude Code)

`searchJiraIssuesUsingJql` can exceed the MCP response token cap and be spilled to a file. To avoid/handle this, request **only the fields you need** (e.g. `["summary","status","issuetype","customfield_10000"]`); if the result is saved to a file, parse it with `jq` instead of re-reading the whole blob.

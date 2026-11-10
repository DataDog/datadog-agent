# Runtime & MCP tooling

This skill works with **two kinds** of Atlassian MCP server. Detect which one you have at the start of every run, then use the matching column in the tool mapping. Never assume one server's tool names exist in the other.

## Detection

- If `jira_get_issue` / `jira_get_issue_development_info` is callable → **`mcp-atlassian`** path (community [`mcp-atlassian`](https://github.com/sooperset/mcp-atlassian), e.g. the `user-atlassian` server in Cursor).
- If `get_issue` from the `atlassian` server is callable (`mcp__atlassian__get_issue` in Claude Code) → **internal** path (Datadog's internal Atlassian MCP server, the `atlassian` entry in this repo's MCP configs).
- If neither, stop and ask the user to connect/authenticate an Atlassian MCP server.

## Tool mapping

| Logical operation | `mcp-atlassian` | Internal (`atlassian`) |
|---|---|---|
| Fetch issue | `jira_get_issue` — `issue_key`, `fields` (CSV), `comment_limit` | `mcp__atlassian__get_issue` — `issue_key` only |
| Search children | `jira_search` — `jql`, `fields` (CSV), `limit` | `mcp__atlassian__search_issues` — `jql`, `max_results` |
| PR detail / Tier 0 | `jira_get_issue_development_info` — `issue_key`, `application_type:"GitHub"`, `data_type:"pullrequest"` | **NOT AVAILABLE** — skip Phase A1 entirely (see *Internal server capability gap* in `references/pr-discovery.md`) |
| Post comment | `jira_add_comment` — `issue_key`, `body` | `mcp__atlassian__add_comment` — `issue_key`, `body`, `body_format:"markdown"` |

## Key runtime differences

- **`mcp-atlassian`** — `fields` is CSV; has full Jira Development panel access (Tier 0 PRs).
- **Internal** — no field selection: `get_issue` and `search_issues` return the server's default set of issue fields, so read what you need from the response. If a field is missing (e.g. `customfield_10000`), treat it as unavailable on this server. **No dev-status endpoint** — Phase A1/Tier 0 is unavailable, all PR URLs come from GitHub search (Phase B). No `cloudId`: the server only serves the Datadog Atlassian site.

## Reading comments

Comments carry context that is not in the description or PRs, so this skill reads them on the Epic (Step 2) and on relevant child issues (Step 3).

- **`mcp-atlassian`** — pass `comment_limit: <N>` (e.g. `20`) to `jira_get_issue`; the comments come back on the issue payload.
- **Internal** — read the comments from the `get_issue` response; there is no parameter to request or limit them.
- **Never request the `comment` field in the bulk *Search children* call** (`mcp-atlassian`) — it explodes the response size (see *Large responses*). On both servers, read comments per issue with individual *Fetch issue* calls instead, and cap how many issues / comments you pull.

## JQL for Epic children

Use `parent = <EPIC-KEY>` — works on both servers and modern Jira hierarchies. Fall back to `"Epic Link" = <EPIC-KEY>` (double quotes exactly as shown) only if `parent =` is rejected on a classic project.

## Large responses

A search can exceed the MCP response token cap and be spilled to a file. To avoid/handle this, request **only the fields you need** on `mcp-atlassian` (e.g. `summary,status,issuetype`); on the internal server, which has no field selection, keep `max_results` to what you need. If the result is saved to a file, parse it with `jq` instead of re-reading the whole blob.

---
name: create-epic-recap
description: "Use when an engineer or manager asks to recap, summarize, or post a Jira Epic resolution: gathers completed child issues, merged GitHub PRs, and release notes, previews a stakeholder-ready recap, and posts only after approval."
argument-hint: "<EPIC-KEY e.g. OTAGENT-820> [--dry-run]"
model: sonnet
allowed-tools: Bash, Read, Write, Glob, Grep, AskUserQuestion, mcp__atlassian__getJiraIssue, mcp__atlassian__searchJiraIssuesUsingJql, mcp__atlassian__getJiraIssueRemoteIssueLinks, mcp__atlassian__addCommentToJiraIssue, mcp__atlassian__getAccessibleAtlassianResources
---

Generate a resolution recap for the Jira Epic **$ARGUMENTS**, aggregating information from merged GitHub PRs and release notes. Show a preview and post the recap as a comment on the Epic only after explicit user approval.

This skill is intended for an engineer who has finished work on an Epic and wants to communicate the resolution to PMs and stakeholders without losing flow. See [OTAGENT-1038](https://datadoghq.atlassian.net/browse/OTAGENT-1038) for motivation.

**Owning team:** `team/opentelemetry-agent` (`@DataDog/opentelemetry-agent`)

## Prerequisites

Before running this skill, verify the following. If any check fails, stop and tell the user what to fix.

1. **Atlassian MCP server** — an Atlassian/Jira MCP server must be connected and authenticated. Cursor uses the community `mcp-atlassian` server (`user-atlassian`); Claude Code uses the official Atlassian Rovo server. Run a quick probe (e.g. fetch a known issue) to confirm connectivity; if it fails, ask the user to authenticate or connect the server.
2. **GitHub CLI (`gh`)** — must be installed and authenticated. Run `gh auth status`; if it reports no active account, ask the user to run `gh auth login`.
3. **Repository context** — the skill uses `gh search prs --owner DataDog` and `gh pr view --repo DataDog/<repo>`. It does not need to be run from inside a checkout, but `gh` must have access to the DataDog GitHub org.

## Example

**Input:** `/create-epic-recap OTAGENT-304 --dry-run`

**What happens:**
1. Fetches Epic OTAGENT-304 ("Move opentelemetry-mapping-go to datadog-agent repo")
2. Finds 11 completed child issues, filters to Done status
3. Queries Jira Development panel (Phase A) and GitHub search (Phase B) for merged PRs across all child keys
4. Discovers 8 PRs in `datadog-agent`, 2 in `opentelemetry-mapping-go`, 1 in `documentation`; excludes 6 cross-references
5. Reads release notes from PR file lists
6. Renders the recap and prints a preview (dry-run — no Jira comment posted)

**Output preview (abbreviated):**

```markdown
# Resolution recap: OTAGENT-304 — Move opentelemetry-mapping-go to datadog-agent repo

## Summary
Migrated the opentelemetry-mapping-go library into the datadog-agent monorepo,
bumped OTel Collector dependencies through v0.136.0, removed the deprecated
routing processor, and added support for the new `deployment.environment.name`
semconv in workload meta.

## What's new for users
- OTel Collector dependencies upgraded to v0.136.0
- Deprecated OTel routing processor removed (upstream removal)
- New `deployment.environment.name` env convention supported in workload meta
...

## Linked PRs & release notes
- [datadog-agent#36512](…) — [OTAGENT-304] Move opentelemetry-mapping-go to datadog-agent repo
- [datadog-agent#40230](…) — [OTAGENT-510] bump otel versions to v0.133.0
- [opentelemetry-mapping-go#767](…) — migrate pkg/otlp/rum to otel/semconv _(linked via Jira)_
...
```

`Saved draft to /tmp/OTAGENT-304-recap.md`

## Runtime & MCP tooling

This skill runs in **two environments** with different Atlassian MCP servers. Detect which one you have at the start of every run, then use the matching column in the tool mapping below. Never assume one runtime's tool names exist in the other.

**Detection:** if `jira_get_issue` / `jira_get_issue_development_info` is callable → **Cursor** path. If `mcp__atlassian__getJiraIssue` is callable → **Claude Code** path. If neither, stop and ask the user to connect/authenticate an Atlassian MCP server.

### Tool mapping

| Logical operation | Cursor (`mcp-atlassian`) | Claude Code (Atlassian Rovo) |
|---|---|---|
| Fetch issue (Steps 2 & 4-A2) | `jira_get_issue` — `issue_key`, `fields` (CSV), `comment_limit` | `mcp__atlassian__getJiraIssue` — `cloudId`, `issueIdOrKey`, `fields` (array), `responseContentFormat:"markdown"` |
| Search children (Step 3) | `jira_search` — `jql`, `fields` (CSV), `limit` | `mcp__atlassian__searchJiraIssuesUsingJql` — `cloudId`, `jql`, `fields` (array), `maxResults` |
| **PR detail / Tier 0 (Step 4-A1)** | `jira_get_issue_development_info` — `issue_key`, `application_type:"GitHub"`, `data_type:"pullrequest"` | **NOT AVAILABLE** — skip A1 entirely (see [Appendix A](#appendix-a--claude-code-rovo-capability-gap)) |
| Post comment (Step 11) | `jira_add_comment` — `issue_key`, `body` | `mcp__atlassian__addCommentToJiraIssue` — `cloudId`, `issueIdOrKey`, `commentBody`, `contentFormat:"markdown"` |

**Key runtime differences (details in Appendix):**
- **Cursor** — no `cloudId` needed; `fields` is CSV; has full Jira Development panel access (Tier 0 PRs).
- **Claude Code** — `cloudId` required on every call (pass `"datadoghq.atlassian.net"`; see [Appendix B](#appendix-b--cloudid-and-large-responses-claude-code)); `fields` is a JSON array; **no dev-status endpoint** — Phase A1/Tier 0 is unavailable, all PR URLs come from GitHub search.

**JQL for Epic children (Step 3):** use `parent = <EPIC-KEY>` — works on both runtimes and modern Jira hierarchies. Fall back to `"Epic Link" = <EPIC-KEY>` only if `parent =` is rejected on a classic project.

## Step 1: Parse arguments

Parse `$ARGUMENTS`:
- **EPIC-KEY** (required, first positional): Jira issue key matching `^[A-Z][A-Z0-9_]+-\d+$`, e.g. `OTAGENT-820`.
- `--dry-run` (optional flag): render and preview the recap, but never post it. The skill prints the markdown and exits.

If `EPIC-KEY` is missing or does not match the pattern, stop and ask the user to provide it.

## Step 2: Fetch the Epic

Call the **Fetch issue** tool for your runtime (see *Runtime & MCP tooling → Tool mapping*) requesting fields `summary, description, status, issuetype, labels, assignee, reporter`:
- **Cursor:** `jira_get_issue` — `issue_key: EPIC-KEY`, `fields` as the CSV above, `comment_limit: 10`.
- **Claude Code:** `mcp__atlassian__getJiraIssue` — `cloudId`, `issueIdOrKey: EPIC-KEY`, `fields` as an array, `responseContentFormat: "markdown"`.

Validate:
- If the issue cannot be found, stop and inform the user.
- Read the issue type name from whichever shape the runtime returns — accept **both** `issuetype.name` (Jira's native field, the Claude Code/Rovo shape) **and** `issue_type.name` (the snake_cased shape some `mcp-atlassian` versions return). If the resolved name is not `Epic`, stop and inform the user that this skill only works on Epics. Suggest using `/run-jira` for non-Epic issues. Do **not** reject the issue just because one of the two shapes is absent.

Keep the response in memory — `summary`, `description`, `status`, `labels` will be used during rendering.

## Step 3: Fetch child issues

Call the **Search children** tool for your runtime (see *Tool mapping*) with:
- `jql`: `parent = <EPIC-KEY>` — works on both runtimes and modern Jira hierarchies. Fall back to `"Epic Link" = <EPIC-KEY>` (double quotes exactly as shown) only if `parent =` is rejected on a classic project.
- `fields`: `summary, status, issuetype, assignee, labels` (CSV on Cursor; array on Claude Code).
- limit: `50` — param is `limit` on Cursor, `maxResults` on Claude Code.

On Claude Code, also request `customfield_10000` in this same call so Step 4-A2 PR counts come back for free. If the response is large and gets spilled to a file, parse it with `jq` rather than re-reading the whole blob.

Collect each child issue's `key`, `summary`, `status.name`, and `status.category` (or `status.statusCategory.key` depending on the API shape — accept both).

**Filter out unfinished work.** A resolution recap is about what shipped, so drop any child issue whose `status.category` (or `statusCategory.key`) is `To Do` / `new` / `indeterminate`. Keep only children in the `Done` category (statuses like `Done`, `Closed`, `Resolved`). Record the dropped children in a `skipped_children` list so you can mention them in the preview if the user asks.

If the resulting list of completed children is empty, that is fine — some Epics are resolved by PRs that reference the Epic key directly. Continue with just `<EPIC-KEY>` as the search term.

## Step 4: Find merged PRs

Build the list of Jira keys to search for: `[EPIC-KEY, <completed child keys from Step 3>]`.

### Phase A — Jira Development panel (Tier 0)

Phase A has two sub-steps: A1 tries to get full PR details, A2 falls back to a cached summary for validation.

#### A1 — Detail endpoint (full PR info)

> **Claude Code (Atlassian Rovo): SKIP A1 entirely.** This server has no dev-status / Development-panel tool, so Tier 0 PR URLs cannot be read from Jira. Go straight to A2 (counts) and rely on Phase B for the actual URLs. The rest of A1 is **Cursor (`mcp-atlassian`) only.**

For each key, call `jira_get_issue_development_info` on the `user-atlassian` server with:
- `issue_key`: `<KEY>`
- `application_type`: `"GitHub"` (**CamelCase is mandatory** — lowercase `"github"` returns empty results; this is a Jira REST API quirk in `/rest/dev-status/1.0/issue/detail`)
- `data_type`: `"pullrequest"` (**mandatory** — the Jira Cloud API requires `dataType` in the query; omitting it causes HTTP 500 with `"message":"dataType"`)

Do **not** use the batch tool `jira_get_issues_development_info` — it returns HTTP 500 when `application_type` is passed.

**Throttling — Jira dev-status API does not tolerate parallel calls.** Parallel requests to `/rest/dev-status/1.0/issue/detail` trigger server-side rate limiting (HTTP 500). Apply all of the following:
- **Sequential calls only** — issue one `jira_get_issue_development_info` at a time, never in parallel.
- **~1 s pause** between calls (`sleep 1` or equivalent).
- **Retry once on 500** — wait 5 s, then retry the failing key. If the retry also fails, record the key as "Phase A1 miss" and move on.
- **Fail-fast after 3 consecutive 500s** — the endpoint is likely down. Stop A1 entirely, log `Phase A1 aborted after 3 consecutive 500s, falling back to A2`, and proceed to A2 for **all** keys.

From each successful response, collect `pullRequests` entries where `status == "MERGED"`. Record each as a **Tier 0** PR with fields: `id` (e.g. `#52248`), `name` (title), `url`, `status`, `source.branch`, `destination.branch`, `author`, `reviewers`, `lastUpdate`, `repositoryUrl`.

**Tier 0 PRs are auto-included with the highest confidence** — they are explicitly linked to the Jira issue by the GitHub↔Jira integration. No heuristic filtering is needed.

Deduplicate Tier 0 results by `(repository, PR number)` across all keys.

#### A2 — Development summary field (fallback for PR count validation)

If A1 failed (fail-fast triggered) or returned zero Tier 0 PRs for any key, fetch the cached development summary. This does **not** use the flaky `dev-status` endpoint — it reads a standard Jira custom field via the stable `/rest/api/2/issue/` API.

Call the **Fetch issue** tool for your runtime (see *Tool mapping*) requesting only `customfield_10000` (the "Development" field, type `com.atlassian.jira.plugins.jira-development-integration-plugin:devsummarycf`):
- **Cursor:** `jira_get_issue` — `issue_key: <KEY>`, `fields: "customfield_10000"`, `comment_limit: 0`. The field comes back under `customfield_10000.value`.
- **Claude Code:** `mcp__atlassian__getJiraIssue` — `cloudId`, `issueIdOrKey: <KEY>`, `fields: ["customfield_10000"]`. The value is a **string** (see [Appendix A — `customfield_10000` shape](#customfield_10000-shape-a2)), not an object — extract the embedded `json={…}` first.

In both cases the embedded JSON holds `cachedValue.summary.pullrequest.overall.count` (number of linked PRs) and `…overall.state` (`MERGED`, `OPEN`, etc.).

Record the expected PR count per key in a `jira_pr_counts` map: `{ "OTAGENT-307": 4, "OTAGENT-510": 3, … }`. Keys with no `pullrequest` in the summary (or where `customfield_10000` is null) have count 0.

This map is used in Phase B for **cross-validation**: after tier classification, compare the number of included PRs (Tier 0 + Tier 1 + Tier 2) per key against `jira_pr_counts`. Record every key where Phase B found fewer in a `pr_shortfall` list (`{key, jira_count, found_count}`) — it drives **both** the preview warning below **and** the report's `{{pr_discovery_note}}` (Step 9), so the limitation reaches stakeholders, not just the engineer. If Phase B found fewer, show a warning in the preview:

```
⚠️ Jira says OTAGENT-307 has 4 linked PRs, but only 2 were found via GitHub search.
   Check the Jira Development panel manually or promote Tier 3 candidates.
```

A2 calls can run **in parallel** (they use the standard Jira API, not `dev-status`). Batch them with Step 2/3 calls when possible.

### Phase B — GitHub search (Tiers 1–4)

**Always run Phase B for all keys** — even those with Tier 0 PRs from A1. This ensures maximum recall: the Jira integration may not capture all PRs (e.g., PRs in other repos, or PRs where the key is only in the body). Run `gh search prs` for every key.

**One key per request — `OR` is not supported.** `gh search prs` treats the query as a literal string, so `"OTAGENT-410 OR OTAGENT-411"` will match zero PRs (verified in the wild). Issue one `gh search prs` call per Jira key:

```bash
gh search prs \
  --owner DataDog \
  --merged \
  --limit 20 \
  --json repository,title,url,number,labels,author,body \
  -- "<KEY>"
```

**Throttling — avoid GitHub's secondary rate limit.** GitHub's search API has a low secondary limit (~30 req/min); a batch of 10+ keys easily trips it. Apply both:
- Cap parallelism at **at most 4 concurrent `gh search prs` calls** (smaller batches are safer).
- Sleep **~500 ms between waves** of parallel calls. A simple `sleep 0.5` between batches is sufficient.

If a call returns HTTP 403 with a `secondary rate limit` message, follow the back-off in the "Errors and edge cases" section (60 s pause then retry the failing keys one at a time).

Notes:
- The `--` separates flags from the query; the bare key goes in the query (no quotes around the key needed inside the JSON shell array).
- `gh search prs` matches the key in title, body, and commit messages — which is exactly the source of false positives below.
- Deduplicate by `(repository.nameWithOwner, number)` across all keys **and** against Tier 0 PRs from Phase A. If a PR was already collected as Tier 0, do not downgrade it — keep Tier 0.

### Tier classification (Phase B PRs only)

**Classify each PR from Phase B into one of four tiers, based on where the searched key `<KEY>` appears.** This trades aggressive precision (lots of false positives, e.g. cross-references) for recall, and is precision-over-recall by design — Tier 3 is shown in preview so the user can opt-in to include borderline items.

Apply the tiers in order; the first match wins.

**Tier 1 — auto-include (key in PR title).** Word-boundary match `\b<KEY>\b` against `title`. Standard forms:
- `[OTAGENT-410] …` (bracketed, the standard `team/opentelemetry-agent` format)
- bare `OTAGENT-410` as a word
- `(OTAGENT-410)` inside parentheses

**Tier 2 — auto-include (key with closing keyword in body).** Match the key in `body` only when it is preceded by a standard GitHub closing keyword or a JIRA-field label, on the same logical line, at the start of a line (multiline mode):

```regex
^\s*(Resolves|Closes|Fixes|Fix|JIRA|Jira ticket)[:\s]+\b<KEY>\b
```

The `^` anchor (multiline) is critical — it rejects mentions buried inside paragraphs and only accepts explicit "this PR closes X" statements. Treat the body's `\r\n` line endings as line boundaries when applying the regex.

**Tier 3 — auto-EXCLUDE by default, show in preview (key in body, no closing keyword).** Anything else where `\b<KEY>\b` matches in body but Tier 1/2 didn't fire. This includes:
- `### Motivation\n<URL containing the key>` (very common in datadog-agent PRs — but ambiguous between "this PR closes the ticket" and "this PR was motivated by the ticket but doesn't fully close it")
- Bare key mentions in the middle of a paragraph
- Markdown links to the Jira ticket without a closing keyword

Record these as `tier3_candidates` with: PR URL, title, and the 1-2 surrounding lines from the body that contain the key. Step 10 surfaces this list in the preview so the user can opt-in via `Edit`.

**Tier 4 — auto-exclude (key in body with explicit cross-reference language).** Apply this BEFORE checking Tier 3. If `\b<KEY>\b` is preceded within 60 characters on the same line by any of:

```regex
(?i)\b(follow[- ]?up|related[ -]?to|see also|supersedes|cf\.|referenced in|context for|after|before|companion to|part of)\b[^\n]{0,60}\b<KEY>\b
```

… the PR is a cross-reference, not a resolution of the searched key. Drop it silently — these would be noise in the preview's skipped-list too. Reasoning: when an author writes "Follow-up to OTAGENT-392" in the body, they are not claiming to resolve OTAGENT-392.

**Filter order summary (Phase B only; Tier 0 PRs bypass this entirely):**
1. Tier 1 (title match) → include
2. Tier 4 (cross-reference language in body) → exclude silently
3. Tier 2 (closing keyword in body) → include
4. Tier 3 (any other body match) → exclude by default, surface in preview

### Additional drop rules (apply after tier classification, all tiers including Tier 0)

- **Drop reverts** — PRs whose title or body explicitly says "revert" of another PR in the list (keep both if they end up cancelling out — let the user decide during preview).
- **Drop bot PRs** — `author.is_bot == true`, or author login ending with `[bot]`, unless they touch release notes or code. Backport bot PRs (e.g. titled `[Backport X.Y.x] [OTAGENT-XXX] …`) should be deduplicated against the primary PR by Jira key; keep the primary and drop the backport from the recap (mention only in a footnote if relevant).

If zero PRs are found across both phases, ask the user via `AskUserQuestion` whether to:
- Provide PR URLs manually (comma-separated list)
- Continue with an empty PR section (recap will rely on Epic description + user input)
- Cancel

## Step 5: Fetch PR details

For each merged PR identified in Step 4, fetch full details. Run these in parallel when possible.

```bash
gh pr view <number> \
  --repo <owner>/<repo> \
  --json title,body,files,labels,mergedAt,baseRefName,author
```

Collect:
- `title`, `body`, `mergedAt`, `baseRefName`
- `files[].path` — used in Step 7 (signal classification) and Step 6 (release-note discovery)
- `labels[].name` — look for `team/opentelemetry`, `component/*`, `changelog/*`, `qa/*`

## Step 6: Read release notes from the PRs

For each PR, filter `files[].path` for entries that start with:
- `releasenotes/notes/` (main Agent)
- `releasenotes-dca/notes/` (Cluster Agent)
- `releasenotes-installscript/notes/` (Install script)

For each matching path, fetch its content from the PR's `baseRefName` of that repo. The PRs in scope here may live in `DataDog/datadog-agent` or in other Datadog repos that follow the same `releasenotes/` convention.

```bash
gh api "repos/<owner>/<repo>/contents/<path>?ref=<baseRefName>" \
  --jq '.content' | base64 -d
```

If a release-note file is **added** by the PR (not yet on `main` at the PR's base), fall back to fetching it from the merge commit:

```bash
gh api "repos/<owner>/<repo>/contents/<path>?ref=<mergeSha>" \
  --jq '.content' | base64 -d
```

Parse each YAML release note and collect the section name (`features`, `enhancements`, `fixes`, `upgrade`, `deprecations`, `security`, `other`, `issues`) and its prose. Keep the original wording — release notes are already customer-facing.

**Empty release notes are common, not an error.** Several teams (notably `team/opentelemetry-agent`, which routinely labels DDOT PRs `changelog/no-changelog`) ship user-visible behaviour without reno entries. If no release notes are found across all PRs, do not stop or warn — Step 9 has an explicit fallback that derives `What's new` from PR titles and bodies. Record this fact so the preview can mention `_None of the linked PRs included release notes_` at the bottom of the `Linked PRs` section.

## Step 7: Classify the change

Build a `signals` object with these fields, derived from PR file paths and release-note prose. Each field can have multiple values; omit it from the recap entirely when no signal matches.

**Signal path** (based on file path prefixes across all PRs):
- `comp/otelcol/`, `comp/core/configsync/`, `cmd/otel-agent/`, `pkg/config/otel/` → `agent-otel-ingest` and/or `ddot`
- `pkg/opentelemetry-mapping-go/` → `dd-exporter-contrib`
- Helm charts, `chart/`, `Dockerfile.otel`, `images/otel-agent/` → `standalone-ddot`

**Signal type** (based on file path prefixes):
- `pkg/logs/`, `comp/logs/` → `logs`
- `pkg/metrics/`, `pkg/opentelemetry-mapping-go/otlp/metrics/`, `comp/metrics/` → `metrics`
- `pkg/trace/`, `cmd/trace-agent/` → `traces`
- `pkg/collector/corechecks/ebpf/`, `pkg/gpu/`, `pkg/security/`, `pkg/profiler/` → `profiles/system`

A change can be in multiple signal types — record them all.

**API & config changes**: scan PR diffs and release-note content for:
- File paths matching `pkg/config/setup/config.go`, `pkg/config/**/*.yaml`, `comp/core/config/`, `cmd/*/subcommands/*/command.go`
- Release-note prose containing tokens like `config`, `option`, `setting`, `API`, `endpoint`, `flag`

If you find evidence, list the concrete config keys / API surfaces added or changed (from the release notes when available; from the diff when not). If no evidence, mark this section as "None".

**Repositories touched**: distinct `repository.nameWithOwner` from Step 4, sorted alphabetically.

## Step 8: Ask the user for the remaining sections

Use `AskUserQuestion` to collect the pieces that cannot be derived from code:

1. **Performance impact** — text. Accept `Not measured` as a valid answer. Encourage including benchmark numbers, links to load tests, regression-detector results.
2. **Agent footprint** — text. Accept `No change` as a valid answer. Encourage including RSS / CPU / binary size deltas with links to quality-gates dashboard if available.
3. **Customer utilisation tracking** — text. Ask the user how PMs can track customer adoption: dashboard URL, internal metric name, log query, telemetry event, or `Not tracked yet`.

Ask all three in a single `AskUserQuestion` call (multi-question form), each with a free-text option in addition to the most common canned answers (`Not measured`, `No change`, `Not tracked yet`).

## Step 9: Render the recap

Read [recap-template.md](recap-template.md) and substitute each `{{placeholder}}` with the rendered content:

| Placeholder | Source |
|---|---|
| `{{epic_key}}` | Step 1 |
| `{{epic_summary}}` | Step 2 |
| `{{summary}}` | Synthesised 1-2 sentences for PMs. Prefer combining the Epic summary + release-note headlines. If no release notes exist (see fallback below), combine the Epic summary with the most user-relevant PR titles. |
| `{{whats_new}}` | Bullet list of user-facing wins. Sources, in order of preference: (1) release-note prose from the `features` and `enhancements` sections; (2) `fixes` / `upgrade` / `deprecations` sections if user-visible; (3) **fallback when release notes are empty**: derive one bullet per PR using the PR title (stripped of the `[OTAGENT-XXX]` prefix and rewritten in user-facing language) plus a one-sentence summary of the PR body's `### What does this PR do?` section. Many DDOT and trace PRs are labelled `changelog/no-changelog` even when they ship user-visible behaviour changes, so the fallback is the normal path for some teams. Drop bullets that describe purely internal refactors, dep bumps with no behaviour change, or test-only PRs. |
| `{{signal_path}}` | Step 7 values as a bullet list, or omit the section if empty |
| `{{signal_type}}` | Step 7 values as a bullet list, or omit the section if empty |
| `{{api_config_changes}}` | Step 7 content, or omit the section if "None" and no relevant release notes |
| `{{performance_impact}}` | Step 8 answer, or omit the section if the user picked `Not measured` AND no perf-related release notes exist |
| `{{agent_footprint}}` | Step 8 answer, or omit the section if `No change` AND no footprint-relevant release notes exist |
| `{{repositories_touched}}` | Step 7 list, bullet form |
| `{{customer_tracking}}` | Step 8 answer, or omit the section if `Not tracked yet` |
| `{{linked_prs}}` | Bullet list. Format per PR: `- [<repo>#<number>](<url>) — <title>` followed by indented bullets listing release notes (`  - <section>: <one-line excerpt>`). Group Tier 0 PRs (from Jira Development panel) first with a `_(linked via Jira)_` annotation, then Tier 1/2 PRs from GitHub search. On Claude Code there are no Tier 0 PRs, so start directly with Tier 1/2. |
| `{{pr_discovery_note}}` | PR-discovery caveat for the report (hybrid). Render one of the two blocks defined in **"PR discovery note"** below: the quiet footnote whenever Tier 0 (Phase A1) was unavailable this run (always on Claude Code/Rovo) and there is no shortfall; the loud warning whenever `pr_shortfall` (Step 4 Phase A2) is non-empty. Render empty (collapse, no blank line) only when Tier 0 was available (Cursor) **and** `pr_shortfall` is empty. |

### PR discovery note (`{{pr_discovery_note}}`)

Render this inside the *Linked PRs & release notes* section, right after `{{linked_prs}}`. It degrades gracefully when the MCP server cannot read Jira's Development panel — pick exactly one variant:

- **Quiet footnote** — when Tier 0 (Phase A1) was unavailable this run (always the case on Claude Code/Rovo) **and** `pr_shortfall` is empty. The PR list came only from GitHub search, so the report must not imply it is authoritative:

  ```
  > ℹ️ _PR discovery was GitHub-search-only: this MCP server has no dev-status endpoint, so PRs attached only in Jira's Development panel can't be read. The list above may be incomplete._
  ```

- **Loud warning** — whenever `pr_shortfall` is non-empty (Jira's A2 count exceeds the PRs found for one or more keys — proof that attached PRs were missed). List each affected key, then the MCP-install recommendation:

  ```
  > ⚠️ **Some Jira-attached PRs could not be retrieved due to MCP server limitations.**
  > Jira's Development panel reports more linked PRs than GitHub search found:
  > - OTAGENT-307: Jira 4, found 2
  > These PRs are attached in Jira, but their URLs live in the dev-status endpoint, which this MCP server (official Atlassian Rovo) does not expose.
  > **To recover them:** connect a community Atlassian MCP server that wraps dev-status — e.g. [`mcp-atlassian`](https://github.com/sooperset/mcp-atlassian) (the `user-atlassian` server in the Cursor setup), which provides `jira_get_issue_development_info`. Then re-run the recap.
  ```

  Render one `- <KEY>: Jira <n>, found <m>` line per entry in `pr_shortfall`.

- **Empty** — when Tier 0 was available (Cursor) **and** `pr_shortfall` is empty. Collapse the placeholder, leaving no blank line.

Keep the footnote tone neutral — it is PM-facing. The MCP-install recommendation appears only in the loud variant, so stakeholders never see developer-setup noise unless PRs were actually missed.

Drop the HTML rendering-rules comment from the template before producing the final markdown.

When omitting an optional section, remove its `##` heading too — do not leave empty headings.

## Step 10: Preview and approval

Print the rendered markdown to the chat under a heading like `### Preview — <EPIC-KEY> recap`.

**Before the recap, print a one-line PR discovery summary** so the user can see what was found where (on Claude Code the Tier 0 count is always 0 — Tier 0 is unavailable, so make clear PR discovery was GitHub-only):

```
> Found N PRs: X via Jira Development panel (Tier 0), Y via GitHub search (Tier 1/2). Z Tier 3 candidates skipped (see below).
```

If `jira_pr_counts` from Phase A2 is available and any key has more Jira-linked PRs than Phase B found (i.e. `pr_shortfall` is non-empty), print a warning block after the summary line. The same shortfall is **also** rendered into the posted report via `{{pr_discovery_note}}` (Step 9) — the preview shows it to the engineer, the report carries it to stakeholders:

```
> ⚠️ OTAGENT-307: Jira says 4 linked PRs, found 2. Check Tier 3 candidates or the Jira Development panel.
```

**After the rendered recap, if `tier3_candidates` from Step 4 is non-empty, print a separate `### Skipped (Tier 3 — opt-in)` block.** This block lives outside the recap markdown — it is not part of what gets posted to Jira, only shown to the user. Format:

```
### Skipped (Tier 3 — opt-in)

The following PRs mention the searched Jira keys in their body but without a closing keyword (`Resolves`/`Closes`/`Fixes`/`JIRA:`). They are excluded from the recap by default. Pick `Edit` and say "include #N, #M" to add them.

- [<repo>#<number>](<url>) — <title>
  - Searched key: <KEY>
  - Body context: «…<the 1-2 lines around the key match>…»
- …
```

Then call `AskUserQuestion` with options:

- `Post` — proceed to Step 11
- `Edit` — ask the user for free-text instructions. Common edits:
  - "shorten the summary"
  - "drop the perf section"
  - "include #N" / "include all Tier 3" — promote one or more Tier 3 candidates into the recap, then re-render
  - "add a note about backport"

  Apply the edits and loop back to the preview.
- `Cancel` — go to Step 12 (save and exit).

If `--dry-run` was set in Step 1, skip the question entirely and jump straight to Step 12 with `cancel` semantics. Print a notice that the recap was not posted because of `--dry-run`. The Tier 3 skipped block is still printed in dry-run mode so the user can review what was filtered.

## Step 11: Post the comment

Call the **Post comment** tool for your runtime (see *Tool mapping*) with the rendered markdown from Step 9 (including the attribution footer):
- **Cursor:** `jira_add_comment` — `issue_key: EPIC-KEY`, `body: <markdown>`.
- **Claude Code:** `mcp__atlassian__addCommentToJiraIssue` — `cloudId`, `issueIdOrKey: EPIC-KEY`, `commentBody: <markdown>`, `contentFormat: "markdown"`.

Do not set `visibility` / `commentVisibility` — this is a regular comment.

**POST-action verification**: re-fetch the Epic with the **Fetch issue** tool (Cursor: `comment_limit=5`; Claude Code: `fields: ["comment"]`) and confirm the new comment is present (match by the attribution footer string `Generated by create-epic-recap`). If verification fails, surface the error to the user and do not retry automatically.

On success, print:

```
Recap posted: https://datadoghq.atlassian.net/browse/<EPIC-KEY>
```

## Step 12: Save and exit (when not posting)

When the user picks `Cancel` or `--dry-run` was specified:
1. Write the final rendered markdown to `/tmp/<EPIC-KEY>-recap.md`.
2. Print the path so the user can pick it up later, e.g.: `Saved draft to /tmp/OTAGENT-820-recap.md`.
3. Exit cleanly.

## Errors and edge cases

- **Authentication failure on Atlassian MCP** — stop, do not try a different transport, ask the user to authenticate.
- **Claude Code (Rovo) `cloudId` errors** — if a Jira call fails citing an invalid/unknown `cloudId`, call `mcp__atlassian__getAccessibleAtlassianResources` and use the `datadoghq` site's `id` (UUID) instead of the hostname.
- **`jira_get_issue_development_info` returns 500 or empty** — the `/rest/dev-status/1.0/issue/detail` endpoint is fragile: it does not tolerate parallel calls, occasionally goes down entirely, and requires exact CamelCase `"GitHub"` for `application_type`. When A1 fails, A2 (`customfield_10000`) provides PR counts for cross-validation. Phase B (GitHub search) always provides actual PR details regardless of A1 status. **On Claude Code (Rovo) this tool does not exist at all** — A1 is always skipped there, and A2 + Phase B are the entire pipeline.
- **`gh` not authenticated** — run `gh auth status`; if it fails, ask the user to run `gh auth login`.
- **`gh search prs` rate-limited** — back off once for 60 seconds, then retry once; if still failing, ask the user to provide PR URLs manually (Step 4 fallback).
- **Very large PR set (> 25 PRs)** — present a numbered list and ask the user via `AskUserQuestion` whether to include all of them or to narrow down by date / label / repo.
- **Posting fails** — keep the rendered markdown on disk (Step 12 path) and report the error verbatim; do not silently retry.

## Important constraints

- **Never post without explicit user approval** — `--dry-run` and `Cancel` must result in no Jira write.
- **Never modify the Epic description or other fields** — comments only.
- **Always include the attribution footer** so future readers know the recap was AI-generated.
- **Never include secrets, internal-only URLs, or sensitive customer data** in the recap. If a release note contains a customer name, mask it as `<customer>` and flag it during preview.
- **Comment body uses markdown** — pass Markdown directly; do not pre-render to ADF or Wiki markup. On Cursor `jira_add_comment` accepts Markdown in `body`; on Claude Code pass `commentBody` with `contentFormat: "markdown"`.

---

## Appendix A — Claude Code (Rovo) capability gap

The Rovo server exposes **no Development/dev-status endpoint**, so on Claude Code:

- **Phase A1 / Tier 0 is unavailable** — you cannot read linked-PR **URLs** from Jira. Do **not** use remote links as a substitute: `getJiraIssueRemoteIssueLinks` does **not** return GitHub PRs (the GitHub↔Jira integration stores them in dev-status, not remote links — verified empty on OTAGENT-304).
- The only Jira-side PR signal is **Phase A2** (`customfield_10000`), which gives **counts + state, never URLs**.
- Therefore **all PR URLs come from Phase B (`gh search prs`)**; the A2 counts are only a weak cross-check. `gh search prs <KEY>` over-matches body/cross-reference mentions, so a count can coincide while the actual PRs differ — e.g. OTAGENT-307: Jira count 4, `gh` also returns 4, but two carry a *different* key in their title. Lean on the Tier 1/2/4 classification and surface Tier 3 in preview rather than trusting a count match.
- **Surface this limitation in the recap itself**, not just in the preview. Step 9 renders a `{{pr_discovery_note}}` into the posted report: a quiet footnote that PR discovery was GitHub-only, escalating to a loud warning + MCP-install recommendation when an A2 shortfall proves Jira-attached PRs were missed. The fix for the user is to connect a community Atlassian MCP server with a dev-status tool (e.g. `mcp-atlassian`, the `user-atlassian` server, which exposes `jira_get_issue_development_info`); the recap tells them so rather than failing silently.

### `customfield_10000` shape (A2)

On Rovo, `customfield_10000` comes back as a **single string**, e.g. `{pullrequest={dataType=pullrequest, state=MERGED, stateCount=4}, json={"cachedValue":{…}}}`. Extract the embedded `json={…}` object and read `cachedValue.summary.pullrequest.overall.count` and `…overall.state`. Keys whose dev field has no `pullrequest` block (or is null) have count 0.

## Appendix B — cloudId and large responses (Claude Code)

**cloudId:** pass `cloudId: "datadoghq.atlassian.net"` directly — the site hostname works. If a call rejects it, call `mcp__atlassian__getAccessibleAtlassianResources` once and use the returned `id` (UUID) for the `datadoghq` site.

**Large search responses:** `searchJiraIssuesUsingJql` can exceed the MCP response token cap and be spilled to a file. To avoid/handle this, request **only the fields you need** (e.g. `["summary","status","issuetype","customfield_10000"]`); if the result is saved to a file, parse it with `jq` instead of re-reading the whole blob.

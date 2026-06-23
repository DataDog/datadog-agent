---
name: create-epic-recap
description: Generate a resolution recap for a Jira Epic from merged PRs and release notes, then post it as a comment after user approval
argument-hint: "<EPIC-KEY e.g. OTAGENT-820> [--dry-run]"
model: sonnet
allowed-tools: Bash, Read, Write, Glob, Grep, AskUserQuestion
---

Generate a resolution recap for the Jira Epic **$ARGUMENTS**, aggregating information from merged GitHub PRs and release notes. Show a preview and post the recap as a comment on the Epic only after explicit user approval.

This skill is intended for an engineer who has finished work on an Epic and wants to communicate the resolution to PMs and stakeholders without losing flow. See [OTAGENT-1038](https://datadoghq.atlassian.net/browse/OTAGENT-1038) for motivation.

## Step 1: Parse arguments

Parse `$ARGUMENTS`:
- **EPIC-KEY** (required, first positional): Jira issue key matching `^[A-Z][A-Z0-9_]+-\d+$`, e.g. `OTAGENT-820`.
- `--dry-run` (optional flag): render and preview the recap, but never post it. The skill prints the markdown and exits.

If `EPIC-KEY` is missing or does not match the pattern, stop and ask the user to provide it.

## Step 2: Fetch the Epic

Call the Atlassian MCP tool `mcp__atlassian__jira_get_issue` (or, depending on the runtime, `jira_get_issue` on the `user-atlassian` server) with:
- `issue_key`: `EPIC-KEY`
- `fields`: `"summary,description,status,issuetype,labels,assignee,reporter"`
- `comment_limit`: `10`

Validate:
- If the issue cannot be found, stop and inform the user.
- If `issue_type.name` is not `Epic`, stop and inform the user that this skill only works on Epics. Suggest using `/run-jira` for non-Epic issues.

Keep the response in memory — `summary`, `description`, `status`, `labels` will be used during rendering.

## Step 3: Fetch child issues

Call `mcp__atlassian__jira_search` with:
- `jql`: `"Epic Link" = <EPIC-KEY>` (use double quotes around `Epic Link` exactly as shown).
- `fields`: `"summary,status,issuetype,assignee,labels"`
- `limit`: `50`

Collect each child issue's `key`, `summary`, and `status.name`. These keys (plus the Epic key itself) become the search terms for finding PRs in Step 4.

If the search returns zero children, that is fine — some Epics are resolved by PRs that reference the Epic key directly. Continue with just `<EPIC-KEY>` as the search term.

## Step 4: Find merged PRs

Build the list of Jira keys to search for: `[EPIC-KEY, <child keys>]`.

For each key (run in parallel as separate Bash invocations when more than one), search merged PRs across the `DataDog` GitHub org:

```bash
gh search prs \
  --owner DataDog \
  --merged \
  --limit 50 \
  --json repository,title,url,number,labels,body,author \
  -- "<KEY>"
```

Notes:
- The `--` separates flags from the query; the bare key goes in the query.
- `gh search prs` matches the key in title, body, and commit messages.
- Deduplicate by `(repository.nameWithOwner, number)` across all keys.

Filter out:
- PRs whose title or body explicitly says "revert" of another PR in the list (keep both if they end up cancelling out — let the user decide during preview).
- Bot PRs (`author.is_bot == true`, or author login ending with `[bot]`) unless they touch release notes or code.

If zero PRs are found, ask the user via `AskUserQuestion` whether to:
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
| `{{summary}}` | Synthesised 1-2 sentences combining Epic summary + release-note headlines, written for PMs |
| `{{whats_new}}` | Bullet list of user-facing wins, primarily from release-note prose (`features` and `enhancements` sections) |
| `{{signal_path}}` | Step 7 values as a bullet list, or omit the section if empty |
| `{{signal_type}}` | Step 7 values as a bullet list, or omit the section if empty |
| `{{api_config_changes}}` | Step 7 content, or omit the section if "None" and no relevant release notes |
| `{{performance_impact}}` | Step 8 answer, or omit the section if the user picked `Not measured` AND no perf-related release notes exist |
| `{{agent_footprint}}` | Step 8 answer, or omit the section if `No change` AND no footprint-relevant release notes exist |
| `{{repositories_touched}}` | Step 7 list, bullet form |
| `{{customer_tracking}}` | Step 8 answer, or omit the section if `Not tracked yet` |
| `{{linked_prs}}` | Bullet list. Format per PR: `- [<repo>#<number>](<url>) — <title>` followed by indented bullets listing release notes (`  - <section>: <one-line excerpt>`) |

Drop the HTML rendering-rules comment from the template before producing the final markdown.

When omitting an optional section, remove its `##` heading too — do not leave empty headings.

## Step 10: Preview and approval

Print the rendered markdown to the chat under a heading like `### Preview — <EPIC-KEY> recap`. Then call `AskUserQuestion` with options:

- `Post` — proceed to Step 11
- `Edit` — ask the user for free-text instructions (e.g. "shorten the summary", "drop the perf section", "add a note about backport"). Apply the edits and loop back to the preview.
- `Cancel` — go to Step 12 (save and exit).

If `--dry-run` was set in Step 1, skip the question entirely and jump straight to Step 12 with `cancel` semantics. Print a notice that the recap was not posted because of `--dry-run`.

## Step 11: Post the comment

Call `mcp__atlassian__jira_add_comment` (or `jira_add_comment` on the `user-atlassian` server) with:
- `issue_key`: `EPIC-KEY`
- `body`: the rendered markdown from Step 9 (including the attribution footer)

Do not set `visibility` or `public` — this is a regular comment.

**POST-action verification**: call `mcp__atlassian__jira_get_issue` again with `comment_limit=5` and confirm the new comment is present (match by the attribution footer string `Generated by create-epic-recap`). If verification fails, surface the error to the user and do not retry automatically.

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
- **`gh` not authenticated** — run `gh auth status`; if it fails, ask the user to run `gh auth login`.
- **`gh search prs` rate-limited** — back off once for 60 seconds, then retry once; if still failing, ask the user to provide PR URLs manually (Step 4 fallback).
- **Very large PR set (> 25 PRs)** — present a numbered list and ask the user via `AskUserQuestion` whether to include all of them or to narrow down by date / label / repo.
- **Posting fails** — keep the rendered markdown on disk (Step 12 path) and report the error verbatim; do not silently retry.

## Important constraints

- **Never post without explicit user approval** — `--dry-run` and `Cancel` must result in no Jira write.
- **Never modify the Epic description or other fields** — comments only.
- **Always include the attribution footer** so future readers know the recap was AI-generated.
- **Never include secrets, internal-only URLs, or sensitive customer data** in the recap. If a release note contains a customer name, mask it as `<customer>` and flag it during preview.
- **Comment body uses markdown** — `jira_add_comment` accepts Markdown in `body`; do not pre-render to ADF or Wiki markup.

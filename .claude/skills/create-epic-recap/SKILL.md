---
name: create-epic-recap
description: "Use when an engineer or manager asks to recap, summarize, or post a Jira Epic resolution: gathers completed child issues, merged GitHub PRs, and release notes, previews a stakeholder-ready recap, and posts only after approval."
argument-hint: "<EPIC-KEY e.g. OTAGENT-820> [--dry-run]"
model: sonnet
allowed-tools: Bash, Read, Write, Glob, Grep, AskUserQuestion, mcp__atlassian__getJiraIssue, mcp__atlassian__searchJiraIssuesUsingJql, mcp__atlassian__getJiraIssueRemoteIssueLinks, mcp__atlassian__addCommentToJiraIssue, mcp__atlassian__getAccessibleAtlassianResources
---

Generate a resolution recap for the Jira Epic **$ARGUMENTS**, aggregating merged GitHub PRs and release notes. Show a preview and post it as a comment on the Epic **only after explicit user approval**. This lets an engineer who has finished an Epic communicate the resolution to PMs and stakeholders without losing flow (motivation: [OTAGENT-1038](https://datadoghq.atlassian.net/browse/OTAGENT-1038)).

**Owning team:** `team/opentelemetry-agent` (`@DataDog/opentelemetry-agent`)

## Reference files (load as needed)

- **`references/runtime-tooling.md`** — runtime detection, the Cursor vs Claude Code tool mapping, `cloudId`, large responses, JQL. **Read this before Step 2.**
- **`references/pr-discovery.md`** — the full Step 4 algorithm: Phase A/B, tier classification + regex, drop rules, throttling, the `{{pr_discovery_note}}` variants, and the Claude Code capability gap. **Read this before Step 4.**

This skill runs in **two runtimes** with different Atlassian MCP servers (Cursor's `mcp-atlassian` and Claude Code's Atlassian Rovo). The key gap: Rovo has **no dev-status endpoint**, so Phase A1/Tier 0 is Cursor-only and `cloudId` is required on every Rovo call. Whenever a step says "call the *Fetch issue* / *Search children* / *Post comment* tool", look up the exact tool and params in `references/runtime-tooling.md`.

## Prerequisites

If any check fails, stop and tell the user what to fix.

1. **Atlassian MCP server** — connected and authenticated (Cursor: `user-atlassian`; Claude Code: Atlassian Rovo). Probe with a known issue fetch; if it fails, ask the user to authenticate/connect.
2. **GitHub CLI (`gh`)** — installed and authenticated for the DataDog org. Run `gh auth status`; if no active account, ask the user to run `gh auth login`.

## Example

**Input:** `/create-epic-recap OTAGENT-304 --dry-run`

Fetches Epic OTAGENT-304, finds its completed child issues, discovers merged PRs across all child keys (Jira Development panel on Cursor + GitHub search), reads release notes from the PR file lists, renders the recap, prints a preview, and — because of `--dry-run` — saves a draft without posting:

```
Saved draft to /tmp/OTAGENT-304-recap.md
```

## Step 1: Parse arguments

- **EPIC-KEY** (required, first positional): matches `^[A-Z][A-Z0-9_]+-\d+$`, e.g. `OTAGENT-820`. If missing or malformed, stop and ask the user.
- `--dry-run` (optional flag): render and preview only, never post.

## Step 2: Fetch the Epic

Call the **Fetch issue** tool (see `references/runtime-tooling.md`) requesting fields `summary, description, status, issuetype, labels, assignee, reporter`.

Validate:
- If the issue cannot be found, stop and inform the user.
- Read the issue type from whichever shape the runtime returns — accept **both** `issuetype.name` (Rovo) **and** `issue_type.name` (some `mcp-atlassian` versions). If the resolved name is not `Epic`, stop and tell the user this skill only works on Epics (suggest `/run-jira` for non-Epics). Do not reject just because one of the two shapes is absent.

Keep `summary`, `description`, `status`, `labels` in memory for rendering.

## Step 3: Fetch child issues

Call the **Search children** tool with the Epic-children JQL (see `references/runtime-tooling.md`), `fields: summary, status, issuetype, assignee, labels`, limit `50`. On Claude Code, also request `customfield_10000` in this call so Step 4 Phase A2 counts come back for free. If the response spills to a file, parse with `jq`.

Collect each child's `key`, `summary`, `status.name`, and `status.category` (accept `status.statusCategory.key` too).

**Filter out unfinished work** — drop children whose status category is `To Do`/`new`/`indeterminate`; keep only `Done` (statuses like `Done`, `Closed`, `Resolved`). Record dropped children in `skipped_children` so you can mention them if asked.

An empty list of completed children is fine — some Epics are resolved by PRs that reference the Epic key directly. Continue with just `<EPIC-KEY>` as the search term.

## Step 4: Find merged PRs

**Read `references/pr-discovery.md` and follow it.** In short:
- Build the key list `[EPIC-KEY, <completed child keys>]`.
- **Phase A** (Cursor only): A1 reads Tier 0 PRs from the Jira Development panel; A2 reads merged-PR counts from `customfield_10000` into `jira_pr_counts` for cross-validation. On Claude Code, skip A1 (no dev-status) and use A2 + Phase B only.
- **Phase B** (both runtimes): `gh search prs` once per key, then classify each hit into Tier 1 (include) / Tier 2 (include) / Tier 3 (opt-in, surfaced in preview) / Tier 4 (cross-ref, drop).
- Apply the revert/bot drop rules, dedup across phases, and record `tier3_candidates` and `pr_shortfall`.
- If zero PRs are found, use the manual-URL / empty-section / cancel fallback from the reference.

## Step 5: Fetch PR details

For each merged PR, fetch details (run in parallel when possible):

```bash
gh pr view <number> \
  --repo <owner>/<repo> \
  --json title,body,files,labels,mergedAt,baseRefName,author,mergeCommit
```

Collect:
- `title`, `body`, `mergedAt`, `baseRefName`.
- `mergeCommit.oid` — **store as `mergeSha`**; Step 6 needs it to read release-note files added by the PR that aren't on the base branch. If `mergeCommit` is null (rebase/squash merge), fall back to the last commit's `oid`: `gh pr view <number> --repo <owner>/<repo> --json commits --jq '.commits[-1].oid'`.
- `files[].path` — used in Steps 6 and 7.
- `labels[].name` — note `team/opentelemetry`, `component/*`, `changelog/*`, `qa/*`.

## Step 6: Read release notes from the PRs

For each PR, filter `files[].path` for entries starting with `releasenotes/notes/` (main Agent), `releasenotes-dca/notes/` (Cluster Agent), or `releasenotes-installscript/notes/` (Install script). PRs may live in `datadog-agent` or other Datadog repos using the same convention.

Fetch each matching path from the PR's `baseRefName`:

```bash
gh api "repos/<owner>/<repo>/contents/<path>?ref=<baseRefName>" --jq '.content' | base64 -d
```

If the file was **added** by the PR (not yet on base) or has since been removed, fall back to the merge commit via `mergeSha`:

```bash
gh api "repos/<owner>/<repo>/contents/<path>?ref=<mergeSha>" --jq '.content' | base64 -d
```

If `mergeSha` is unavailable, skip the file and note its release note could not be read — do not fail; Step 9's PR-body fallback covers it.

Parse each YAML note and collect the section name (`features`, `enhancements`, `fixes`, `upgrade`, `deprecations`, `security`, `other`, `issues`) and its prose. Keep the original wording — release notes are already customer-facing.

**Empty release notes are common, not an error.** Several teams (notably `team/opentelemetry-agent`, which routinely labels DDOT PRs `changelog/no-changelog`) ship user-visible behaviour without reno entries. If none are found, do not stop or warn — Step 9 derives `What's new` from PR titles/bodies. Record this so the preview can note `_None of the linked PRs included release notes_`.

## Step 7: Classify the change

Build a `signals` object from PR file paths and release-note prose. Each field can have multiple values; omit it from the recap when no signal matches.

**Signal path** (file-path prefixes):
- `comp/otelcol/`, `comp/core/configsync/`, `cmd/otel-agent/`, `pkg/config/otel/` → `agent-otel-ingest` and/or `ddot`
- `pkg/opentelemetry-mapping-go/` → `dd-exporter-contrib`
- Helm charts, `chart/`, `Dockerfile.otel`, `images/otel-agent/` → `standalone-ddot`

**Signal type** (file-path prefixes; a change can hit several):
- `pkg/logs/`, `comp/logs/` → `logs`
- `pkg/metrics/`, `pkg/opentelemetry-mapping-go/otlp/metrics/`, `comp/metrics/` → `metrics`
- `pkg/trace/`, `cmd/trace-agent/` → `traces`
- `pkg/collector/corechecks/ebpf/`, `pkg/gpu/`, `pkg/security/`, `pkg/profiler/` → `profiles/system`

**API & config changes** — scan PR diffs and release-note content for paths like `pkg/config/setup/config.go`, `pkg/config/**/*.yaml`, `comp/core/config/`, `cmd/*/subcommands/*/command.go`, or prose with `config`/`option`/`setting`/`API`/`endpoint`/`flag`. If found, list the concrete config keys / API surfaces (from release notes when available, else the diff). Otherwise mark "None".

**Repositories touched** — distinct `repository.nameWithOwner` from Step 4, sorted alphabetically.

## Step 8: Ask the user for the remaining sections

Use a single multi-question `AskUserQuestion` for the pieces that cannot be derived from code, each with a free-text option plus the canned answer:

1. **Performance impact** — text; `Not measured` valid. Encourage benchmark numbers / load-test / regression-detector links.
2. **Agent footprint** — text; `No change` valid. Encourage RSS / CPU / binary-size deltas with quality-gates dashboard links.
3. **Customer utilisation tracking** — how PMs track adoption: dashboard URL, metric name, log query, telemetry event, or `Not tracked yet`.

## Step 9: Render the recap

Read [recap-template.md](recap-template.md) and substitute each `{{placeholder}}`:

| Placeholder | Source |
|---|---|
| `{{epic_key}}` | Step 1 |
| `{{epic_summary}}` | Step 2 |
| `{{summary}}` | Synthesised 1-2 sentences for PMs. Prefer Epic summary + release-note headlines; if no release notes, combine the Epic summary with the most user-relevant PR titles. |
| `{{whats_new}}` | Bullet list of user-facing wins, in order of preference: (1) `features`/`enhancements` release-note prose; (2) `fixes`/`upgrade`/`deprecations` if user-visible; (3) **fallback when release notes are empty**: one bullet per PR from the title (strip the `[OTAGENT-XXX]` prefix, rewrite in user-facing language) + a one-sentence summary of the PR body's `### What does this PR do?`. The fallback is the normal path for `changelog/no-changelog` teams. Drop internal refactors, behaviourless dep bumps, and test-only PRs. |
| `{{signal_path}}` | Step 7 bullet list, or omit the section if empty |
| `{{signal_type}}` | Step 7 bullet list, or omit the section if empty |
| `{{api_config_changes}}` | Step 7 content, or omit if "None" and no relevant release notes |
| `{{performance_impact}}` | Step 8 answer, or omit if `Not measured` AND no perf-related release notes |
| `{{agent_footprint}}` | Step 8 answer, or omit if `No change` AND no footprint-relevant release notes |
| `{{repositories_touched}}` | Step 7 list, bullet form |
| `{{customer_tracking}}` | Step 8 answer, or omit if `Not tracked yet` |
| `{{linked_prs}}` | Bullet list `- [<repo>#<number>](<url>) — <title>`, then indented release-note bullets (`  - <section>: <one-line excerpt>`). Group Tier 0 PRs first with a `_(linked via Jira)_` annotation, then Tier 1/2 from GitHub search. On Claude Code there are no Tier 0 PRs — start with Tier 1/2. |
| `{{pr_discovery_note}}` | One of the quiet/loud/empty variants — see *PR discovery note* in `references/pr-discovery.md`. Quiet whenever Tier 0 was unavailable (always on Rovo) and no shortfall; loud whenever `pr_shortfall` is non-empty; empty only when Tier 0 was available (Cursor) and `pr_shortfall` is empty. |

Drop the HTML rendering-rules comment from the template before producing the final markdown. When omitting an optional section, remove its `##` heading too — no empty headings.

## Step 10: Preview and approval

Print the rendered markdown under `### Preview — <EPIC-KEY> recap`.

**Before the recap, print a one-line PR discovery summary** (on Claude Code the Tier 0 count is always 0 — make clear discovery was GitHub-only):

```
> Found N PRs: X via Jira Development panel (Tier 0), Y via GitHub search (Tier 1/2). Z Tier 3 candidates skipped (see below).
```

If `pr_shortfall` is non-empty, print a warning block after the summary (the same shortfall is also rendered into the posted report via `{{pr_discovery_note}}`):

```
> ⚠️ OTAGENT-307: Jira says 4 linked PRs, found 2. Check Tier 3 candidates or the Jira Development panel.
```

**After the recap, if `tier3_candidates` is non-empty, print a separate `### Skipped (Tier 3 — opt-in)` block** (shown to the user only, not posted to Jira):

```
### Skipped (Tier 3 — opt-in)

The following PRs mention the searched Jira keys in their body but without a closing keyword (`Resolves`/`Closes`/`Fixes`/`JIRA:`). They are excluded by default. Pick `Edit` and say "include #N, #M" to add them.

- [<repo>#<number>](<url>) — <title>
  - Searched key: <KEY>
  - Body context: «…<the 1-2 lines around the key match>…»
```

Then call `AskUserQuestion` with options:
- `Post` — proceed to Step 11.
- `Edit` — ask for free-text instructions (e.g. "shorten the summary", "drop the perf section", "include #N" / "include all Tier 3" to promote candidates, "add a note about backport"), apply, and loop back to the preview.
- `Cancel` — go to Step 12.

If `--dry-run` was set, skip the question and jump to Step 12 with `cancel` semantics, printing a notice that the recap was not posted. The Tier 3 block is still printed in dry-run.

## Step 11: Post the comment

Call the **Post comment** tool (see `references/runtime-tooling.md`) with the rendered markdown from Step 9 (including the attribution footer). Do not set `visibility`/`commentVisibility` — this is a regular comment.

**POST-action verification:** re-fetch the Epic (Cursor: `comment_limit=5`; Claude Code: `fields: ["comment"]`) and confirm the new comment is present (match the footer string `Generated by create-epic-recap`). If verification fails, surface the error and do not retry automatically.

On success, print:

```
Recap posted: https://datadoghq.atlassian.net/browse/<EPIC-KEY>
```

## Step 12: Save and exit (when not posting)

When the user picks `Cancel` or `--dry-run` was specified:
1. Write the final markdown to `/tmp/<EPIC-KEY>-recap.md`.
2. Print the path, e.g. `Saved draft to /tmp/OTAGENT-820-recap.md`.
3. Exit cleanly.

## Errors and edge cases

- **Atlassian MCP auth failure** — stop, do not try a different transport, ask the user to authenticate.
- **Claude Code `cloudId` errors** — see `references/runtime-tooling.md` (use `getAccessibleAtlassianResources` to resolve the UUID).
- **`jira_get_issue_development_info` 500/empty** — the dev-status endpoint is fragile (no parallel calls, occasional downtime, exact CamelCase `"GitHub"`). On failure, A2 + Phase B carry the pipeline; on Rovo this tool doesn't exist at all. Full handling in `references/pr-discovery.md`.
- **`gh` not authenticated** — `gh auth status`; if it fails, ask the user to `gh auth login`.
- **`gh search prs` rate-limited** — back off 60 s, retry once; if still failing, ask for manual PR URLs (Step 4 fallback).
- **Very large PR set (> 25 PRs)** — present a numbered list and ask via `AskUserQuestion` whether to include all or narrow by date / label / repo.
- **Posting fails** — keep the markdown on disk (Step 12 path), report the error verbatim, do not silently retry.

## Important constraints

- **Never post without explicit user approval** — `--dry-run` and `Cancel` must result in no Jira write.
- **Never modify the Epic description or other fields** — comments only.
- **Always include the attribution footer** so future readers know the recap was AI-generated.
- **Never include secrets, internal-only URLs, or sensitive customer data.** Mask customer names as `<customer>` and flag during preview.
- **Comment body is markdown** — pass it directly; do not pre-render to ADF/Wiki. On Cursor `jira_add_comment` takes Markdown in `body`; on Claude Code pass `commentBody` with `contentFormat: "markdown"`.

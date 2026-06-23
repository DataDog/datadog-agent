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

Collect each child issue's `key`, `summary`, `status.name`, and `status.category` (or `status.statusCategory.key` depending on the API shape — accept both).

**Filter out unfinished work.** A resolution recap is about what shipped, so drop any child issue whose `status.category` (or `statusCategory.key`) is `To Do` / `new` / `indeterminate`. Keep only children in the `Done` category (statuses like `Done`, `Closed`, `Resolved`). Record the dropped children in a `skipped_children` list so you can mention them in the preview if the user asks.

If the resulting list of completed children is empty, that is fine — some Epics are resolved by PRs that reference the Epic key directly. Continue with just `<EPIC-KEY>` as the search term.

## Step 4: Find merged PRs

Build the list of Jira keys to search for: `[EPIC-KEY, <completed child keys from Step 3>]`.

### Phase A — Jira Development panel (Tier 0)

For each key, call `jira_get_issue_development_info` on the `user-atlassian` server with:
- `issue_key`: `<KEY>`
- `application_type`: `"GitHub"` (**CamelCase is mandatory** — lowercase `"github"` returns empty results; this is a Jira REST API quirk in `/rest/dev-status/1.0/issue/detail`)

Do **not** use the batch tool `jira_get_issues_development_info` — it returns HTTP 500 when `application_type` is passed.

From each response, collect `pullRequests` entries where `status == "MERGED"`. Record each as a **Tier 0** PR with fields: `id` (e.g. `#52248`), `name` (title), `url`, `status`, `source.branch`, `destination.branch`, `author`, `reviewers`, `lastUpdate`, `repositoryUrl`.

**Tier 0 PRs are auto-included with the highest confidence** — they are explicitly linked to the Jira issue by the GitHub↔Jira integration. No heuristic filtering is needed.

If a call returns empty `pullRequests` or errors, that is fine — Phase B covers it. Log it and continue.

Deduplicate Tier 0 results by `(repository, PR number)` across all keys.

### Phase B — GitHub search (Tiers 1–4)

**Skip keys that already have at least one Tier 0 PR from Phase A** — unless you want maximum recall. For keys with zero Tier 0 results, and optionally for all keys to catch PRs missed by Jira integration, run `gh search prs`.

**One key per request — `OR` is not supported.** `gh search prs` treats the query as a literal string, so `"OTAGENT-410 OR OTAGENT-411"` will match zero PRs (verified in the wild). Issue one `gh search prs` call per Jira key:

```bash
gh search prs \
  --owner DataDog \
  --merged \
  --limit 20 \
  --json repository,title,url,number,labels,author \
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
| `{{linked_prs}}` | Bullet list. Format per PR: `- [<repo>#<number>](<url>) — <title>` followed by indented bullets listing release notes (`  - <section>: <one-line excerpt>`). Group Tier 0 PRs (from Jira Development panel) first with a `_(linked via Jira)_` annotation, then Tier 1/2 PRs from GitHub search. |

Drop the HTML rendering-rules comment from the template before producing the final markdown.

When omitting an optional section, remove its `##` heading too — do not leave empty headings.

## Step 10: Preview and approval

Print the rendered markdown to the chat under a heading like `### Preview — <EPIC-KEY> recap`.

**Before the recap, print a one-line PR discovery summary** so the user can see what was found where:

```
> Found N PRs: X via Jira Development panel (Tier 0), Y via GitHub search (Tier 1/2). Z Tier 3 candidates skipped (see below).
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
- **`jira_get_issue_development_info` returns empty or errors** — this is expected for some projects where the GitHub↔Jira integration is partially configured, or when `application_type` casing is wrong. Phase B (GitHub search) covers these cases. Log a note like `Jira dev panel returned no PRs for <KEY>, falling back to GitHub search` and continue.
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

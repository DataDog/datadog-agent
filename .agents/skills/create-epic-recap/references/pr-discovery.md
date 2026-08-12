# PR discovery (Step 4 detail)

Find every **merged** PR that resolves the Epic or one of its completed children. Build the list of Jira keys to search for: `[EPIC-KEY, <completed child keys from Step 3>]`.

Two phases run for **all** keys: Phase A reads Jira's Development panel (Cursor only), Phase B searches GitHub (both runtimes). Phase B is the source of truth for PR URLs; Phase A adds high-confidence Tier 0 hits and a cross-validation count.

## Phase A — Jira Development panel (Tier 0)

### A1 — Detail endpoint (full PR info) — Cursor only

> **Claude Code (Rovo): SKIP A1 entirely.** This server has no dev-status / Development-panel tool, so Tier 0 PR URLs cannot be read from Jira. Go straight to A2 (counts) and rely on Phase B for URLs.

For each key, call `jira_get_issue_development_info` on the `user-atlassian` server with:
- `issue_key`: `<KEY>`
- `application_type`: `"GitHub"` — **CamelCase is mandatory**; lowercase `"github"` returns empty results (Jira REST API quirk in `/rest/dev-status/1.0/issue/detail`).
- `data_type`: `"pullrequest"` — **mandatory**; omitting it causes HTTP 500 with `"message":"dataType"`.

Do **not** use the batch tool `jira_get_issues_development_info` — it returns HTTP 500 when `application_type` is passed.

**Throttling — the dev-status API does not tolerate parallel calls** (parallel requests trigger HTTP 500 rate limiting):
- **Sequential calls only** — one at a time, never in parallel.
- **~1 s pause** between calls (`sleep 1`).
- **Retry once on 500** — wait 5 s, then retry the failing key. If the retry also fails, record the key as "Phase A1 miss" and move on.
- **Fail-fast after 3 consecutive 500s** — stop A1, log `Phase A1 aborted after 3 consecutive 500s, falling back to A2`, and proceed to A2 for **all** keys.

From each successful response, collect `pullRequests` entries where `status == "MERGED"`. Record each as a **Tier 0** PR with: `id` (e.g. `#52248`), `name`, `url`, `status`, `source.branch`, `destination.branch`, `author`, `reviewers`, `lastUpdate`, `repositoryUrl`.

**Tier 0 PRs are auto-included with the highest confidence** — they are explicitly linked to the Jira issue by the GitHub↔Jira integration; no heuristic filtering needed. Deduplicate by `(repository, PR number)` across all keys.

### A2 — Development summary field (PR count validation)

If A1 failed (fail-fast) or returned zero Tier 0 PRs for any key, fetch the cached development summary. This reads a standard Jira custom field via the stable `/rest/api/2/issue/` API (not the flaky `dev-status` endpoint).

Call the **Fetch issue** tool (see `references/runtime-tooling.md`) requesting only `customfield_10000` (the "Development" field):
- **Cursor:** `jira_get_issue` — `fields: "customfield_10000"`, `comment_limit: 0`. Value under `customfield_10000.value`.
- **Claude Code:** `mcp__atlassian__getJiraIssue` — `fields: ["customfield_10000"]`. Value is a **string** (see *customfield_10000 shape* below), not an object — extract the embedded `json={…}` first.

In both cases the embedded JSON holds `cachedValue.summary.pullrequest.overall.count` (total linked PRs in **any** state) and `…overall.state` (`MERGED`, `OPEN`, etc.).

**Count merged PRs only — `overall.count` is not comparable.** This skill collects only **merged** PRs, but `overall.count` totals every linked PR regardless of state, producing a false `pr_shortfall` whenever a completed issue still has an open/backport PR linked. Derive a **merged-only** count:
- If the summary exposes a per-state breakdown (`stateCount` for `state: "MERGED"`, or a per-state map), use the merged count directly.
- Otherwise, only treat `overall.count` as the expected merged count when `overall.state == "MERGED"`. If `overall.state` is anything else (`OPEN`, `DECLINED`, mixed), mark this key's merged count **unknown** and skip the shortfall comparison for it.

Record per-key merged counts in a `jira_pr_counts` map: `{ "OTAGENT-307": 4, … }`. Keys with no `pullrequest` in the summary (or null `customfield_10000`) have count 0. Keys whose merged count is unknown are omitted so they never trigger a shortfall.

A2 calls can run **in parallel** (standard Jira API). Batch them with Step 2/3 calls when possible.

**Cross-validation:** after tier classification, for each key with a **known** merged count, compare the number of included PRs (Tier 0 + Tier 1 + Tier 2) against `jira_pr_counts`. Record every key where Phase B found fewer in a `pr_shortfall` list (`{key, jira_count, found_count}`). This drives both the Step 10 preview warning and the report's `{{pr_discovery_note}}` (see *PR discovery note* below).

## Phase B — GitHub search (Tiers 1–4)

**Always run Phase B for all keys** — even those with Tier 0 PRs from A1 — for maximum recall (the Jira integration may miss PRs in other repos or PRs where the key is only in the body).

**One key per request — `OR` is not supported.** `gh search prs` treats the query as a literal string, so `"OTAGENT-410 OR OTAGENT-411"` matches zero PRs. One call per key:

```bash
gh search prs \
  --owner DataDog \
  --merged \
  --limit 20 \
  --json repository,title,url,number,labels,author,body \
  -- "<KEY>"
```

**Throttling — avoid GitHub's secondary rate limit** (~30 req/min):
- Cap parallelism at **at most 4 concurrent** `gh search prs` calls.
- Sleep **~500 ms between waves** (`sleep 0.5`).
- On HTTP 403 `secondary rate limit`: back off 60 s, then retry the failing keys one at a time.

Notes:
- The `--` separates flags from the query; the bare key goes in the query (no quotes needed inside the JSON shell array).
- `gh search prs` matches the key in title, body, and commit messages — the source of the false positives the tiers filter out.
- Deduplicate by `(repository.nameWithOwner, number)` across all keys **and** against Tier 0 PRs from Phase A. If a PR was already collected as Tier 0, keep it as Tier 0 — do not downgrade.

## Tier classification (Phase B PRs only)

Classify each Phase B PR into one of four tiers, based on where the searched key `<KEY>` appears. This is precision-over-recall by design — Tier 3 is shown in preview so the user can opt-in. Apply tiers in this order; first match wins:

1. **Tier 1 — auto-include** (title match) → include
2. **Tier 4 — cross-reference language in body** → exclude silently
3. **Tier 2 — closing keyword in body** → include
4. **Tier 3 — any other body match** → exclude by default, surface in preview

**Tier 1 — key in PR title.** Word-boundary match `\b<KEY>\b` against `title`. Standard forms: `[OTAGENT-410] …` (bracketed, the standard `team/opentelemetry-agent` format), bare `OTAGENT-410`, `(OTAGENT-410)`.

**Tier 4 — explicit cross-reference language (checked BEFORE Tier 3).** If `\b<KEY>\b` is preceded within 60 chars on the same line by cross-reference language, the PR references but does not resolve the key — drop it silently:

```regex
(?i)\b(follow[- ]?up|related[ -]?to|see also|supersedes|cf\.|referenced in|context for|after|before|companion to|part of)\b[^\n]{0,60}\b<KEY>\b
```

Reasoning: "Follow-up to OTAGENT-392" is not a claim to resolve OTAGENT-392.

**Tier 2 — key with closing keyword in body.** Match the key in `body` only when preceded by a GitHub closing keyword or JIRA-field label, at the start of a logical line (multiline mode):

```regex
^\s*(Resolves|Closes|Fixes|Fix|JIRA|Jira ticket)[:\s]+\b<KEY>\b
```

The `^` anchor (multiline) is critical — it rejects mentions buried inside paragraphs. Treat `\r\n` as line boundaries.

**Tier 3 — key in body, no closing keyword.** Anything else where `\b<KEY>\b` matches in body but Tier 1/2 didn't fire (e.g. `### Motivation\n<URL containing the key>`, bare mentions mid-paragraph, markdown links without a closing keyword). Record as `tier3_candidates` with: PR URL, title, and the 1-2 surrounding body lines containing the key. Surfaced in the Step 10 preview for opt-in.

## Additional drop rules (apply after tier classification, all tiers including Tier 0)

- **Drop reverts** — PRs whose title/body explicitly says "revert" of another PR in the list (keep both if they cancel out — let the user decide during preview).
- **Drop bot PRs** — `author.is_bot == true`, or login ending `[bot]`, unless they touch release notes or code. Backport bot PRs (e.g. `[Backport X.Y.x] [OTAGENT-XXX] …`) are deduplicated against the primary PR by Jira key; keep the primary, drop the backport (mention only in a footnote if relevant).

## Zero PRs found

If zero PRs are found across both phases, ask the user via `AskUserQuestion` whether to:
- Provide PR URLs manually (comma-separated)
- Continue with an empty PR section (recap relies on Epic description + user input)
- Cancel

## PR discovery note (`{{pr_discovery_note}}`)

Rendered in Step 9 inside the *Linked PRs & release notes* section, right after `{{linked_prs}}`. It degrades gracefully when the MCP server cannot read Jira's Development panel — pick exactly one variant:

- **Quiet footnote** — when Tier 0 (Phase A1) was unavailable this run (always on Claude Code/Rovo) **and** `pr_shortfall` is empty:

  ```
  > ℹ️ _PR discovery was GitHub-search-only: this MCP server has no dev-status endpoint, so PRs attached only in Jira's Development panel can't be read. The list above may be incomplete._
  ```

- **Loud warning** — whenever `pr_shortfall` is non-empty (proof that attached PRs were missed). One `- <KEY>: Jira <n>, found <m>` line per entry, then the MCP-install recommendation:

  ```
  > ⚠️ **Some Jira-attached PRs could not be retrieved due to MCP server limitations.**
  > Jira's Development panel reports more linked PRs than GitHub search found:
  > - OTAGENT-307: Jira 4, found 2
  > These PRs are attached in Jira, but their URLs live in the dev-status endpoint, which this MCP server (official Atlassian Rovo) does not expose.
  > **To recover them:** connect a community Atlassian MCP server that wraps dev-status — e.g. [`mcp-atlassian`](https://github.com/sooperset/mcp-atlassian) (the `user-atlassian` server in the Cursor setup), which provides `jira_get_issue_development_info`. Then re-run the recap.
  ```

- **Empty** — when Tier 0 was available (Cursor) **and** `pr_shortfall` is empty. Collapse the placeholder, leaving no blank line.

Keep the tone neutral — it is PM-facing. The MCP-install recommendation appears only in the loud variant, so stakeholders never see developer-setup noise unless PRs were actually missed.

## Claude Code (Rovo) capability gap

The Rovo server exposes **no Development/dev-status endpoint**, so on Claude Code:

- **Phase A1 / Tier 0 is unavailable** — you cannot read linked-PR **URLs** from Jira. Do **not** use remote links as a substitute: `getJiraIssueRemoteIssueLinks` does **not** return GitHub PRs (the integration stores them in dev-status, not remote links — verified empty on OTAGENT-304).
- The only Jira-side PR signal is **Phase A2** (`customfield_10000`): counts + state, never URLs.
- Therefore **all PR URLs come from Phase B (`gh search prs`)**; A2 counts are a weak cross-check. `gh search prs <KEY>` over-matches body/cross-reference mentions, so a count can coincide while the actual PRs differ (e.g. OTAGENT-307: Jira count 4, `gh` also returns 4, but two carry a *different* key in their title). Lean on the Tier 1/2/4 classification and surface Tier 3 in preview rather than trusting a count match.
- **Surface this limitation in the recap itself** via `{{pr_discovery_note}}` (Step 9), not just in the preview.

### customfield_10000 shape (A2)

On Rovo, `customfield_10000` comes back as a **single string**, e.g. `{pullrequest={dataType=pullrequest, state=MERGED, stateCount=4}, json={"cachedValue":{…}}}`. Extract the embedded `json={…}` object and read `cachedValue.summary.pullrequest.overall.count` and `…overall.state`. Keys whose dev field has no `pullrequest` block (or is null) have count 0.

Remember `overall.count` totals **all** linked PRs, not just merged ones. The summary-prefix `state`/`stateCount` pair (`state=MERGED, stateCount=4` above) is the merged-only count when present; otherwise only trust `overall.count` as a merged count when `overall.state == "MERGED"`. When the state is `OPEN`/mixed, treat the merged count as unknown and skip the shortfall comparison for that key.

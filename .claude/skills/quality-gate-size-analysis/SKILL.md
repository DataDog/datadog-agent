---
name: quality-gate-size-analysis
description: Analyze static quality gate on-disk size changes, correlate with Confluence exception records and GitHub PRs by milestone
argument-hint: "<time-range e.g. 3mo, 6mo> [optional: specific release e.g. 7.77]"
---

Analyze the agent's static quality gate on-disk size metrics, pull approved exception records from Confluence, fetch PR data from GitHub with milestone-based release attribution, and produce a cross-referenced report.

## Step 1: Query Datadog Metrics

### 1a. Relative on-disk size (per-commit deltas)

Query `datadog.agent.static_quality_gate.relative_on_disk_size` filtered to `ci_commit_ref_slug:main`, grouped by `gate_name` and `arch`:

```
max:datadog.agent.static_quality_gate.relative_on_disk_size{ci_commit_ref_slug:main} by {gate_name,arch}
```

- Use `from=now-$TIME_RANGE` and `to=now`
- Request `raw_data=true` to get CSV with timestamps for spike identification
- Identify the **largest single-commit jumps** per gate — these are the candidates for exception correlation

### 1b. Absolute package size (branch comparison)

Query `datadog.agent.package.size` for `main` and relevant release branches to measure inter-release deltas:

```
max:datadog.agent.package.size{git_ref:main,package:datadog-agent,os:debian,arch:amd64}
max:datadog.agent.package.size{git_ref:7-XX-x,package:datadog-agent,os:debian,arch:amd64}
```

- Compare the stable average of each release branch to quantify the bump
- Include both the prior release and the current release branches

### 1c. Other useful metrics

- `datadog.agent.static_quality_gate.on_disk_size` — absolute on-disk size per gate
- `datadog.agent.static_quality_gate.on_wire_size` — compressed/network size
- `datadog.agent.static_quality_gate.max_allowed_on_disk_size` — configured thresholds

### 1d. Reference dashboard

The [Agent Package size metrics](https://app.datadoghq.com/dashboard/7gi-nmp-qdh) dashboard contains all relevant widgets.

## Step 2: Fetch Confluence Exception Records

Exception records live in the **ABLD** Confluence space under folder ID `5996904656`.

### 2a. Enumerate all exception pages

Use CQL search:

```
ancestor = 5996904656 AND type = page ORDER BY created DESC
```

- `cloudId`: `datadoghq.atlassian.net`
- Exclude the `[TEMPLATE]` page from results

### 2b. Fetch each page body

For every page returned, call `getConfluencePage` with `contentFormat=markdown` to extract:

- **Decision status**: Approved / Pending / Declined
- **Disk size increase**: the number in the "Measures" or "Bounds Granted" row
- **Scope**: which platforms/packages are affected
- **Linked PRs**: PR numbers/URLs mentioned in the "Feature" or "PR" rows
- **Requester**: who filed the exception
- **Expiry / Payback**: any commitments to recover the size

### 2c. Build an exception table

Compile all exceptions into a structured table with columns: Title, Date, Decision, Disk Increase, Scope, PRs, Requester.

## Step 3: Pull GitHub PR Data

### 3a. Find PRs linked from Confluence

Extract PR numbers from the Confluence pages (they appear as GitHub URLs in the exception text). For each PR:

```bash
gh pr view <NUMBER> --repo DataDog/datadog-agent --json number,title,mergedAt,milestone,labels,state
```

The **milestone** field (e.g. `7.77.0`) determines which release the PR belongs to. Do NOT use labels for release attribution.

### 3b. Search for additional size-impacting PRs

Search for PRs merged in the time window that may have size impact but no exception:

```bash
gh pr list --repo DataDog/datadog-agent --base main --state merged \
  --search "merged:>=YYYY-MM-DD" --json number,title,mergedAt,milestone,labels
```

Filter for PRs with labels like `qa/rc-required`, `component/system-probe`, etc., and cross-check against the exception list.

### 3c. Search by feature name

For exceptions that don't directly link a PR, search by keyword:

```bash
gh pr list --repo DataDog/datadog-agent --base main --state merged \
  --search "<feature-keyword> merged:>=YYYY-MM-DD" --json number,title,mergedAt,milestone
```

## Step 4: Cross-Reference and Correlate

### 4a. Match metric spikes to PRs

For each significant spike in `relative_on_disk_size`:
1. Identify the approximate date from the metric timestamp
2. Find PRs merged on or just before that date
3. Check if those PRs are covered by a Confluence exception

### 4b. Match PRs to releases via milestone

Group all size-impacting PRs by their **milestone** (not by label or merge date):
- `milestone:7.75.0` → Release 7.75
- `milestone:7.76.0` → Release 7.76
- etc.

### 4c. Identify gaps

Flag:
- PRs with size impact but **no Confluence exception**
- Confluence exceptions still in **Pending** status
- Exceptions with **empty "Bounds Granted"** fields
- PRs with **no milestone** set

## Step 5: Generate Report

Write a markdown report to the repository root (e.g. `quality_gate_size_analysis_YYYYQN.md`) with these sections:

1. **Executive Summary** — one paragraph overview
2. **Metrics Overview** — absolute sizes per release branch, largest jumps table
3. **Confluence Exceptions** — full table split by Approved vs Pending
4. **Release Attribution** — PRs grouped by milestone, each linked to its exception status
5. **Coverage Gaps** — list of unmatched PRs or pending exceptions
6. **Methodology** — brief description of data sources and correlation approach

## Key Details and Pitfalls

- The `relative_on_disk_size` metric uses `ci_commit_ref_slug` (not `git_ref`) for branch filtering.
- The `package.size` metric uses `git_ref` for branch filtering — branch names use hyphens (e.g. `7-77-x`).
- The Confluence folder `5996904656` is a **folder**, not a page — use CQL `ancestor =` to search, not `getConfluencePageDescendants`.
- PR **milestones** determine release attribution, not labels. Use `gh pr view --json milestone`.
- The `gate_name` tag values look like `static_quality_gate_docker_agent_amd64` — use these to distinguish between agent variants (docker, MSI, DCA, dogstatsd, IoT, heroku, FIPS).
- Some exceptions cover multiple PRs across multiple releases (e.g. PAR landed in 7.76, with follow-on work in 7.77 and 7.78).

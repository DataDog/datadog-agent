---
name: analyze-quality-gate-security-base-experiment
description: Compare production CWS metrics with SMP regression experiment metrics and lading config for quality_gate_security_base
user_invocable: true
allowed-tools: Bash, Read, Skill
---

# analyze-quality-gate-security-base-experiment

Compare production CWS `perf_buffer.events.write` rates against what the `quality_gate_security_base` lading config generates and what SMP captures.

## Step 1: Invoke /explain-lading-config

Call the `explain-lading-config` skill on `test/regression/cases/quality_gate_security_base/lading/lading.yaml`. Record the expected filesystem event rate (opens + renames per second).

## Step 2: Verify pup auth

Run `pup auth status`. If expired, run `pup auth login`. If auth fails entirely, note that Datadog MCP tools are available as a fallback for read-only queries.

## Step 3: Query production metric

Query the production `perf_buffer.events.write` rate filtered to file activity, averaged across all hosts:

```bash
pup metrics query --query 'avg:datadog.runtime_security.perf_buffer.events.write{category:file_activity}' --from 1d --to now
```

Compute mean/min/max from the pointlist values.

## Step 4: Query SMP metric

Query the SMP equivalent metric captured during regression runs:

```bash
pup metrics query --query 'avg:single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write{category:file_activity}' --from 7d --to now
```

SMP runs are currently infrequent — expect sparse data (3-10 points over 7d).

## Step 5: Compare and analyze

Perform a three-way comparison focused on whether the lading config accurately models production:

| Source | Value | Description |
|--------|-------|-------------|
| Lading config | from Step 1 | opens/sec + renames/sec (from explain-lading-config) |
| Production avg | from Step 3 | avg across all hosts, category:file_activity |
| SMP captured | from Step 4 | single-instance metric from SMP runs |

**Important:** The `file_tree` generator's rename operation performs 2 syscalls (rename away, rename back — see `file_tree.rs:rename_folder`). When computing effective event rate from lading config: `open_per_second + (rename_per_second × 2)`.

Analysis:
- Compare the production average against the lading effective event rate
- Compare the SMP captured metric against production — if SMP matches production, the experiment is valid
- If production differs from the lading rate, flag the gap and suggest specific lading config changes (e.g. adjust `rename_per_second` or `open_per_second`)

## Step 6: Output report

Print a markdown report answering "does the lading config reflect the production case?" with three sections:

1. **Lading config** — configured rate (events/sec, broken down by operation type)
2. **SMP captured** — rate the SMP run measured
3. **Production average** — rate across all hosts (avg, category:file_activity)

Followed by a one-line assessment: match, mismatch, or insufficient data.

## Step 7: Propose lading config changes

If a mismatch was found, propose concrete changes to `test/regression/cases/quality_gate_security_base/lading/lading.yaml`:

1. **Compute target rates** — use the production median as the target effective event rate. Work backwards from `target = open_per_second + (rename_per_second × 2)` to find values that match. Prefer adjusting `rename_per_second` first (it has 2x leverage), then `open_per_second`.
2. **Show the diff** — print the exact YAML change (old value → new value) for each field.
3. **Offer to apply** — ask whether to edit the lading config file directly.

If data was insufficient (e.g. no SMP points), offer to re-query with a wider time range before proposing changes.

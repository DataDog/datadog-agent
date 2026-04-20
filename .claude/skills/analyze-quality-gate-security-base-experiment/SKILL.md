---
name: analyze-quality-gate-security-base-experiment
description: Compare production CWS metrics with SMP regression experiment metrics from quality_gate_security_base and quality_gate_security_idle
user_invocable: true
allowed-tools: Bash, Read, Skill
---

# analyze-quality-gate-security-base-experiment

Compare production CWS `perf_buffer.events.write` open rates against what the `quality_gate_security_base` lading config generates and what SMP captures, using `quality_gate_security_idle` as the idle baseline.

Two SMP experiments form a pair:
- **`quality_gate_security_idle`** — CWS enabled, no generator. Measures the floor: background event noise and idle memory footprint.
- **`quality_gate_security_base`** — CWS enabled, `file_tree` generator. Measures loaded overhead.

The difference between them isolates the generator's contribution, which should match production workload above background noise.

The primary signal is `event_type:open` — it is the most common event type in production CWS workloads. Open events have high background noise from always-on VFS hooks (`hook_do_dentry_open`, `hook_vfs_open` under the `"*"` probe selector), but the `security_idle` experiment directly measures this noise floor, enabling clean subtraction.

## Step 1: Invoke /explain-lading-config

Call the `explain-lading-config` skill on `test/regression/cases/quality_gate_security_base/lading/lading.yaml`. Record the expected open event rate: `open_per_second` events/sec (each open operation produces 1 open event). Note `rename_per_second` but do not include it in the primary comparison.

Note: `quality_gate_security_idle` has `generator: []` (no load generator). There is nothing to explain for it — its purpose is to measure the idle baseline.

## Step 2: Verify pup auth

Run `pup auth status`. If expired, run `pup auth login`. If auth fails entirely, note that Datadog MCP tools are available as a fallback for read-only queries.

## Step 3: Identify the latest job for each experiment and query it exclusively

SMP runs multiple replicas per experiment; each is tagged with a unique `job_id`. Pin the analysis to one specific run by finding the latest `job_id` per experiment and filtering on it. This avoids time-window heuristics and the 300 s rollup gap-fill that deflates means when a short capture only partially occupies a bucket.

### 3a. List jobs grouped by `job_id`

Run grouped queries over the last 1 day (widen to 2d, then 7d only if nothing is returned). These are **inventory-only** — do not use their values, only their `job_id` scopes and timestamps:

```bash
# security_base — jobs
pup metrics query --query 'avg:single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write{event_type:open,variant:comparison,experiment:quality_gate_security_base} by {job_id}.as_rate()' --from 1d --to now

# security_idle — jobs
pup metrics query --query 'avg:single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write{event_type:open,variant:comparison,experiment:quality_gate_security_idle} by {job_id}.as_rate()' --from 1d --to now
```

For each returned series, read its `scope` (contains `job_id:<UUID>`) and its `pointlist`. Null-safe: drop points whose value is `None`. Per job, record the timestamp of the **last non-zero point**. The `job_id` with the most recent last-non-zero timestamp is "the latest job" for that experiment.

### 3b. Re-query each latest `job_id` exclusively

Add `job_id:<UUID>` to the tag filter. Pin the query window to the job itself — not a relative range like `--from 1h`. Using a relative range is non-deterministic across runs: it shifts with wall-clock time, and the API rollup may include or exclude an edge bucket depending on where "now" lands, producing different pointlists and different means for the same `job_id`.

From Step 3a you have the first and last non-zero epoch-ms (`FIRST_MS`, `LAST_MS`) for each latest `job_id`. Derive a deterministic window:

```bash
FROM_TS=$(( FIRST_MS / 1000 - 60 ))   # 1 minute of padding before
TO_TS=$((   LAST_MS  / 1000 + 60 ))   # 1 minute of padding after
```

Pass these as absolute Unix seconds to `pup`. The window width should stay under ~1 h so the API still returns 20-second interval data.

Run in parallel:

```bash
# security_base — latest job
pup metrics query --query 'avg:single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write{event_type:open,variant:comparison,experiment:quality_gate_security_base,job_id:<BASE_UUID>}.as_rate()' --from "$BASE_FROM_TS" --to "$BASE_TO_TS"
pup metrics query --query 'avg:single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write{category:file_activity,variant:comparison,experiment:quality_gate_security_base,job_id:<BASE_UUID>}.as_rate()' --from "$BASE_FROM_TS" --to "$BASE_TO_TS"

# security_idle — latest job
pup metrics query --query 'avg:single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write{event_type:open,variant:comparison,experiment:quality_gate_security_idle,job_id:<IDLE_UUID>}.as_rate()' --from "$IDLE_FROM_TS" --to "$IDLE_TO_TS"
pup metrics query --query 'avg:single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write{category:file_activity,variant:comparison,experiment:quality_gate_security_idle,job_id:<IDLE_UUID>}.as_rate()' --from "$IDLE_FROM_TS" --to "$IDLE_TO_TS"
```

The `.as_rate()` modifier guarantees the result is in events/sec. Both this metric and the production metric are type `rate` (statsd_interval=10), so `.as_rate()` is a no-op today — but it documents intent and protects against metadata changes.

Compute `mean` over **non-zero, non-null** points only. Record the `job_id`, the first and last non-zero timestamps (capture window), the interval returned, the data points count, and the raw sum used for the mean.

When reporting `capture_first_ts` / `capture_last_ts`, do not compute ISO strings by hand — use the shell:

```bash
date -u -r $(( MS / 1000 )) +%Y-%m-%dT%H:%M:%SZ
```

Always print the raw epoch-ms alongside the ISO string so the reader can verify the conversion.

Do not include min/max in the report.

### 3c. Sanity checks

- If fewer than ~5 non-zero points come back for a latest job, the capture may be in flight or interrupted. Try the second-most-recent `job_id` and report both so the reviewer can judge.
- If the `security_base` and `security_idle` latest `job_id`s ran more than 24 h apart, flag the staleness — background noise floor drifts over time.
- Never fall back to wider windows with coarser rollups (≥300 s intervals) as a substitute — rollup gap-fill deflates per-job means when a capture only partially occupies a bucket.

### 3d. Anti-pattern (why `job_id` is authoritative)

Do **not** identify runs by time heuristics (contiguous non-zero clusters, gap-detection, "last N minutes"). A single SMP run at 20 s resolution still contains transient zero buckets that masquerade as run boundaries once rolled up, and multiple replicas of the same experiment run concurrently, so any time-slice may contain overlapping jobs. The `job_id` tag is the only authoritative grouping for a single run.

## Step 4: Query production metric (weekly mean)

Query production data over the last 7 days to get the weekly mean. Run both queries in parallel:

```bash
# Primary — open-only, weekly mean
pup metrics query --query 'avg:datadog.runtime_security.perf_buffer.events.write{event_type:open}.as_rate()' --from 7d --to now

# Context — all file activity, weekly mean
pup metrics query --query 'avg:datadog.runtime_security.perf_buffer.events.write{category:file_activity}.as_rate()' --from 7d --to now
```

All queries use `.as_rate()` so every value in the comparison chain is in the same unit (events/sec):
- Lading config: `open_per_second` × 1 = events/sec
- SMP captured: `.as_rate()` → events/sec
- Production: `.as_rate()` → events/sec

Compute the mean from the weekly pointlist values. Do not include min/max in the report.

## Step 5: Compare and analyze

The primary comparison uses `event_type:open` values only.

All values must be in **events/sec** (guaranteed by `.as_rate()` on metric queries, and by the explain-lading-config breakdown for lading).

**Primary table — open events/sec:**

| Source | Value (events/sec) | Description |
|--------|---------------------|-------------|
| Lading config | from Step 1 | `open_per_second` × 1 |
| SMP — security_idle (open) | from Step 3 | Background open noise with CWS, no generator |
| SMP — security_base (open) | from Step 3 | Open rate with generator running |
| Generator contribution | computed | security_base - security_idle |
| Org2 per-host avg (open, weekly) | from Step 4 | production `event_type:open` `.as_rate()` weekly mean |

**Supplementary context** (not used for tuning decisions):

| Source | Value (events/sec) | Description |
|--------|---------------------|-------------|
| SMP — security_idle (all file activity) | from Step 3 | idle `category:file_activity` — background noise floor |
| SMP — security_base (all file activity) | from Step 3 | loaded `category:file_activity` |
| Org2 per-host avg (all file activity, weekly) | from Step 4 | production `category:file_activity` weekly mean |

Analysis:
- **Idle baseline check**: The `security_idle` open rate captures background noise from always-on VFS hooks. Record this value as the noise floor.
- **Generator contribution check**: `generator_contribution = security_base_open - security_idle_open` reflects the generator's observable effect on CWS. Note that this need not equal `open_per_second` — one lading syscall can produce multiple CWS events (and vice versa).
- **Production validity check**: Compare the SMP `security_base` open rate against the org2 per-host weekly open average. If they diverge, flag the gap.
- Note the `category:file_activity` totals for reference

## Step 6: Output report

Print a markdown report answering "does the lading config reflect the production open workload?" with these sections:

1. **Lading config** — configured open rate (`open_per_second` × 1 = events/sec)
2. **SMP Idle Baseline (security_idle, latest job `<IDLE_UUID>`)** — open rate and file activity with CWS but no generator, showing the job_id, capture window (first → last non-zero epoch-ms with ISO derived via `date -u -r`), interval, data points count, and the mean expressed as `sum=<S> / n=<N> = <mean>` so the reader can reproduce the arithmetic without trusting the model
3. **SMP Loaded (security_base, latest job `<BASE_UUID>`)** — open rate and file activity with CWS and generator, showing the job_id, capture window (epoch-ms + `date -u -r` derived ISO), interval, data points count, and the mean expressed as `sum=<S> / n=<N> = <mean>`
4. **Generator Contribution** — delta between security_base and security_idle for open events (between the two latest jobs)
5. **Org2 per-host avg (open, weekly)** — production open rate weekly mean

Followed by a supplementary note showing `category:file_activity` totals from both SMP experiments and production for reference.

Followed by a one-line assessment: match, mismatch, or insufficient data.

## Step 7: Propose lading config changes

If a mismatch was found, propose concrete changes to `test/regression/cases/quality_gate_security_base/lading/lading.yaml`:

1. **Target**: the production open weekly mean from Step 4.
2. **Direction of change**: adjust `open_per_second` up or down to push `security_base_open` toward the target. Do not assume a 1:1 mapping between lading syscalls and CWS events — the security-agent can observe multiple kernel events per lading operation (e.g. a single rename may surface more than two events) and may dedupe or filter others. Treat the lading-to-CWS ratio as empirical, derived from the specific `job_id` used in Step 3b — cite the job_id alongside the ratio so the tuning decision is traceable.
3. **Show the diff** — print the exact YAML change (old value → new value) for `open_per_second`. Do not adjust `rename_per_second` — rename events are not the primary signal.
4. **Offer to apply** — ask whether to edit the lading config file directly.

If data was insufficient (e.g. the latest job had too few non-zero points), offer to use the second-most-recent `job_id` before proposing changes. Do not fall back to wider time windows with coarser rollups.

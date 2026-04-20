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

## Step 3: Query SMP metrics from both experiments

Query SMP metrics from both `security_base` (loaded) and `security_idle` (baseline). The `experiment:` tag filter disambiguates the two experiments. Run all queries in parallel:

```bash
# security_base — open-only (primary signal)
pup metrics query --query 'avg:single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write{event_type:open,variant:comparison,experiment:quality_gate_security_base}.as_rate()' --from 7d --to now

# security_base — all file activity (context)
pup metrics query --query 'avg:single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write{category:file_activity,variant:comparison,experiment:quality_gate_security_base}.as_rate()' --from 7d --to now

# security_idle — open-only (background noise floor)
pup metrics query --query 'avg:single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write{event_type:open,variant:comparison,experiment:quality_gate_security_idle}.as_rate()' --from 7d --to now

# security_idle — all file activity (background noise floor)
pup metrics query --query 'avg:single_machine_performance.regression_detector.capture.datadog.runtime_security.perf_buffer.events.write{category:file_activity,variant:comparison,experiment:quality_gate_security_idle}.as_rate()' --from 7d --to now
```

The `.as_rate()` modifier guarantees the result is in events/sec. Both this metric and the production metric are type `rate` (statsd_interval=10), so `.as_rate()` is a no-op today — but it documents intent and protects against metadata changes.

SMP runs are intermittent (a few runs per week). Extract the timestamps from the pointlist to identify run windows. If no data is returned, widen to `--from 14d` or `--from 30d`.

**Fallback**: If the `security_idle` experiment has not yet run (no data returned), proceed with the existing 3-way comparison (lading vs security_base vs production) and note that idle baseline data is pending. The noise model in Step 7 falls back to the inferred approach.

## Step 4: Query production metric scoped to SMP run windows

For each SMP run window found in Step 3, query production data over the matching time range. Use the SMP pointlist timestamps to determine `--from` and `--to`. Run all four queries in parallel:

```bash
# Primary — open-only, window-matched
pup metrics query --query 'avg:datadog.runtime_security.perf_buffer.events.write{event_type:open}.as_rate()' --from <smp_window_start> --to <smp_window_end>

# Context — all file activity, window-matched
pup metrics query --query 'avg:datadog.runtime_security.perf_buffer.events.write{category:file_activity}.as_rate()' --from <smp_window_start> --to <smp_window_end>

# Primary — open-only, 24h baseline
pup metrics query --query 'avg:datadog.runtime_security.perf_buffer.events.write{event_type:open}.as_rate()' --from 1d --to now

# Context — all file activity, 24h baseline
pup metrics query --query 'avg:datadog.runtime_security.perf_buffer.events.write{category:file_activity}.as_rate()' --from 1d --to now
```

If SMP data spans a short window (e.g. a single hourly point), expand the production query slightly (±30min) to get enough data points for comparison.

All queries use `.as_rate()` so every value in the comparison chain is in the same unit (events/sec):
- Lading config: `open_per_second` × 1 = events/sec
- SMP captured: `.as_rate()` → events/sec
- Production: `.as_rate()` → events/sec

Compute mean/min/max from both the window-matched and 24h pointlist values.

## Step 5: Compare and analyze

Perform a comparison using **time-aligned** data. The primary comparison uses `event_type:open` values only.

All values must be in **events/sec** (guaranteed by `.as_rate()` on metric queries, and by the explain-lading-config breakdown for lading).

**Primary table — open events/sec:**

| Source | Value (events/sec) | Description |
|--------|---------------------|-------------|
| Lading config | from Step 1 | `open_per_second` × 1 |
| SMP — security_idle (open) | from Step 3 | Background open noise with CWS, no generator |
| SMP — security_base (open) | from Step 3 | Open rate with generator running |
| Generator contribution | computed | security_base - security_idle |
| Org2 per-host avg (open, SMP window) | from Step 4 | production `event_type:open` `.as_rate()` during the same minutes SMP was running |
| Org2 per-host avg (open, 24h) | from Step 4 | broader context showing diurnal range |

**Supplementary context** (not used for tuning decisions):

| Source | Value (events/sec) | Description |
|--------|---------------------|-------------|
| SMP — security_idle (all file activity) | from Step 3 | idle `category:file_activity` — background noise floor |
| SMP — security_base (all file activity) | from Step 3 | loaded `category:file_activity` |
| Org2 per-host avg (all file activity, SMP window) | from Step 4 | production `category:file_activity` |

Analysis:
- **Idle baseline check**: The `security_idle` open rate captures background noise from always-on VFS hooks. Record this value — it is the noise floor subtracted in the tuning model.
- **Generator contribution check**: `generator_contribution = security_base_open - security_idle_open` should approximately equal `open_per_second` (the lading-configured rate). If they diverge, the generator is not behaving as configured.
- **Production validity check**: Compare the SMP `security_base` open rate against the org2 per-host open average *during the same time window*. If they diverge, the experiment is not measuring what org2 sees.
- Compare the lading effective open rate (`open_per_second`) against both the window-matched org2 open rate and the 24h context
- The 24h query is supplementary context (diurnal range), not the modeling target — the org2 per-host open average is the primary comparison
- If the org2 open rate differs from the SMP security_base open rate, flag the gap and suggest specific `open_per_second` changes
- Note the `category:file_activity` totals for reference

## Step 6: Output report

Print a markdown report answering "does the lading config reflect the production open workload?" with these sections:

1. **Lading config** — configured open rate (`open_per_second` × 1 = events/sec)
2. **SMP Idle Baseline (security_idle)** — open rate and file activity with CWS but no generator, with timestamps of each run window
3. **SMP Loaded (security_base)** — open rate and file activity with CWS and generator, with timestamps of each run window
4. **Generator Contribution** — delta between security_base and security_idle for open events (this isolates the generator's effect)
5. **Org2 per-host avg (open, SMP window)** — org2 per-host open rate during the same time windows SMP ran
6. **Org2 per-host avg (open, 24h)** — supplementary context (mean, median, min, max, diurnal pattern)

Followed by a supplementary note showing `category:file_activity` totals from both SMP experiments and production for reference.

Followed by a one-line assessment: match, mismatch, or insufficient data. If security_idle data is not yet available, note this and fall back to the 3-way comparison.

## Step 7: Propose lading config changes

If a mismatch was found, propose concrete changes to `test/regression/cases/quality_gate_security_base/lading/lading.yaml`:

1. **Select the target** — use the **production open mean during the SMP run window** (Step 4, window-matched `event_type:open` query) as the target. This is the open rate production experiences during the hours the SMP experiment actually executes. Do not use the 24h mean, median, or overnight trough — those mix in time periods the experiment never runs during.

2. **Compute target rates using the additive noise model** — all values are in events/sec (from `.as_rate()` queries and the lading config breakdown). Background open noise is now directly measured from the `security_idle` experiment rather than inferred:
   ```
   background_open_noise (events/sec) = security_idle_open_mean   # directly measured
   new_lading_open_rate (events/sec)  = production_open_target - background_open_noise
   new_open_per_second                = new_lading_open_rate
   ```
   **Fallback** (if security_idle data is not yet available): infer background noise as `smp_open_mean - current_open_per_second`.

   Do not use a multiplicative model (config × amplification_factor) — the amplification ratio conflates lading-generated events with background noise and does not hold when the config changes. Do not ignore background noise and set lading rate equal to the production target directly.

3. **Show the diff** — print the exact YAML change (old value → new value) for `open_per_second`. Do not adjust `rename_per_second` — rename events are not the primary signal.
4. **Offer to apply** — ask whether to edit the lading config file directly.

If data was insufficient (e.g. no SMP points), offer to re-query with a wider time range before proposing changes.

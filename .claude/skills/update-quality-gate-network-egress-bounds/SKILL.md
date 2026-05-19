---
name: update-quality-gate-network-egress-bounds
description: Compute and apply a per-replicate `total_bytes_received` bound to a quality_gate SMP experiment based on the max observed across recent nightly runs
user_invocable: true
allowed-tools: Bash, Read, Edit, Skill
---

# update-quality-gate-network-egress-bounds

Set (or re-tune) the per-replicate network-egress quality gate for a `quality_gate_*` SMP experiment. The bound catches regressions that increase how much data the agent sends per replicate without necessarily changing the optimization-goal metric (memory, CPU).

The bound is applied per replicate, *within each job* — SMP runs ~1000 replicates per nightly job, and every single replicate must stay under `upper_bound` or the gate fails. This is stricter than a job-mean bound and is appropriate because nightly variance is tight (per-replicate values typically fall within ~3% of the mean for these experiments).

The bound shape is: `max-observed × 1.10`, rounded up to a clean binary unit. The 10% headroom is chosen so noise alone cannot trip the gate, but a real regression of more than ~10% in bytes-per-replicate fails fast — usually on the first nightly after merge.

## When to use this skill

- Adding a `total_bytes_received` check to a `quality_gate_*` experiment for the first time
- Re-tuning an existing `total_bytes_received` bound because the workload changed intentionally (e.g., new generator settings, agent feature flipped on) and the prior bound is now either over-tight or far too slack
- **Not** for fixing a flaky bound. If the bound is tripping intermittently on unchanged code, the right answer is usually to investigate the regression, not to widen the bound.

## Step 1: Identify the target experiment(s)

The skill applies to any `test/regression/cases/quality_gate_*` experiment. The four originally bounded are `quality_gate_idle`, `quality_gate_idle_all_features`, `quality_gate_logs`, `quality_gate_metrics_logs`. Other quality gates (e.g. `quality_gate_security_*`) follow the same pattern.

## Step 2: Verify pup auth

```bash
pup auth status
```

If expired, `pup auth login`. Datadog MCP tools are an alternative for read-only metric queries.

## Step 3: Query `total_bytes_received` per (job_id, replicate)

For each experiment, run:

```bash
pup --read-only metrics query --query \
  'sum:single_machine_performance.regression_detector.capture.total_bytes_received{purpose:quality_gates,variant:comparison,experiment:<EXP>} by {job_id,replicate}.as_count().rollup(sum, daily)' \
  --from 14d --to now > "/tmp/qg_egress/<EXP>.json"
```

Key tags:

- **`purpose:quality_gates`** — restricts to nightly quality_gate runs (the runs that gate `main`). PR-triggered SMP runs use a different `purpose` and would skew the baseline.
- **`variant:comparison`** — the actual experiment arm, not the reference baseline arm SMP also runs internally.
- **`by {job_id,replicate}`** — groups so each `(job_id, replicate)` becomes one series. The bound is enforced per replicate, so this is the unit of analysis.
- **`.as_count().rollup(sum, daily)`** — the metric reports a running total; `.as_count()` converts to a per-interval delta and `.rollup(sum, daily)` sums those deltas across each daily bucket. With a 14d window, the API returns daily buckets and each (job_id, replicate) ends up with exactly one non-null bucket — the day the replicate ran.

Run the four queries in parallel via `&` + `wait`; each returns ~15 MB of JSON.

## Step 4: Verify the data shape

For each experiment, confirm:

- ~9 unique `job_id`s in 14d (one nightly per day, minus failed/skipped runs)
- ~1000 unique `replicate`s
- Each (job_id, replicate) series has **exactly 1** non-null daily point — anything else means the experiment straddled a day boundary or rolled up oddly, and your sum-per-series is hiding two distinct runs.

Sanity script:

```python
from collections import Counter
import json
data = json.loads(open(f"/tmp/qg_egress/{exp}.json").read())
series = data["data"]["series"]
nonnull = Counter()
for s in series:
    nn = sum(1 for p in s["pointlist"] if p[1] is not None)
    nonnull[nn] += 1
# Expect: {1: ~9000}
print(nonnull)
```

If you see anything other than `{1: N}`, stop and investigate before computing the threshold.

## Step 5: Compute max × 1.10 per experiment

For each series, sum the non-null pointlist values to get `total_bytes_received` for that replicate-run. Take the MAX across all series. Multiply by 1.10. Round UP to a clean binary unit.

```python
import json
from pathlib import Path

data = json.loads(Path(f"/tmp/qg_egress/{exp}.json").read_text())
per_replicate = []
for s in data["data"]["series"]:
    tags = dict(t.split(":", 1) for t in s["scope"].split(",") if ":" in t)
    vals = [p[1] for p in s["pointlist"] if p[1] is not None]
    if vals:
        per_replicate.append((sum(vals), tags["job_id"], tags["replicate"]))
per_replicate.sort(reverse=True)
max_val, max_job, max_rep = per_replicate[0]
threshold = max_val * 1.10
print(f"  max:        {max_val:,d} bytes (job_id={max_job} replicate={max_rep})")
print(f"  max * 1.10: {threshold:,.0f} bytes ({threshold/1024/1024:.2f} MiB)")
```

Also print the **mean** and **min** across replicates as a sanity check. If `(max - min) / mean > ~5%`, the variance is wider than expected for a quality_gate; consider whether the workload is actually deterministic. Wide variance does not invalidate the methodology, but it means the 10% headroom is closer to noise floor than you'd like.

## Step 6: Choose `upper_bound` value and units

Round the `max × 1.10` value up to the smallest clean binary unit. The bound checker accepts:

- `"<n> MiB"` (quoted) or `<n>MiB` (unquoted)
- `<n>KiB`, `<n.n>GiB`, fractional values like `1.25 MiB`
- Plain numeric integers (interpreted as raw bytes)

Match the surrounding style in the file. Where `memory_usage` already uses quoted `"<n> MiB"`, mirror that quoting.

Examples used when this skill was authored (14d window, max across 9 jobs × 1000 replicates):

| Experiment | Max per-replicate | `max × 1.10` | Bound applied |
|---|---|---|---|
| `quality_gate_idle` | 0.72 MiB | 0.79 MiB | `"0.8 MiB"` |
| `quality_gate_idle_all_features` | 1.13 MiB | 1.24 MiB | `"1.25 MiB"` |
| `quality_gate_logs` | 264.89 MiB | 291.38 MiB | `"292 MiB"` |
| `quality_gate_metrics_logs` | 967.73 MiB | 1064.51 MiB | `"1065 MiB"` |

Round close to 10% — going to ~30%+ over (e.g. `1 MiB` instead of `0.8 MiB`) defeats the point of the gate.

## Step 7: Add the bounds check

Insert at the end of the `checks:` list, before `report_links:`. The series name is the suffix after `single_machine_performance.regression_detector.capture.` — i.e. just `total_bytes_received`. This mirrors the convention used by `total_pss_bytes`, `missed_bytes`, `total_cpu_usage_millicores`.

```yaml
  - name: total_bytes_received
    description: "Bytes received quality gate. This bounds total network egress."
    bounds:
      series: total_bytes_received
      upper_bound: "<computed> MiB"
```

Keep the description terse — the value above is the established convention.

## Step 8: Commit

```bash
git add test/regression/cases/quality_gate_*/experiment.yaml
git commit -m "Update per-replicate network egress bound for quality_gate_<exp>"
```

If multiple experiments were updated together, list the new bounds in the commit body.

## Anti-patterns

**Do not use mean or p95 as the baseline.** Nightly quality_gate variance is tight (<5% between min and max per replicate). Using mean × 1.10 will trip on the next slightly-warm replicate. Using max × 1.10 guarantees every historical replicate clears the bound.

**Do not omit `purpose:quality_gates`.** PR-branch SMP runs use a different `purpose` and run under unrelated workload conditions. Including them in the baseline contaminates the threshold.

**Do not group only by `job_id`.** That gives the per-job sum (which is ~1000× higher than per-replicate) and the bound checker enforces per-replicate, so you'd be sizing the wrong quantity. The query must group by both `job_id` and `replicate`.

**Do not use a wide rollup or a long-window mean.** A 14-day window with `.rollup(sum, daily)` gives one bucket per replicate-run, which is what we want. Switching to `.rollup(avg, ...)` over coarser intervals would muddle individual replicate values.

**Do not round up to the next round number "for cleanliness".** If `max × 1.10` is 0.79 MiB, the bound is `0.8 MiB`, not `1 MiB`. The 10% headroom is the spec; rounding multiples that headroom is silent slack.

**Do not bound a non-`quality_gate_*` experiment with this methodology.** Non-quality-gate experiments often have erratic workloads where per-replicate variance is high; the max-times-1.10 approach there yields a bound that catches nothing.

## Output report (recommended)

When proposing bounds to a reviewer, include this table per experiment so they can audit the arithmetic:

```
=== <experiment> ===
  unique (job_id, replicate) series in 14d window: <N>
  MAX per-replicate:  <bytes>  (<MiB>)
    job_id=<UUID> replicate=<R>
  MIN per-replicate:  <bytes>  (<MiB>)
  MEAN per-replicate: <bytes>  (<MiB>)
  Top 5 replicates:
    <bytes>  job_id=<UUID> replicate=<R>
    ...
  Proposed bound: <value>  (= max × 1.10, rounded up)
  Headroom over max: <pct>%
```

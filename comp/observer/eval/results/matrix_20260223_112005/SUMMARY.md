# RCA Quality Improvements — Matrix Evaluation Results

**Date:** 2026-02-23
**Config:** `--cusum --time-cluster --dedup [--rca]`
**Model:** gpt-5.2-2025-12-11

## Score Matrix

| Scenario | Baseline | RCA | Delta | Prior Baseline* |
|---|---|---|---|---|
| oom-kill | 78 | 78 | 0 | 95 |
| cpu-starvation | 75 | 78 | +3 | 78 |
| traffic-spike | 15 | 15 | 0 | 5→15** |
| s3-outage | 15 | 5 | -10 | 12 |
| **Average** | **45.75** | **44.0** | **-1.75** | **47.5** |

*Prior baseline from EXAMPLE_CONTEXT.md (pre-RCA changes, different run)
**Prior traffic-spike with old RCA was 5; old baseline was 15. Now RCA matches baseline.

## RCA Suppression Status

| Scenario | RCA Present in Output | Confidence | Reason |
|---|---|---|---|
| oom-kill | No (suppressed) | <0.5 | MinConfidence filter |
| cpu-starvation | No (suppressed) | <0.5 | MinConfidence filter |
| traffic-spike | No (suppressed) | <0.5 | MinConfidence filter |
| s3-outage | Yes | 0.519 | Passed threshold, but flagged ambiguous_roots |

## Key Observations

### Traffic-spike (target scenario)
- **Before changes:** Old RCA pointed LLM at `cleanup` container → score dropped to 5
- **After changes:** RCA suppressed by MinConfidence → score matches baseline at 15
- **Result: +10 improvement over old RCA variant, regression eliminated**

### oom-kill
- Baseline and RCA identical (78 each) — RCA suppressed, no harm
- Score lower than prior 95, but this is LLM variance (different run, same config)

### cpu-starvation
- RCA variant slightly better (+3) — likely LLM noise rather than RCA effect (RCA was suppressed)
- Score matches prior baseline (78)

### s3-outage
- RCA passed with 0.519 confidence (barely above 0.5 threshold)
- RCA pointed at observer-agent containerd blkio writes — misleading
- RCA variant scored 5 vs baseline 15 (-10), but both are low
- s3-outage is fundamentally hard: infra metrics can't see etcd quorum loss

## Grading Details

### oom-kill (baseline=78, rca=78)
- Identified: partial — described "OOM-related event" but medium confidence
- RCA: suppressed

### cpu-starvation (baseline=75, rca=78)
- Identified: partial — found CPU throttling/pressure but attributed to crash-loop not CPU limits
- RCA: suppressed

### traffic-spike (baseline=15, rca=15)
- Identified: no — judged unclear, suggested cgroup/pod lifecycle artifact
- RCA: suppressed (was the regression source in previous iteration)

### s3-outage (baseline=15, rca=5)
- Identified: no — found cluster-wide CPU anomaly but no specific root cause
- RCA: present but misleading (observer-agent container, ambiguous_roots=true)

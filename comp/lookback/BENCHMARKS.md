# Lookback Component — Benchmark Results

All benchmarks run on `stormeagle.us1.staging.dog` (16-node staging cluster)
using the testbench at `agent-q-branch-test-bench/docker/bench`.

Workload: `dogstatsd-p95` — 800 KiB/s DogStatsD traffic, 25 K contexts,
~8 K samples/s, ~1 700 flushed series/s.
Each configuration: **2 runs × 90 s** with 15 s warmup.
Measurement: cgroup `memory.current` sampled every second; values are mean ± stddev across runs.

---

## 1. Hook-only overhead (check runner disabled)

The baseline measures the cost of subscribing to the metrics pipeline hook,
writing 24-byte WAL records, and maintaining the bloom-filtered context store.
No independent check runner.

| Metric | Baseline | Lookback ON | Delta | Delta % |
|---|---|---|---|---|
| Agent RSS mean | 132.2 ± 0.4 MB | 133.0 ± 1.5 MB | **+0.8 ± 1.5 MB** | +0.6% |
| Agent RSS P95 | 140.2 ± 1.4 MB | 141.2 ± 0.9 MB | +1.0 ± 1.7 MB | +0.7% |
| Agent anon RSS mean | 56.3 ± 1.0 MB | 56.8 ± 1.0 MB | +0.6 ± 1.4 MB | +1.0% |
| Agent CPU mean | 6 428 ± 282 mc | 6 362 ± 49 mc | −66 ± 286 mc | −1.0% |
| Agent CPU max | 678 785 mc | 671 824 mc | −6 961 mc | −1.0% |

**WAL write rate:** 82 records/s · 1.9 KB/s → 6.8 MB/h projected  
**WAL records / 90 s window:** 7 373

---

## 2. Check runner @ 5 s interval (cpu + memory + load)

The check runner runs `cpu`, `memory`, and `load` checks every 5 s via the
standard Runner + Scheduler, using a dedicated `walSender` that bypasses the
agent's aggregator entirely. This is 3× more frequent than the normal 15 s
check interval.

| Metric | Baseline | Lookback ON | Delta | Delta % |
|---|---|---|---|---|
| Agent RSS mean | 129.3 ± 0.9 MB | 132.3 ± 1.6 MB | **+2.9 ± 1.8 MB** | +2.3% |
| Agent RSS P50 | 134.4 ± 0.4 MB | 137.2 ± 2.4 MB | +2.8 ± 2.4 MB | +2.1% |
| Agent RSS P95 | 136.1 ± 0.6 MB | 138.7 ± 2.7 MB | +2.6 ± 2.8 MB | +1.9% |
| Agent anon RSS mean | 53.6 ± 0.5 MB | 56.3 ± 1.5 MB | +2.7 ± 1.6 MB | +5.1% |
| Agent CPU mean | 6 060 ± 115 mc | 6 326 ± 90 mc | **+266 ± 146 mc** | +4.4% |
| Agent CPU max | 639 873 mc | 667 923 mc | +28 050 mc | +4.4% |

**WAL write rate:** 82 records/s · 1.9 KB/s → 6.8 MB/h projected  
**WAL records / 90 s window:** 7 373

> The WAL rate is identical to hook-only because at 5 s the check runner
> contributes ~3 metrics × 3 checks = 9 new context keys per interval, which
> is negligible compared to the 25 K DSD contexts flowing through the hook.

---

## 3. Check runner @ 100 ms interval (cpu + memory + load)

The 100 ms path uses a direct `time.NewTicker` goroutine per check (the
standard scheduler enforces a 1 s minimum). This is 150× more frequent than
the default 15 s check interval and 50× more frequent than the 5 s
configuration.

| Metric | Baseline | Lookback ON | Delta | Delta % |
|---|---|---|---|---|
| Agent RSS mean | 130.2 ± 0.4 MB | 134.1 ± 0.3 MB | **+3.8 ± 0.5 MB** | +2.9% |
| Agent RSS P50 | 134.3 ± 2.3 MB | 136.2 ± 0.2 MB | +1.9 ± 2.4 MB | +1.4% |
| Agent RSS P95 | 136.7 ± 1.4 MB | 138.6 ± 0.5 MB | +2.0 ± 1.4 MB | +1.4% |
| Agent RSS max | 137.3 ± 1.7 MB | 141.4 ± 3.5 MB | +4.1 ± 3.9 MB | +3.0% |
| Agent anon RSS mean | 53.5 ± 0.5 MB | 57.2 ± 0.4 MB | +3.7 ± 0.6 MB | +7.0% |
| Agent CPU mean | 6 326 ± 156 mc | 6 897 ± 94 mc | **+571 ± 182 mc** | +9.0% |
| Agent CPU P50 | 8 ± 0 mc | 34 ± 0 mc | +26 mc | +325% |
| Agent CPU max | 668 012 mc | 725 676 mc | +57 665 mc | +8.6% |

**WAL write rate:** 687 records/s · 16.1 KB/s → 56.6 MB/h projected  
**WAL records / 90 s window:** 61 829  
**Projected disk usage:** 1.3 GB/day

> The CPU P50 jump (+26 mc, +325%) reflects the constant ticker firing every
> 100 ms. Mean overhead (+9%) stays proportional to the 50× frequency
> increase because the checks themselves are lightweight.

---

## Summary

| Configuration | RSS Δ (mean) | CPU Δ (mean) | WAL rate | Disk/day |
|---|---|---|---|---|
| Hook only | **+0.8 MB** (+0.6%) | −1.0% (noise) | 82/s | ~0.2 GB |
| Check runner @ 5 s | **+2.9 MB** (+2.3%) | +266 mc (+4.4%) | 82/s | ~0.2 GB |
| Check runner @ 100 ms | **+3.8 MB** (+2.9%) | +571 mc (+9.0%) | 687/s | **~1.3 GB** |

### Notes

- All deltas are computed on the full lookback stack (WAL + context store +
  bloom filter + check runner where applicable).
- RSS includes the bloom filter fixed allocation (~1.2 MB), WAL write buffers
  (16 shards × 64 KB = 1 MB), and walSender sample buffers.
- The `anon RSS` delta (heap only) is a more reliable signal than total RSS
  because it excludes file-backed pages shared with the OS page cache.
- At 100 ms, disk usage of 1.3 GB/day is only practical for short-duration
  captures. For continuous monitoring, 5 s (0.2 GB/day) is the recommended
  default.
- CPU P95 figures are noisy at 2-run averages; mean CPU is more stable and
  the primary overhead metric.

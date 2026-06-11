# DogStatsD stats feature-cost SMP

This note covers the `dogstatsd-stats` feature-cost evidence. The strongest
result is the paired run where `origin/main` keeps DogStatsD metric stats
disabled while the experimental Stage W stack keeps `dogstatsd-stats` enabled.
That answers the product-shaped question: can we do more local observability
work and still use less Agent CPU than main with the feature off?

Short answer for these UDS/repeated-tagset workloads: yes.

## Strongest result: experiment stats on vs main stats off

### Images

- Baseline: `datadog/agent-dev:smp-dsd-main`
  - image ID `sha256:6f67e85689c833453bb60ba0697d234561a39a9f8df37d36f2a7fd6372316419`
  - Agent commit `3ec880f14a3`
  - `dogstatsd-stats` disabled
- Comparison: `datadog/agent-dev:smp-dsd-columnar-v3-fastlane-stats-shared-enabled-stats-on`
  - image ID `sha256:ba144c74d1b379c4350fa2622def353cc670f616c60d1e8197f413566f3d0039`
  - Agent commit `b6db1d81aa1`
  - derived from `datadog/agent-dev:smp-dsd-columnar-v3-fastlane-stats-shared-enabled`
  - baked env includes:
    - `DD_DOGSTATSD_METRICS_STATS_ENABLE=true`
    - `DD_DOGSTATSD_LOGGING_ENABLED=false`
    - Stage W exact-tagset-cache / compact-identity / direct-parse / fast-lane gates

Experiment directory:

- `reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-stageQ-native-serializer`

The stats enablement is baked only into the comparison image so the baseline main
image remains stats-disabled in the same paired SMP run.

### Standard compact-batch workload

Case: `uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch`

Command:

```bash
smp local run \
  --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-stageQ-native-serializer \
  --case uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch \
  --baseline-image datadog/agent-dev:smp-dsd-main \
  --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-fastlane-stats-shared-enabled-stats-on \
  --replicates 3 \
  --total-samples 270
```

SMP result:

| Metric | Result |
|---|---:|
| Δ mean | `+1.79%` |
| Δ mean CI | `[+1.56%, +2.02%]` |
| Confidence | `100%` |

Selected metrics:

| Metric | Main, stats off | Experiment, stats on | Δ |
|---|---:|---:|---:|
| generator written MiB/s | `97.40` | `99.02` | `+1.67%` |
| DogStatsD UDS bytes MiB/s | `97.07` | `98.68` | `+1.66%` |
| processed metric samples/s | `111,890` | `113,746` | `+1.66%` |
| Agent CPU cores | `0.627` | `0.368` | `-41.3%` |
| RSS MiB | `457.7` | `422.6` | `-7.7%` |
| Go heap alloc MiB | `224.9` | `213.1` | `-5.2%` |
| MetricSample pool gets/s | `3907.0` | `0.17` | `-99.996%` |
| Experiment retained debug contexts | n/a | `2092` | n/a |

Artifacts:

- log: `reports/smp/dogstatsd-agg-serde-20260516-143205/cases/branch-stats-on-vs-main-off-standard.log`
- captures: `reports/smp/dogstatsd-agg-serde-20260516-143205/captures/branch-stats-on-vs-main-off-standard/`

### High-rate metrics-only workload

Case: `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only_compact_batch`

Command:

```bash
smp local run \
  --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-stageQ-native-serializer \
  --case uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only_compact_batch \
  --baseline-image datadog/agent-dev:smp-dsd-main \
  --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-fastlane-stats-shared-enabled-stats-on \
  --replicates 3 \
  --total-samples 270
```

SMP result:

| Metric | Result |
|---|---:|
| Δ mean | `+25.83%` |
| Δ mean CI | `[+25.55%, +26.11%]` |
| Confidence | `100%` |

Selected metrics:

| Metric | Main, stats off | Experiment, stats on | Δ |
|---|---:|---:|---:|
| generator written MiB/s | `196.88` | `247.70` | `+25.8%` |
| DogStatsD UDS bytes MiB/s | `196.26` | `246.87` | `+25.8%` |
| processed metric samples/s | `242,563` | `305,120` | `+25.8%` |
| Agent CPU cores | `0.977` | `0.560` | `-42.7%` |
| RSS MiB | `178.7` | `230.5` | `+29.0%` |
| Go heap alloc MiB | `76.0` | `101.4` | `+33.4%` |
| MetricSample pool gets/s | `8385.7` | `0.17` | `-99.998%` |
| Experiment retained debug contexts | n/a | `2094` | n/a |

Read: this is the clearest “doing more with much less” result. The experimental
Agent keeps `dogstatsd-stats` enabled, processes substantially more DogStatsD
traffic than stats-disabled main, and uses about 43% less Agent CPU. Memory is
not universally better: in this high-rate stats-on-vs-stats-off comparison, the
experiment retains more RSS/heap because the new dictionaries/descriptors/view
state are live.

Artifacts:

- log: `reports/smp/dogstatsd-agg-serde-20260516-143205/cases/branch-stats-on-vs-main-off-high.log`
- captures: `reports/smp/dogstatsd-agg-serde-20260516-143205/captures/branch-stats-on-vs-main-off-high/`
- selected metrics: `reports/smp/dogstatsd-agg-serde-20260516-143205/dogstatsd_stats_on_vs_main_stats_off_selected_metrics.csv`

## Secondary result: stats enabled on both sides

This older run compares `origin/main` against the current experimental Stage W
stack with `dogstatsd-stats` enabled on both sides. It answers a different
question: if a user turns on DogStatsD metric stats, how does main compare to the
shared-identity/direct/fast-lane prototype?

### Images

- Baseline: `datadog/agent-dev:smp-dsd-main`
  - image ID `sha256:6f67e85689c833453bb60ba0697d234561a39a9f8df37d36f2a7fd6372316419`
  - Agent commit `3ec880f14a3`
- Comparison: `datadog/agent-dev:smp-dsd-columnar-v3-fastlane-stats-shared-enabled`
  - image ID `sha256:6e035a7d3cab03f30fc0d4b1bee4375bb9aacecfbe93f1ce0850830464c40e1c`
  - Agent commit `b6db1d81aa1`

Experiment directory:

- `reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-dogstatsd-stats-debug-on-stageW`

Important environment:

- `DD_DOGSTATSD_METRICS_STATS_ENABLE=true`
- `DD_DOGSTATSD_LOGGING_ENABLED=false`
- `DD_SERIALIZER_EXPERIMENTAL_USE_V3_API_SERIES_ENDPOINTS='["http://127.0.0.1:9091"]'`
- Stage W gates for the comparison path:
  - `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true`
  - `DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES=true`
  - `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_DIRECT_PARSE=true`
  - `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_COMPACT_ROWS_OMIT_DESCRIPTOR=true`
  - `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_FASTLANE=true`

Per-metric DogStatsD stats logging is disabled intentionally. This isolates the
stats materialized-view cost instead of measuring log I/O.

### Standard compact-batch workload

| Metric | Main + stats | Experiment + stats | Δ |
|---|---:|---:|---:|
| SMP Δ mean | n/a | `+2.23%` | CI `[+1.98%, +2.47%]` |
| generator written MiB/s | `97.09` | `99.11` | `+2.09%` |
| DogStatsD UDS bytes MiB/s | `96.75` | `98.79` | `+2.10%` |
| Agent CPU cores | `0.844` | `0.366` | `-56.7%` |
| RSS MiB | `634.3` | `423.4` | `-33.2%` |
| Go heap alloc MiB | `360.9` | `211.6` | `-41.4%` |
| MetricSample pool gets/s | `3885.8` | `0.17` | `-99.996%` |

Artifacts:

- log: `reports/smp/dogstatsd-agg-serde-20260516-143205/cases/dogstatsd-stats-debug-on-standard-main-vs-fastlane.log`
- selected metrics: `reports/smp/dogstatsd-agg-serde-20260516-143205/dogstatsd_stats_debug_on_standard_selected_metrics.csv`
- captures: `reports/smp/dogstatsd-agg-serde-20260516-143205/captures/dogstatsd-stats-debug-on-standard/`

### High-rate metrics-only workload

| Metric | Main + stats | Experiment + stats | Δ |
|---|---:|---:|---:|
| SMP Δ mean | n/a | `+62.58%` | CI `[+62.30%, +62.87%]` |
| generator written MiB/s | `152.82` | `247.55` | `+62.0%` |
| DogStatsD UDS bytes MiB/s | `152.38` | `246.73` | `+61.9%` |
| processed metric samples/s | `187,948` | `304,946` | `+62.2%` |
| Agent CPU cores | `0.987` | `0.553` | `-43.9%` |
| RSS MiB | `619.5` | `230.1` | `-62.9%` |
| Go heap alloc MiB | `394.8` | `102.6` | `-74.0%` |
| MetricSample pool gets/s | `6504.3` | `0.17` | `-99.997%` |

Read: with debug stats enabled on both sides, main could not sustain the
requested 250 MiB/s fixed-rate workload on this local SMP setup. The
experimental path stayed near the requested rate while using less Agent CPU and
less live memory.

Artifacts:

- log: `reports/smp/dogstatsd-agg-serde-20260516-143205/cases/dogstatsd-stats-debug-on-high-main-vs-fastlane.log`
- selected metrics: `reports/smp/dogstatsd-agg-serde-20260516-143205/dogstatsd_stats_debug_on_high_selected_metrics.csv`
- captures: `reports/smp/dogstatsd-agg-serde-20260516-143205/captures/dogstatsd-stats-debug-on-high/`

## Caveats

- These comparisons run the whole experimental stack; they do not isolate only
  the stats-store rewrite.
- The comparison path uses opt-in experimental gates and remains fallback-safe.
- The result is for UDS, repeated tagsets, and metrics-heavy workloads.
- `dogstatsd_logging_enabled=false`; enabling per-metric logging would measure
  log I/O, not just stats maintenance.
- The high-rate results are local fixed-rate/backpressure results, not yet full
  max-throughput sweeps.
- Memory is mixed versus stats-disabled main: standard memory was lower, but the
  high-rate run retained more RSS/heap.

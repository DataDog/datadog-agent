# DogStatsD stats feature-cost SMP

This run compares `origin/main` against the current experimental Stage W stack with
`dogstatsd-stats` enabled on both sides. The goal is not to isolate every stage;
it answers the product-shaped question: if a user turns on DogStatsD metric stats,
how does main compare to the shared-identity/direct/fast-lane prototype?

## Images

- Baseline: `datadog/agent-dev:smp-dsd-main`
  - image ID `sha256:6f67e85689c833453bb60ba0697d234561a39a9f8df37d36f2a7fd6372316419`
  - Agent commit `3ec880f14a3`
- Comparison: `datadog/agent-dev:smp-dsd-columnar-v3-fastlane-stats-shared-enabled`
  - image ID `sha256:6e035a7d3cab03f30fc0d4b1bee4375bb9aacecfbe93f1ce0850830464c40e1c`
  - Agent commit `b6db1d81aa1`

## Shared configuration

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

## Standard compact-batch workload

Case: `uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch`

Command:

```bash
smp local run \
  --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-dogstatsd-stats-debug-on-stageW \
  --case uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch \
  --baseline-image datadog/agent-dev:smp-dsd-main \
  --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-fastlane-stats-shared-enabled \
  --replicates 3 \
  --total-samples 270
```

SMP result:

| Metric | Result |
|---|---:|
| Δ mean | `+2.23%` |
| Δ mean CI | `[+1.98%, +2.47%]` |
| Confidence | `100%` |

Selected metrics:

| Metric | Main + stats | Experiment + stats | Δ |
|---|---:|---:|---:|
| generator written MiB/s | `97.09` | `99.11` | `+2.09%` |
| DogStatsD UDS bytes MiB/s | `96.75` | `98.79` | `+2.10%` |
| Agent CPU cores | `0.844` | `0.366` | `-56.7%` |
| RSS MiB | `634.3` | `423.4` | `-33.2%` |
| Go heap alloc MiB | `360.9` | `211.6` | `-41.4%` |
| MetricSample pool gets/s | `3885.8` | `0.17` | `-99.996%` |
| Experiment retained debug contexts | n/a | `2093` | n/a |

Artifacts:

- log: `reports/smp/dogstatsd-agg-serde-20260516-143205/cases/dogstatsd-stats-debug-on-standard-main-vs-fastlane.log`
- selected metrics: `reports/smp/dogstatsd-agg-serde-20260516-143205/dogstatsd_stats_debug_on_standard_selected_metrics.csv`
- captures: `reports/smp/dogstatsd-agg-serde-20260516-143205/captures/dogstatsd-stats-debug-on-standard/`

## High-rate metrics-only workload

Case: `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only_compact_batch`

Command:

```bash
smp local run \
  --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-dogstatsd-stats-debug-on-stageW \
  --case uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only_compact_batch \
  --baseline-image datadog/agent-dev:smp-dsd-main \
  --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-fastlane-stats-shared-enabled \
  --replicates 3 \
  --total-samples 270
```

SMP result:

| Metric | Result |
|---|---:|
| Δ mean | `+62.58%` |
| Δ mean CI | `[+62.30%, +62.87%]` |
| Confidence | `100%` |

Selected metrics:

| Metric | Main + stats | Experiment + stats | Δ |
|---|---:|---:|---:|
| generator written MiB/s | `152.82` | `247.55` | `+62.0%` |
| DogStatsD UDS bytes MiB/s | `152.38` | `246.73` | `+61.9%` |
| processed metric samples/s | `187,948` | `304,946` | `+62.2%` |
| Agent CPU cores | `0.987` | `0.553` | `-43.9%` |
| RSS MiB | `619.5` | `230.1` | `-62.9%` |
| Go heap alloc MiB | `394.8` | `102.6` | `-74.0%` |
| MetricSample pool gets/s | `6504.3` | `0.17` | `-99.997%` |
| Experiment retained debug contexts | n/a | `2093` | n/a |

Read: with debug stats enabled, main could not sustain the requested 250 MiB/s
fixed-rate workload on this local SMP setup. The experimental path stayed near
the requested rate while using less Agent CPU and less live memory. This is the
clearest macro signal so far that sharing parser-side identity and avoiding
`MetricSample` materialization makes this feature much cheaper under load.

Artifacts:

- log: `reports/smp/dogstatsd-agg-serde-20260516-143205/cases/dogstatsd-stats-debug-on-high-main-vs-fastlane.log`
- selected metrics: `reports/smp/dogstatsd-agg-serde-20260516-143205/dogstatsd_stats_debug_on_high_selected_metrics.csv`
- captures: `reports/smp/dogstatsd-agg-serde-20260516-143205/captures/dogstatsd-stats-debug-on-high/`

## Caveats

- This compares main to the current experimental stack; it does not isolate only
  the stats-store rewrite.
- The comparison path uses opt-in experimental gates and remains fallback-safe.
- The result is for UDS, repeated tagsets, and metrics-heavy workloads.
- `dogstatsd_logging_enabled=false`; enabling per-metric logging would measure
  log I/O, not just stats maintenance.
- The high-rate result is a local fixed-rate/backpressure result, not yet a full
  max-throughput sweep.

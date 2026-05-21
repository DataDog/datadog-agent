# Stage W: parser no-materialization fast lane

Stage W tests whether compact DogStatsD identities can save meaningful per-sample
work before full `MetricSample` materialization. Unlike Stage V, this is a narrow
fast lane for exact raw-tagset cache hits and repeated descriptors.

## Commit and images

Commit: `658b3c805cb0 dogstatsd: add columnar fastlane for compact identities`

Images:

- `datadog/agent-dev:smp-dsd-columnar-v3-fastlane`
  - image ID `sha256:241b655929f4d624bd5e22b1919d993feb7f41eff942399a9b6d4702c9ad3aa8`
- `datadog/agent-dev:smp-dsd-columnar-v3-fastlane-baseline`
  - image ID `sha256:254f98d84a24c29519a8c97f6fd51e1aff949cc684b32c63ce57ea7a74c8c772`
  - env: exact tagset cache + compact identities + direct parse + descriptor omission
- `datadog/agent-dev:smp-dsd-columnar-v3-fastlane-enabled`
  - image ID `sha256:18e6b54087515f1bcc46cad1b379257ace2e517a468a355ab9b7fc00409ee11d`
  - env: baseline plus `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_FASTLANE=true`

Agent version:

```text
Agent 7.81.0-devel - Meta: git.89.658b3c8 - Commit: 658b3c805cb - Serialization version: v5.0.196 - Go version: go1.25.10
```

## Implementation summary

Gate:

- `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_FASTLANE=true`
- `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_FASTLANE_SIZE=<entries>` default `65536`

The fast lane is intentionally narrow:

- supported only when Stage V direct parse is eligible;
- requires exact raw-tagset cache hit with non-zero tagset ID;
- single-value gauge/count only in this slice;
- no mapper, extra tags, hist-to-distribution, timestamp, origin fields, UDS
  origin, or process metadata;
- `dogstatsd-stats` can be updated from the same shared parser-side series
  identity; the experiment no longer treats stats as a separate debug identity;
- fallback-safe: misses and unsupported messages use Stage V/direct legacy path.

On hit, the parser looks up a worker-local descriptor by raw metric name +
tagset ID + metric type. Repeated hits append scalar columnar-v3 rows directly
without materializing tags or a full `MetricSample`.

Columnar compact identity state now also carries a validated descriptor reference
(descriptor ID + generation). Columnar validates the generation/context/type
before inserting by descriptor ref. Descriptor expiry clears the compact state.

## Validation

```bash
dda inv test --targets=./comp/dogstatsd/server/impl,./comp/dogstatsd/internal/identity,./pkg/aggregator,./pkg/metrics --timeout=300
```

Result: passed (`658` package tests; unified report `576` tests).

Focused benchmark:

```bash
dda inv test --targets=./comp/dogstatsd/server/impl --timeout=300 --extra-args='-run=^$ -bench=BenchmarkParseMetricMessageColumnarV3DirectFastLane -benchmem -count=5'
```

Representative focused result:

| Path | ns/op | allocs |
|---|---:|---:|
| Stage V direct materialized hit | `311.8` | `0 B/op`, `0 allocs/op` |
| Stage W fast-lane descriptor hit | `185.2` | `0 B/op`, `0 allocs/op` |

That is a `~40.6%` parser/direct-handoff reduction on the exact cache-hit path.

Follow-up stats-aware focused benchmark with `dogstatsd_logging_enabled=false`:

| Path | ns/op | allocs |
|---|---:|---:|
| Direct materialized hit, stats off | `317.3` | `0 B/op`, `0 allocs/op` |
| Fast-lane descriptor hit, stats off | `192.9` | `0 B/op`, `0 allocs/op` |
| Direct materialized hit, stats on | `452.6` | `0 B/op`, `0 allocs/op` |
| Fast-lane descriptor hit, stats on | `314.2` | `0 B/op`, `0 allocs/op` |

The bounded stats view itself measured `96.14 ns/op`, `0 allocs/op` in the
contention benchmark versus `555.1 ns/op` for the legacy global-lock shape. The
macro feature-cost SMP with `dogstatsd-stats` enabled still needs to be run, but
the focused result shows stats no longer force repeated simple metrics off the
fast lane.

## SMP results

All SMP comparisons use `--replicates 3 --total-samples 270` and compare the
Stage W baseline image against the fast-lane enabled image.

### High-rate metrics-only compact batch

Case: `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only_compact_batch`

Throughput result:

| Metric | Result |
|---|---:|
| Δ mean | `+0.01%` |
| Δ mean CI | `[-0.02%, +0.03%]` |

Selected metrics:

| Metric | Baseline | Fast lane | Δ |
|---|---:|---:|---:|
| agent CPU cores | `0.5789` | `0.4978` | `-14.0%` |
| generator written MiB/s | `247.79` | `248.06` | `+0.11%` |
| DogStatsD UDS bytes MiB/s | `246.97` | `247.23` | `+0.11%` |
| columnar flush duration ns/s | `3.15e6` | `2.29e6` | `-27.3%` |
| string interner hits/s | `71689.7` | `5509.4` | `-92.3%` |
| string interner misses/s | `158.2` | `140.7` | `-11.1%` |

Per-replicate CPU deltas: `-14.2%`, `-14.5%`, `-13.4%`.

### Standard compact batch

Case: `uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch`

Throughput result:

| Metric | Result |
|---|---:|
| Δ mean | `+0.00%` |
| Δ mean CI | `[-0.03%, +0.03%]` |

Selected metrics:

| Metric | Baseline | Fast lane | Δ |
|---|---:|---:|---:|
| agent CPU cores | `0.3637` | `0.3333` | `-8.36%` |
| generator written MiB/s | `99.22` | `99.23` | `+0.00%` |
| DogStatsD UDS bytes MiB/s | `98.89` | `98.90` | `+0.00%` |
| columnar flush duration ns/s | `2.54e6` | `2.26e6` | `-11.3%` |
| string interner hits/s | `26683.3` | `2249.4` | `-91.6%` |
| string interner misses/s | `156.2` | `138.9` | `-11.1%` |

Per-replicate CPU deltas: `-8.4%`, `-9.0%`, `-7.6%`.

### 2 CPU / 240 MiB/s metrics-only sweep case

Case: `uds_dogstatsd_to_api_v3_endpoint_fixed_240mb_metrics_only_2cpu`

Throughput result:

| Metric | Result |
|---|---:|
| Δ mean | `+0.00%` |
| Δ mean CI | `[-0.03%, +0.03%]` |

Selected metrics:

| Metric | Baseline | Fast lane | Δ |
|---|---:|---:|---:|
| agent CPU cores | `0.5611` | `0.4822` | `-14.1%` |
| generator written MiB/s | `237.63` | `237.61` | `-0.01%` |
| DogStatsD UDS bytes MiB/s | `236.83` | `236.81` | `-0.01%` |
| columnar flush duration ns/s | `4.01e6` | `2.60e6` | `-35.1%` |
| string interner hits/s | `68758.7` | `5289.0` | `-92.3%` |
| string interner misses/s | `157.9` | `140.6` | `-11.0%` |

## Read

Stage W is not macro-neutral on resource usage: it consistently saves meaningful
agent CPU (`~8–14%`) and removes most repeated parser string-interner hits on
cache-hit workloads. Throughput remains neutral in these fixed-rate SMP cases
because the generator holds ingress rate constant and the Agent is not saturated
on these local runs. Memory is not yet a win: high-rate RSS/heap rose by about
`+4.8%`/`+11.6%`, while standard and 2-CPU runs were smaller (`~+1%` RSS and
`+0.6–3.2%` heap).

This is the first compact-ID stage with a decisive macro signal. The next
throughput-oriented validation should use a true saturation sweep: increase input
rate or lower available CPU until the Stage W baseline develops lag/drops, then
compare sustainable rate, lag, and CPU. A useful follow-up is adding cheap
fast-lane hit/miss/stale-ref counters so SMP can report hit coverage directly
instead of inferring it from string-interner deltas.

## Artifacts

- `reports/smp/dogstatsd-agg-serde-20260516-143205/stageW_fastlane_high_selected_metrics.csv`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/stageW_fastlane_standard_selected_metrics.csv`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/stageW_fastlane_2cpu_240mb_selected_metrics.csv`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/cases/stageW-fastlane-feature-cost-high.log`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/cases/stageW-fastlane-feature-cost-standard.log`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/cases/stageW-fastlane-feature-cost-2cpu-240mb.log`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/notes/build-stageW-fastlane-linux-container-rerun5.log`

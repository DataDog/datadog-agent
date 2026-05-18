# DogStatsD Aggregator + Serializer SMP Experiment Plan

## Purpose

Use SMP as the source of truth to prove or disprove the claim that the DogStatsD
streaming-foundation work can become a meaningful improvement to core
aggregator + serializer operation.

The claim is intentionally stronger than "debug stats are faster" or "replay is
cleaner":

> DogStatsD can parse, enrich, identify, aggregate, and serialize through a
> payload-aligned semantic stream, reducing repeated tag/context work and
> avoiding unnecessary `metrics.Serie` reconstruction while preserving wire
> semantics and payload size.

This document describes the local SMP workflow we can use before asking for a
full SMP/quality-gate run.

## Branch and image matrix

Keep the reviewable foundation branch separate from invasive experiments.

Recommended branches:

- `dogstatsd-streaming-foundation`: current committed foundation stack.
- `dogstatsd-agg-serde-experiment`: branch from the foundation branch for
  invasive aggregator/serializer prototypes.

Build three local Docker images from the same machine and architecture:

| Image | Branch | Purpose |
|---|---|---|
| `datadog/agent-dev:smp-dsd-main` | `main` or merge-base baseline | Current production baseline |
| `datadog/agent-dev:smp-dsd-foundation` | `dogstatsd-streaming-foundation` | Prove foundation is neutral/safe |
| `datadog/agent-dev:smp-dsd-experiment` | `dogstatsd-agg-serde-experiment` | Prove/disprove core pipeline win |

Example build commands:

```bash
git switch main
dda inv agent.hacky-dev-image-build --target-image datadog/agent-dev:smp-dsd-main --no-development

git switch dogstatsd-streaming-foundation
dda inv agent.hacky-dev-image-build --target-image datadog/agent-dev:smp-dsd-foundation --no-development

git switch -c dogstatsd-agg-serde-experiment
dda inv agent.hacky-dev-image-build --target-image datadog/agent-dev:smp-dsd-experiment --no-development
```

Record the exact commit for each image in the report.

## Local SMP setup

Follow `SMP_LOCAL_BENCHMARK_SKILL.md`.

For Colima on macOS:

```bash
export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"
mkdir -p "$HOME/.tmp-smp"
export TMPDIR="$HOME/.tmp-smp"
```

List available cases:

```bash
find test/regression/cases -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort
```

Smoke-test before full comparisons:

```bash
smp local smoketest \
  --experiment-dir test/regression \
  --case uds_dogstatsd_to_api_v3 \
  --target-image datadog/agent-dev:smp-dsd-foundation \
  --total-samples 60 \
  --follow all
```

## Existing SMP cases to use first

### 1. `uds_dogstatsd_to_api_v3`

Primary core serializer case.

Why it matters:

- UDS DogStatsD ingress.
- High byte rate (`100 MiB/s`).
- Enables `DD_SERIALIZER_EXPERIMENTAL_USE_V3_API_SERIES=true`.
- Directly exercises current v3 serializer dictionary/column path.

Use this as the main case for aggregator + serializer claims.

### 2. `uds_dogstatsd_to_api`

Compatibility/sanity case for the non-v3 serializer path.

Why it matters:

- Shows whether foundation changes regress the established DogStatsD-to-API path.
- Useful as a guardrail, not the main proof for segment/v3 integration.

### 3. `uds_dogstatsd_20mb_12k_contexts_20_senders`

Memory/cardinality/concurrency case.

Why it matters:

- Multi-sender UDS workload.
- Mixed metric types.
- Useful for context-cache, tag-cache, debug-view, and sustained memory behavior.
- Optimization goal is memory, so it complements the throughput-oriented v3 case.

## Comparison stages

### Stage A: baseline vs foundation

Goal: prove the committed foundation stack is neutral or beneficial under SMP.

Run:

```bash
mkdir -p reports/smp/dogstatsd-foundation

for case in \
  uds_dogstatsd_to_api_v3 \
  uds_dogstatsd_to_api \
  uds_dogstatsd_20mb_12k_contexts_20_senders
 do
  smp local run \
    --experiment-dir test/regression \
    --case "$case" \
    --baseline-image datadog/agent-dev:smp-dsd-main \
    --comparison-image datadog/agent-dev:smp-dsd-foundation \
    --replicates 3 \
    --total-samples 270 \
    2>&1 | tee "reports/smp/dogstatsd-foundation/${case}.log"
done
```

Success bar:

- No SMP regression in existing cases.
- Memory case is neutral or better.
- `serverDebug`/capture/lookback substrates do not create measurable overhead
  when disabled/defaulted.

### Stage A local result, 2026-05-17

Local Stage A completed under `reports/smp/dogstatsd-agg-serde-20260516-143205/`
with optimized Linux/arm64 images:

- baseline `main` image: `datadog/agent-dev:smp-dsd-main`, commit `3ec880f14a3`
- foundation image: `datadog/agent-dev:smp-dsd-foundation`, commit `53f7e8fdc3d`

SMP results:

| Case | Goal | Δ mean | Δ mean CI | Regression | Improvement |
|---|---|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.19% | [-0.46%, +0.07%] | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.18% | [-0.38%, +0.01%] | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | +0.70% | [+0.47%, +0.94%] | false | false |

Decision: Stage A supports the foundation branch being neutral enough to use as
the base for Stage B. It does not prove the aggregator/serializer speedup claim;
that remains gated on the experiment branch with semantic diff and payload-size
proof.

### Stage B: instrument-only experiment branch

Before changing dataflow, add measurement points on the experiment branch.

Suggested experimental telemetry/profiling points:

- DogStatsD samples parsed/enqueued.
- Aggregator `trackContext` time or sampled cost.
- Context resolver cache sizes and tag-filter cache hit/miss.
- Number of flushed series/sketch rows.
- Flush duration split:
  - aggregation flush,
  - context-to-serie materialization,
  - serializer v3 dictionary/column build,
  - compression/finish payload.
- Serializer v3 dictionary sizes:
  - names,
  - tag strings,
  - tagsets,
  - resources,
  - origins,
  - units.
- Payload metrics:
  - points per payload,
  - uncompressed bytes,
  - compressed bytes,
  - compressed bytes/point.

Keep instrumentation gated or sampling-based; this branch is for measurement,
not production-ready hot-path telemetry.

Run foundation vs instrumented experiment to ensure instrumentation overhead is
known before interpreting later results.

### Stage B local result, 2026-05-17

Local Stage B compared foundation (`datadog/agent-dev:smp-dsd-foundation`,
`53f7e8fdc3d`) against the instrumentation-only experiment image
(`datadog/agent-dev:smp-dsd-experiment`, `538ae360d89`). The image added
DogStatsD flush split telemetry, serializer timing telemetry, and v3
payload/dictionary stats without changing authoritative dataflow.

SMP results:

| Case | Goal | Δ mean | Δ mean CI | Regression | Improvement |
|---|---|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.29% | [-0.55%, -0.02%] | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.69% | [-0.96%, -0.41%] | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | +0.77% | [+0.56%, +0.99%] | false | false |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.04% | [-0.23%, +0.31%] | false | true |

Design adjustment: the source `uds_dogstatsd_to_api_v3` case still uses the
older `DD_SERIALIZER_EXPERIMENTAL_USE_V3_API_SERIES=true` environment knob and
did not emit current metrics-v3 payload telemetry. A local corrected case,
`uds_dogstatsd_to_api_v3_endpoint_fixed`, was added under the report tree with
`DD_SERIALIZER_EXPERIMENTAL_USE_V3_API_SERIES_ENDPOINTS` to exercise the current
v3 endpoint configuration.

Decision: Stage B instrumentation is neutral enough to use as the measurement
substrate for Stage C/D.

### Stage C: shadow segment builder

Prototype a shadow path that consumes the existing flushed `metrics.Serie` and
`metrics.SketchSeries` output and builds payload-aligned segments.

Current authoritative path still sends normal serializer output. The shadow path
is discarded after collecting measurements.

This proves:

- The segment model can represent current flushed series semantics.
- Segment dictionary cardinality and byte shape are competitive with v3
  serializer dictionaries.
- Semantic row count and content match the current serializer input.

It does not yet prove CPU savings, because it runs after the current path.

Acceptance:

- Zero semantic diffs on the SMP cases.
- Bounded memory use.
- Compressed/uncompressed byte estimates neutral or explainable.
- Clear profile showing where current serializer time is spent vs shadow build
  time.

Run:

```bash
mkdir -p reports/smp/dogstatsd-shadow-segments

for case in \
  uds_dogstatsd_to_api_v3 \
  uds_dogstatsd_20mb_12k_contexts_20_senders
 do
  smp local run \
    --experiment-dir test/regression \
    --case "$case" \
    --baseline-image datadog/agent-dev:smp-dsd-foundation \
    --comparison-image datadog/agent-dev:smp-dsd-experiment \
    --replicates 3 \
    --total-samples 270 \
    2>&1 | tee "reports/smp/dogstatsd-shadow-segments/${case}.log"
done
```

### Stage C local result, 2026-05-17

Local Stage C compared the instrumentation-only image
(`datadog/agent-dev:smp-dsd-experiment`, `538ae360d89`) against the shadow
segment image (`datadog/agent-dev:smp-dsd-shadow`, `911b22716ca`). The shadow
builder consumes the existing flushed `metrics.Serie` and `metrics.SketchSeries`
objects in the serializer, builds payload-local dictionary/cardinality
telemetry, and discards the result. Authoritative output remains unchanged.

SMP results:

| Case | Goal | Δ mean | Δ mean CI | Regression | Improvement |
|---|---|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.19% | [-0.38%, +0.01%] | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.35% | [-0.57%, -0.14%] | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | +0.72% | [+0.47%, +0.96%] | false | false |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | -0.04% | [-0.32%, +0.24%] | false | false |

Decision: Stage C did not introduce an SMP regression and provides useful
payload-shape telemetry, but it still runs after the current path and does not
prove CPU savings.

### Stage D: direct aggregator row sink

Prototype the actual core improvement.

Current path:

```text
ContextMetrics.Flush() -> []*metrics.Serie -> serializer v3 -> payload
```

Experimental path:

```text
ContextMetrics.FlushRows(contextResolver, rowSink)
  -> payload-aligned metric rows using existing Context metadata
  -> segment/v3 builder
  -> payload
```

Design constraints:

- Keep current path authoritative at first.
- Run direct rows in shadow mode and compare normalized semantic rows.
- Do not alter DogStatsD visible behavior, tag filtering, counter zero-fill,
  histogram expansion, sketch flushing, no-index, source, host/resources, or
  unit semantics.
- If a metric type cannot be represented safely yet, explicitly fall back and
  count it.

This is the first stage that can prove reduced allocations/CPU by avoiding or
minimizing `metrics.Serie` reconstruction and duplicate serializer dictionary
work.

Local implementation note: the first Stage D experiment kept the current path
authoritative and added a direct row shadow observer at the aggregator flush
boundary. It observes rows after context resolution/filtering and before
appending to the existing sinks. This proves low-overhead aggregator-row
visibility, but it still does not remove `metrics.Serie` materialization and, in
its current form, observes before later sink-level host-tag injection. Treat it
as a step toward `FlushRows`, not as the final direct-output proof.

Acceptance:

- Zero semantic diffs for supported rows.
- Explicit fallback counters for unsupported rows.
- Lower or neutral flush CPU in profiles.
- Lower allocations during flush/serialize.
- Neutral or better compressed bytes/point.

### Stage D local result, 2026-05-17

Local Stage D compared the shadow segment image (`datadog/agent-dev:smp-dsd-shadow`,
`911b22716ca`) against the direct aggregator row shadow image
(`datadog/agent-dev:smp-dsd-direct-row`, `e3f2f987056`). Stage C serializer
shadowing remained enabled, so this isolates the added aggregator-row observer.

SMP results:

| Case | Goal | Δ mean | Δ mean CI | Regression | Improvement |
|---|---|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3` | ingress throughput | +0.46% | [+0.18%, +0.74%] | false | true |
| `uds_dogstatsd_to_api` | ingress throughput | -0.43% | [-0.65%, -0.22%] | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | +0.30% | [+0.09%, +0.52%] | false | false |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.16% | [-0.04%, +0.36%] | false | true |

Decision: the direct row observer is neutral enough to keep iterating, but this
is not sufficient proof to switch output in production. For local experiments,
Stage E deliberately relaxes that safety gate to estimate end-goal performance
from an active direct serializer path.

### Stage E: direct path as comparison output

For local-only experiments, allow the experiment image to send payloads through a
new direct serializer path while keeping commits and results isolated. Stage E
is enabled by `DD_DOGSTATSD_EXPERIMENTAL_DIRECT_SERIALIZER=true` in copied local
SMP cases.

Local Stage E implementation (`9498d5fee95`) bypasses `IterableSeries` /
`IterableSketches` channel traversal and consumer goroutines. The demultiplexer
passes producer rows directly to `Serializer.SendDirectSeriesAndSketches`, which
writes into the existing v2/v3 serializer pipeline builders and then sends those
pipelines. This is intentionally unsafe and incomplete: it still consumes
existing `metrics.Serie` and `metrics.SketchSeries` rows and supports the
protobuf v2/v3 APIs only.

### Stage E local result, 2026-05-17

Local Stage E compared the direct-row shadow image
(`datadog/agent-dev:smp-dsd-direct-row`, `e3f2f987056`) against the active
direct serializer image (`datadog/agent-dev:smp-dsd-direct-active`,
`9498d5fee95`). Both variants used local-only case copies with
`DD_DOGSTATSD_EXPERIMENTAL_DIRECT_SERIALIZER=true`; the baseline image ignores
that env var.

SMP results:

| Case | Goal | Δ mean | Δ mean CI | Regression | Improvement |
|---|---|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.21% | [-0.43%, +0.01%] | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.07% | [-0.39%, +0.24%] | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | -0.94% | [-1.15%, -0.72%] | false | true |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.56% | [+0.27%, +0.84%] | false | true |

Decision: active direct serialization is locally non-regressing and improves the
corrected current-v3 endpoint plus memory case. It does not prove a broad
throughput win for v2 or the source v3-labeled case. The next experiment should
remove more aggregator materialization or feed the v3 builder from a row
representation that avoids `metrics.Serie` mutation altogether.

### Stage F: direct DogStatsD series rows

Stage F moves one more core boundary into the new design while keeping a separate
local-only switch:

```text
DD_DOGSTATSD_EXPERIMENTAL_DIRECT_SERIALIZER=true
DD_DOGSTATSD_EXPERIMENTAL_DIRECT_ROWS=true
```

Local Stage F implementation (`c7327ad816c`) adds `metrics.SerieRow` as the
serializer-visible row model and lets `TimeSampler.flushSeries` emit rows
directly when the sink supports `metrics.SerieRowSink`. The row normalizes
`device:` and `dd.internal.resource:` tags into dedicated protobuf fields
without requiring serializer-side mutation of shared `*metrics.Serie` objects.
This is still incomplete: sketches, check-sampler series, and metric flush/dedup
internals still use existing structs.

Local Stage F compared the direct-active image
(`datadog/agent-dev:smp-dsd-direct-active`, `9498d5fee95`) against the direct
series row image (`datadog/agent-dev:smp-dsd-direct-rows`, `c7327ad816c`).

SMP results:

| Case | Goal | Δ mean | Δ mean CI | Regression | Improvement |
|---|---|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3` | ingress throughput | +0.22% | [-0.03%, +0.47%] | false | true |
| `uds_dogstatsd_to_api` | ingress throughput | -0.13% | [-0.36%, +0.10%] | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | -0.21% | [-0.44%, +0.01%] | false | true |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | -0.12% | [-0.37%, +0.13%] | false | false |

Decision: direct DogStatsD series rows are locally non-regressing and essentially
neutral. This proves the active time-sampler handoff can move to a row model, but
it does not yet prove a significant speedup. The next stage should remove more
materialization inside metric flush/dedup, or add row-native sketch/check-sampler
handoff, before rerunning foundation-vs-experiment.

### Stage G: direct metric row flush

Stage G (`638b79c3bba`) adds `metrics.SerieRowFragment` and a row-oriented
`ContextMetricsFlusher` path behind:

```text
DD_DOGSTATSD_EXPERIMENTAL_DIRECT_METRIC_ROWS=true
```

on top of the Stage F switches. The goal is a thin value probe: bypass
`*metrics.Serie` allocation inside scalar `Metric.flush` and feed lightweight row
fragments into the direct serializer-visible row path. Histogram/historate still
fall back through the existing `*Serie` materialization path.

SMP results versus Stage F direct series rows:

| Case | Goal | Δ mean | Δ mean CI | Regression | Improvement |
|---|---|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.08% | [-0.35%, +0.19%] | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.00% | [-0.24%, +0.24%] | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | -0.52% | [-0.76%, -0.28%] | false | true |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.02% | [-0.23%, +0.26%] | false | true |

Because the standard throughput cases are effectively capped near their
100 MiB/s generator target, Stage G also added a high-rate metrics-only probe:

```text
uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only
bytes_per_second: 250 MiB
kind_weights: metric=100,event=0,service_check=0
```

On that probe:

| Case | Goal | Δ mean | Δ mean CI | Regression | Improvement |
|---|---|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -0.25% | [-0.48%, -0.01%] | false | false |

Decision: removing `*metrics.Serie` allocation gives a small memory win but no
throughput win, and it slightly hurts in the uncapped/high-rate probe. This is
evidence that `*Serie` allocation itself is not the main value lever.

### Stage H: unordered direct context rows upper-bound probe

Stage H (`c989043ddcc`) adds an intentionally unsafe upper-bound switch:

```text
DD_DOGSTATSD_EXPERIMENTAL_DIRECT_CONTEXT_ROWS=true
```

on top of Stage G. It bypasses context grouping/dedup in
`ContextMetricsFlusher` and flushes row fragments in timestamp/map iteration
order. It is not wire-equivalent because it can emit repeated rows for the same
identity instead of merging points.

SMP result versus Stage G direct metric rows on the high-rate probe:

| Case | Goal | Δ mean | Δ mean CI | Regression | Improvement |
|---|---|---:|---:|---|---|
| `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -0.51% | [-0.73%, -0.29%] | false | false |

Decision: the shortcut is worse. Naively removing grouping/dedup increases
downstream row/payload work more than it saves. Combined with Stage G, the local
value read is now negative for selling this as a major throughput/efficiency
investment based on direct rows alone. Continue only if profiles or production
workloads identify a different bottleneck than serializer iteration,
`*metrics.Serie` allocation, or context grouping.

Run foundation vs experiment on the main cases again only if a later stage finds
a materially positive value signal; the current Stage G/H signal does not justify
that broader comparison.

Success bar for making a significant core-improvement claim:

- `uds_dogstatsd_to_api_v3`: statistically meaningful ingress-throughput
  improvement, or a clear reduction in CPU/allocations at the same throughput.
- `uds_dogstatsd_20mb_12k_contexts_20_senders`: neutral or improved memory.
- No regression in `uds_dogstatsd_to_api`.
- Compressed bytes/point neutral or better; tolerate only a very small increase
  if CPU/heap improvements are large and documented.
- Profiles show reduced time in context/tag reconstruction, `metrics.Serie`
  materialization, serializer dictionary building, or compression-bound flush
  stalls.

## Optional local-only cases to add if existing cases are not discriminating

Add these only after the existing cases establish a baseline.

### `uds_dogstatsd_v3_high_cardinality_tags`

Purpose: maximize repeated tag hashing/dedup/dictionary pressure.

Suggested shape:

- UDS datagram generator.
- v3 serializer enabled.
- 30-60 MiB/s locally, tuned to avoid packet loss.
- 25k-100k contexts.
- 10-50 tags per message.
- Duplicate/reordered tags enabled through lading variability.
- Metric mix: count/gauge/distribution/set/histogram enough to exercise both
  series and sketches.

### `uds_dogstatsd_v3_many_small_series`

Purpose: expose per-series overhead and dictionary churn.

Suggested shape:

- Many metric names and tagsets.
- Low multivalue count.
- Small point batches per context.
- v3 serializer enabled.

### `uds_dogstatsd_v3_tag_filtering`

Purpose: test interaction with `metric_tag_filterlist` and the tag-filter cache.

Suggested shape:

- Enable tag filter stripping in Agent config.
- Include tags designed to match and not match the filter.
- Compare cache hit/miss and context cardinality changes.

### `uds_dogstatsd_v3_always_on_views`

Purpose: quantify the cost of enabling the new always-on substrates.

Suggested toggles, as they become available:

- raw ingress ring enabled with bounded size,
- lookback store enabled,
- semantic shadow projection enabled,
- debug stats retained contexts budget configured.

The claim here is not that these are free; it is that they are bounded and cheap
enough to be enabled intentionally.

## Report artifact layout

Use a stable layout so results can be reviewed and compared:

```text
reports/smp/dogstatsd-agg-serde-YYYYMMDD/
  README.md                         # summary and conclusion
  images.txt                        # image tags, branches, commits, build time
  environment.txt                   # host arch, Docker/Colima info, smp/lading versions
  cases/
    uds_dogstatsd_to_api_v3.log
    uds_dogstatsd_to_api.log
    uds_dogstatsd_20mb_12k_contexts_20_senders.log
  captures/
    <copied comparative-captures per case>
  profiles/
    <pprof or internal profiling links/files if collected>
  summary.csv                       # one row per case/comparison
```

Useful extraction command:

```bash
rg "Comparative Analysis Results|Δ mean|Δ mean CI|Confidence|Erratic|Regression|Improvement" \
  reports/smp/dogstatsd-*/cases/*.log
```

## Report template

For each comparison, summarize:

| Case | Goal | Baseline image | Comparison image | Δ mean | Δ mean CI | Confidence | Regression | Improvement | Notes |
|---|---|---|---|---:|---:|---:|---|---|---|

Then add:

- correctness result: semantic diffs/fallbacks/dropped rows,
- payload result: compressed bytes/point and uncompressed bytes/point,
- CPU/profile result: top functions reduced/increased,
- memory result: heap/rss/context counts,
- decision: continue, adjust, or abandon the core serializer claim.

## Decision framework

### Strong case

We can position this as significant core aggregator+serializer work if local SMP
shows:

- zero semantic diffs,
- no existing-case regressions,
- meaningful improvement in `uds_dogstatsd_to_api_v3`,
- neutral/better memory in the high-context case,
- neutral/better compressed bytes per point,
- profiles identify removed duplicate work.

### Narrower case

If core throughput does not improve but overhead remains neutral, position the
foundation as:

- safer bounded `serverDebug`,
- better capture/replay substrate,
- recent lookback foundation,
- future serializer integration option.

### Stop/rollback signal

Do not push the core-pipeline claim if:

- SMP flags regressions in existing cases,
- compressed payloads grow materially,
- semantic diffs appear in histograms, distributions, sets, counter expiry, tag
  filtering, or origin enrichment,
- direct rows only win microbenchmarks but not SMP cases,
- complexity requires broad production rewrites without a measurable SMP win.

## Stage I local result, 2026-05-18

Stage I implemented the intentionally narrow vertical slice that Stage A-H had
not measured: supported DogStatsD metric samples are accepted after normal
parser/enrichment, inserted into an experimental shard-local columnar v3 table,
and flushed directly as v3 series rows. The slice is gated by
`DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3=true` and only supports on-time Gauge,
Counter, Count, and Set samples; everything else falls back to the legacy path.

Important scope correction: this is still not a full production architecture.
It is v3-only, metric-only, local-only, and unsafe. It does, however, bypass
`TimeSampler`, `ContextMetrics`, `metrics.Metric`, `metrics.Serie`, and iterable
serializer traversal for supported samples, so it is a closer proof/disproof of
the columnar performance idea than Stages A-H.

Stage I images compared against Stage G direct metric rows on
`uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only`:

| Variant | Commit | Δ mean | Δ mean CI | Regression | Improvement |
|---|---|---:|---:|---|---|
| naive parser direct insert | `2ade84801cc` | -4.74% | [-5.12%, -4.35%] | false | false |
| merged flush rows | `9b04b0c6104` | -5.28% | [-5.63%, -4.93%] | false | false |
| descriptor reuse | `0e774a353cb` | -6.68% | [-6.96%, -6.40%] | false | false |
| deferred insert telemetry | `6f3c5ae857a` | -6.02% | [-6.28%, -5.77%] | false | false |
| batched shard workers | `7a43a9d0dae` | -8.02% | [-8.27%, -7.76%] | false | false |
| batched/no-lock workers | `87fbbc1fbb5` | -8.61% | [-8.86%, -8.37%] | false | false |
| bucket-row cache | `e68c3c36110` | -7.11% | [-7.44%, -6.78%] | false | false |

Proof telemetry in Stage I showed the intended bypass:

- old DogStatsD aggregator contexts effectively zero on the comparison side,
- `dogstatsd_columnar_v3.stats{stat:inserted_samples}` > 200k/s,
- columnar flush rows emitted into the direct v3 row serializer,
- legacy direct metric row phase nearly empty for supported samples.

But the columnar variants all increased DogStatsD packet backlog/RSS and reduced
ingress throughput. The strongest interpretation is that this columnar table and
handoff shape shift cost into ingest/worker processing faster than serializer
savings can recover it.

Decision: do not pitch the current columnar replacement as a near-term
throughput win. Keep the foundation and row/dictionary work as useful substrate
for debug/capture/lookback/replay and future options, but require a different
bottleneck/proof point before investing in broad production migration.

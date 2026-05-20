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

## Stage J local result, 2026-05-18

Stage I's negative result turned out to be an implementation artifact. The
columnar path forced `identity.Builder.ResolveHotPath` for every metric sample
when `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3=true`, even with debug stats
disabled and `dogstatsd_pipeline_count=1`. In that configuration the direct-row
baseline did not need parser-side sharding context, while columnar computed both
an unused debug projection and the shard/backend projection.

Stage J changed the hot path to compute only the shard identity when debug is
off. This removes the unused debug key and `strings.Join` display tag string
from the parser hot path. A follow-up generalizes the same idea for sharded
batchers while preserving `HotPathContext.Client` for contracts/tests.

Stage J SMP results:

| Comparison | Case | Δ mean | Δ mean CI | Confidence | Decision |
|---|---|---:|---:|---:|---|
| direct metric rows → columnar shard-only | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +13.57% | [+13.28%, +13.86%] | 100.0% | throughput proof recovered |
| columnar bucket-cache → columnar shard-only | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +24.07% | [+23.57%, +24.57%] | 100.0% | unused debug projection was the major bottleneck |
| direct metric rows → columnar shard-only | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +2.83% | [+2.44%, +3.22%] | 100.0% | standard v3 case improves modestly |
| columnar shard-only → skip legacy flush | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | -0.22% | [-0.57%, +0.14%] | 56.7% | empty fallback flush is not the limiter |

The theory was directionally sound but incomplete. It said "share identity work"
without being strict enough about *which projection* was needed by each view.
`serverDebug` grouping is a compatibility view, not the backend identity; Stage
I accidentally put that view's key on the always-on columnar ingestion path.
Stage J validates the architectural rule that projections must be materialized
only for active consumers.

Current productionization read:

- Throughput potential exists: +13.57% on the high-rate metrics-only v3 probe.
- Memory is not acceptable yet: the standard corrected v3 case showed roughly
  +30% RSS and +39% heap despite lower CPU, and the high-rate overload case still
  builds large packet/channel backlog.
- The implementation is still not ideal: it emits `metrics.SerieRow` rather than
  native v3 protobuf columns, and it does not yet have descriptor expiry or a
  memory-bounded columnar metadata strategy.

Next decision gate: keep the Stage J throughput fix, but require memory/backlog
experiments before claiming an all-around win.

## Stage K/L local result, 2026-05-18

Stage K investigated why Stage J's faster columnar path accumulated a large
`packetsIn` backlog while the slower direct-row baseline did not. A rate sweep
and CPU-allotment sweep showed a bottleneck shift: the baseline backpressures
before the Agent packet channel, while columnar reduces per-sample work enough
to admit more traffic and expose the existing large channel as the overload
absorber.

This motivated Stage L: replace the large buffered `packetsIn` channel, under a
local-only env gate, with a byte-bounded in-memory ingress log.

Prototype commits: `eae5cb364ee` plus follow-up `7630c423017` to avoid recording blocked-time telemetry when an append did not actually block. The SMP numbers below were gathered before the telemetry follow-up, so they are conservative for the ingress-log path.

Prototype gates:

- `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG=true`
- `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES=16777216`

Implementation shape:

```text
listeners -> tiny listener/log handoff channel -> byte-bounded ingress log -> workers
```

The first prototype intentionally keeps packet batches as the record type, so it
is not the final zero-copy raw WAL. It does, however, move overload policy from a
large implicit channel to an explicit byte-bound with telemetry:

- `dogstatsd_ingress_log.bytes`
- `dogstatsd_ingress_log.batches`
- `dogstatsd_ingress_log.packets`
- `dogstatsd_ingress_log.blocked_ns`
- `dogstatsd_ingress_log.stats{stat:*}`

Local Stage L results are single-replicate probes and should be rerun before any
strong external claim.

| Comparison | Case | Δ mean | Key read |
|---|---|---:|---|
| columnar env-off -> columnar ingress-log, 16MiB cap | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | -7.79% | deliberate backpressure trades some peak overload throughput for much lower memory/backlog |
| direct metric rows -> columnar ingress-log, 16MiB cap | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +4.90% | keeps a throughput win while controlling high-rate queue memory |
| direct metric rows -> columnar ingress-log, 16MiB cap | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +1.22% | standard case remains modestly positive/neutral |

Selected high-rate metrics, direct metric rows -> columnar ingress-log:

| Variant | Agent UDS MiB/s | processed/s | packet pool avg | ingress log avg | RSS MiB | heap MiB |
|---|---:|---:|---:|---:|---:|---:|
| direct metric rows | 193.23 | 238,830 | 528 | n/a | 247.12 | 63.16 |
| columnar ingress-log | 202.36 | 250,077 | 807 | 8.3 MiB | 228.91 | 58.09 |

Selected high-rate metrics, columnar env-off -> columnar ingress-log:

| Variant | Agent UDS MiB/s | processed/s | packet channel batches | packet pool avg | RSS MiB | heap MiB |
|---|---:|---:|---:|---:|---:|---:|
| columnar env-off | 221.01 | 272,352 | 777.7 | 24,758 | 734.12 | 365.30 |
| columnar ingress-log | 202.75 | 250,566 | 0.3 | 761 | 226.43 | 58.63 |

Decision: the database-inspired raw ingress log is a promising next-level
abstraction. It gives up some unconstrained overload admission compared with the
large channel, but it converts that hidden memory sink into explicit backpressure
while preserving a measured high-rate win over direct metric rows. Continue with
this direction, but the next version should reduce the extra pump/channel hop and
move from packet-batch records toward a true preallocated byte ring with parser
cursors.

## Stage M local result, 2026-05-19

Stage M tested the next ingress architecture probes after Stage L:

- **M1 sharded packet-batch ingress log** (`057442d4163`, telemetry follow-ups `e61dba295aa` / `d4ed40c5185`): listeners flush packet batches directly into per-worker log shards, removing Stage L's central pump/channel handoff.
- **M2 raw UDS datagram ingress ring** (`052f776aef1`, telemetry follow-up `53a887497f6`): the UDS datagram listener reserves preallocated fixed slots, reads directly into ring-owned storage, commits record metadata, and workers parse/release records without using the packet pool.

Prototype gates:

- `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_SHARDED=true`
- `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS=true`
- shared byte budget: `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES=16777216`

M2 is currently UDS-datagram-only and disables itself when UDP, stream sockets,
named pipes, statsd forwarding, or origin detection are active. The local raw SMP
cases set `dogstatsd_port: 0`; earlier raw runs without that setting were invalid
because UDP made the raw gate fall back to the packet path.

Local Stage M results are single-replicate probes (`--replicates 1 --total-samples 150`):

| Comparison | Case | Δ mean | Key read |
|---|---|---:|---|
| Stage L ingress-log -> M1 sharded packet-batch log | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | -3.77% | sharding/removing the pump did not recover throughput in this run |
| direct metric rows -> M1 sharded packet-batch log | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | -0.09% | throughput neutral, lower RSS/heap and lower packet-pool backlog |
| direct metric rows -> M2 raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +0.80% | small high-rate win, zero packet-pool backlog, lower RSS/heap |
| direct metric rows -> M2 raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +1.16% | standard corrected-v3 case remains positive |

Selected direct-vs-raw high-rate metrics:

| Variant | Agent UDS MiB/s | processed/s | packet pool avg | ingress ring avg | RSS MiB | heap MiB |
|---|---:|---:|---:|---:|---:|---:|
| direct metric rows | 197.56 | 244,007 | 1,450 | n/a | 185.29 | 81.39 |
| raw UDS ring | 199.74 | 246,846 | 0 | 3.34 MiB / 913 slots | 157.26 | 61.60 |

Decision: M2 validates the raw-ingress-ring memory/backpressure shape but not the
full Stage J overload-throughput ceiling. It removes the heap-backed packet-pool
backlog entirely for UDS datagrams while staying slightly faster than direct
metric rows. Continue the raw ingress direction, but the next ceiling test should
replace fixed `dogstatsd_buffer_size` slots with a compact variable-length or
slabbed byte ring, batch notifications/cursors, and include origin/OOB metadata
without regressing the hot path.

## Stage N local result, 2026-05-19

Stage N tested the next raw-ingress ceiling after M2's fixed-slot ring: retain
records in a compact preallocated byte ring instead of dedicating one
`dogstatsd_buffer_size` slot per datagram.

Prototype commit: `4b2c4b6b7e6`.

Prototype gate:

- `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_COMPACT=true`
- shared byte budget: `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES=16777216`

Implementation shape:

```text
UDS datagram listener -> reusable scratch read buffer -> commit exact n bytes
  -> per-worker compact byte ring + metadata ring -> worker cursor -> parser
```

This deliberately trades one scratch-to-ring copy for denser retention. It keeps
M2's current eligibility constraints: UDS datagram only, no UDP, no stream
socket, no named pipe, no statsd forwarding, and no origin detection.

Local Stage N results are single-replicate probes (`--replicates 1 --total-samples 150`):

| Comparison | Case | Δ mean | Key read |
|---|---|---:|---|
| fixed-slot raw UDS ring -> compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +3.92% | compact byte retention beats fixed slots under high offered load |
| direct metric rows -> compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +7.25% | best bounded-ingress high-rate result so far; packet-pool backlog remains zero |
| direct metric rows -> compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +2.17% | standard corrected-v3 case remains positive |
| fixed-slot raw UDS ring -> compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +0.05% | standard case is neutral versus fixed-slot raw |

Selected high-rate direct-vs-compact metrics:

| Variant | Agent UDS MiB/s | processed/s | packet pool avg | ingress ring avg | RSS MiB | heap MiB |
|---|---:|---:|---:|---:|---:|---:|
| direct metric rows | 186.40 | 230,366 | 612 | n/a | 173.39 | 71.67 |
| compact raw ring | 200.10 | 247,216 | 0 | 8.15 MiB / 2,174 records | 163.09 | 63.32 |

Decision: byte-compact retained records are a real ceiling improvement. Stage N
recovers a meaningful chunk of high-rate throughput while preserving the main
M2 property: no heap-backed packet-pool backlog. The next ceiling should focus
on batched compact-ring drains/cursors and/or a no-copy direct-reservation or
size-class slab variant, while adding oldest-age/lag telemetry so higher ring
occupancy is interpreted as bounded backlog rather than hidden overload.

## Stage O local result, 2026-05-19

Stage O tested the next ceiling after Stage N's compact byte ring: reduce
worker-side ring lock traffic by draining and releasing multiple committed raw
records per notification/cursor pass.

Prototype commit: `dc29e8fff7c`.

Prototype gates:

- `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_BATCH_DRAIN=true`
- `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_BATCH_DRAIN_SIZE=32`
- compact raw ring still enabled with `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_COMPACT=true`
- shared byte budget: `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES=16777216`

Implementation shape:

```text
worker raw-ring notification
  -> TryNextBatch(dst[:0])      # one shard lock, up to 32 records
  -> parse records sequentially # no per-record release
  -> ReleaseBatch(len(batch))   # one shard lock for release
```

This keeps Stage N's listener behavior: read into reusable scratch and copy the
actual datagram bytes into the compact ring on commit. It does not add origin or
OOB support and has the same raw-mode limitations as Stage N.

Local Stage O results are single-replicate probes (`--replicates 1 --total-samples 150`):

| Comparison | Case | Δ mean | Key read |
|---|---|---:|---|
| compact raw UDS ring -> batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +1.77% | batched worker drains help under high offered load |
| direct metric rows -> batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +7.39% | best bounded-ingress high-rate result so far; packet-pool backlog remains zero |
| compact raw UDS ring -> batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +0.18% | standard case is neutral/slightly positive versus Stage N compact |
| direct metric rows -> batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +1.33% | standard corrected-v3 case remains positive versus direct rows |

Selected high-rate direct-vs-batch metrics:

| Variant | Agent UDS MiB/s | processed/s | packet pool avg | ingress ring avg | RSS MiB | heap MiB |
|---|---:|---:|---:|---:|---:|---:|
| direct metric rows | 196.70 | 243,054 | 879 | n/a | 172.41 | 72.18 |
| compact raw ring + batch drain | 211.66 | 261,559 | 0 | 8.29 MiB / 2,249 records | 164.14 | 67.83 |

Decision: worker-side batching is a real but smaller ceiling step than compact
byte retention. It should remain a candidate if the raw-ring design continues,
but the next larger ceiling is likely listener-side no-copy storage: a direct
reservation or size-class/slabbed compact ring that removes the scratch-to-ring
copy while preserving bounded backpressure and adding oldest-age/lag telemetry.

## Stage P local result, 2026-05-19

Stage P tested a simple no-copy direct-reservation variant after Stage O:
reserve a max-size contiguous span in the compact raw ring before the UDS socket
read, read directly into ring-owned storage, then reclaim unused bytes on commit.

Prototype commit: `83d06509167`.

Prototype gates:

- `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_DIRECT_COMPACT=true`
- `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_BATCH_DRAIN=true`
- `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_BATCH_DRAIN_SIZE=32`
- shared byte budget: `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES=16777216`

Implementation shape:

```text
UDS datagram listener
  -> Reserve() waits for a max-size contiguous ring reservation
  -> ReadFromUnix(ring-owned reservation)
  -> Commit(n) publishes n bytes and reclaims reserved-n bytes
  -> workers drain with TryNextBatch / ReleaseBatch
```

This removes Stage N/O's scratch-to-ring copy, but also requires one full
`dogstatsd_buffer_size` contiguous reservation before every socket read. The
prototype intentionally allows only one outstanding direct reservation per shard;
this matches the single UDS datagram listener experiment shape, but is not a
general multi-producer design.

Local Stage P results are single-replicate probes (`--replicates 1 --total-samples 150`):

| Comparison | Case | Δ mean | Key read |
|---|---|---:|---|
| compact batch-drain raw UDS ring -> direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | -3.30% | simple direct reservation is worse at high offered load |
| direct metric rows -> direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +4.46% | still positive vs direct rows, but worse than Stage O's +7.39% |
| compact batch-drain raw UDS ring -> direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | -0.03% | standard corrected-v3 case is neutral versus Stage O |
| direct metric rows -> direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +1.78% | standard corrected-v3 case remains positive versus direct rows |

Selected high-rate compact-batch-vs-direct metrics:

| Variant | Agent UDS MiB/s | processed/s | packet pool avg | ingress ring avg | RSS MiB | heap MiB |
|---|---:|---:|---:|---:|---:|---:|
| compact raw ring + batch drain | 210.06 | 259,521 | 0 | 8.48 MiB / 2,308 records | 162.51 | 68.83 |
| direct compact raw ring + batch drain | 203.06 | 250,883 | 0 | 8.61 MiB / 2,391 records | 163.77 | 67.15 |

Decision: the simple no-copy hypothesis is false in this shape. Avoiding the
copy is outweighed by pre-read max-size contiguous reservation/backpressure.
Stage N/O's scratch-copy compact ring remains the best bounded-ingress result
so far. The next ceiling should not be one-max-slot direct reservation; better
candidates are true size-class/slabbed storage, listener/syscall batching, and
oldest-age/consumer-lag telemetry before making further overload claims.

## Stage Q local result, 2026-05-19

Stage Q tested the final serialization-stage hypothesis: if aggregation is
already maintained in a v3-aligned columnar shape, can the flush path construct
v3 metric payload inputs directly instead of reconstructing `metrics.Serie`
rows first?

Prototype commit: `947dc3f2ec3`.

Image metadata:

- `datadog/agent-dev:smp-dsd-columnar-v3-native-columnar-v3`
- image ID `sha256:b6b4c8aeaa277952d6f13608ef1654fd2d4ba5e23c924c8676a693fba87e0e4b`
- agent version `Agent 7.81.0-devel - Meta: git.73.947dc3f - Commit: 947dc3f2ec3`
- final `main` comparison image: `datadog/agent-dev:smp-dsd-main`, image ID
  `sha256:6f67e85689c833453bb60ba0697d234561a39a9f8df37d36f2a7fd6372316419`,
  commit `3ec880f14a3`

Prototype gates:

- v3 native path:
  `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_NATIVE_SERIALIZER=true`
- v2/direct-series compatibility path:
  `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_DIRECT_SERIES_SERIALIZER=true`
- for the bounded UDS datagram slice, Stage O compact raw ingress remained
  enabled:
  - `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_COMPACT=true`
  - `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_BATCH_DRAIN=true`
  - `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_BATCH_DRAIN_SIZE=32`
  - shared byte budget: `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES=16777216`

Implementation shape:

```text
columnar DogStatsD buckets
  -> merge points per descriptor across flushed buckets
  -> metrics.V3MetricPointRow / V3MetricPointRowSink
  -> serializer native v3 row writer
  -> existing v3 payload builder/compressor/sender
```

The first version emitted one row per bucket, which preserved semantics but
changed payload cardinality. The retained version merges points for the same
columnar descriptor across buckets before writing a native row, preserving the
existing v3 payload point/payload shape in the SMP cases.

A reasonable v2 integration was added in the same stage: columnar aggregation can
also flush into `metrics.SerieRow` through `SendDirectSeriesAndSketches` when
`DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_DIRECT_SERIES_SERIALIZER=true`. The demux
selection order is now native v3, direct-series/v2, default columnar-v3
`SerieRow`, direct serializer experiment, then the normal path.

Local verification:

```bash
dda inv test --targets=./pkg/metrics,./pkg/serializer/internal/metrics,./pkg/aggregator --timeout=300
```

Result: all relevant tests passed locally (`397 passed` before the v2 integration
and `222 passed` for the aggregator-focused follow-up run).

Stage Q isolation runs compare Stage O compact raw ring + batch drains to the
native v3 serializer path. These are single-replicate probes
(`--replicates 1 --total-samples 150`):

| Comparison | Case | Δ mean | Δ mean CI | Key read |
|---|---|---:|---:|---|
| Stage O compact/batch -> native v3 | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +0.80% | [-0.10%, +1.71%] | neutral/slightly positive, below the confidence threshold |
| Stage O compact/batch -> native v3 | `uds_dogstatsd_to_api_v3_endpoint_fixed` | -0.65% | [-1.29%, -0.02%] | neutral/slightly negative standard-case result |

Selected Stage Q isolation metrics:

| Case | Variant | Agent UDS MiB/s | processed/s | v3 compressed MiB/s | v3 uncompressed MiB/s | payloads/s | RSS MiB | heap MiB |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| high-rate | Stage O compact/batch | 188.60 | 233,053 | 0.0565 | 0.0753 | 0.183 | 163.22 | 66.11 |
| high-rate | native v3 | 190.13 | 234,919 | 0.0564 | 0.0752 | 0.183 | 161.62 | 65.99 |
| standard | Stage O compact/batch | 97.41 | 112,281 | 0.0565 | 0.0754 | 0.183 | 462.58 | 224.08 |
| standard | native v3 | 96.90 | 111,685 | 0.0566 | 0.0754 | 0.183 | 450.26 | 221.05 |

Final `main` honesty-gate probes compare the current production baseline image
to the Stage Q design. These are also single-replicate local SMP runs:

| Comparison | Case | Δ mean | Δ mean CI | Confidence | Key read |
|---|---|---:|---:|---:|---|
| `main` -> Stage Q native v3 | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | +8.10% | [+7.47%, +8.73%] | 100.0% | bounded raw ring removes packet-pool backlog and throughput is higher |
| `main` -> Stage Q native v3 | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +1.67% | [+1.05%, +2.28%] | 99.9% | standard corrected-v3 case is positive, but memory remains higher |
| `main` -> Stage Q direct-series/v2 | `uds_dogstatsd_to_api` high-rate local variant | +9.77% | [+8.76%, +10.78%] | 100.0% | v2/direct-series integration works and is positive in the high-rate probe |
| `main` -> Stage Q direct-series/v2 | `uds_dogstatsd_to_api` local variant | +3.27% | [+2.50%, +4.03%] | 100.0% | v2/direct-series standard probe is positive, with higher memory |
| `main` -> Stage Q native v3, origin detection on | `uds_dogstatsd_to_api_v3_endpoint_fixed` | +2.68% | [+1.69%, +3.66%] | 99.9% | raw ingress disables itself under origin detection; this probes columnar/native without raw-ring support |

Selected final metrics:

| Case | Variant | Agent UDS MiB/s | processed/s | packet pool avg | ingress ring avg | RSS MiB | heap MiB |
|---|---|---:|---:|---:|---:|---:|---:|
| v3 high-rate | main | 191.06 | 236,113 | 652 | n/a | 158.70 | 65.55 |
| v3 high-rate | Stage Q native | 205.68 | 254,127 | 0 | 7.88 MiB / 2,137 records | 162.52 | 65.39 |
| v3 standard | main | 95.97 | 110,598 | 176 | n/a | 410.79 | 204.01 |
| v3 standard | Stage Q native | 97.94 | 112,882 | 0 | 0.54 MiB / 140 records | 446.49 | 213.41 |
| v2 high-rate | main | 179.51 | 221,873 | 305 | n/a | 153.11 | 58.89 |
| v2 high-rate | Stage Q direct-series | 197.46 | 243,958 | 0 | 8.44 MiB / 2,268 records | 168.16 | 66.96 |
| v2 standard | main | 94.79 | 109,270 | 47 | n/a | 411.28 | 202.97 |
| v2 standard | Stage Q direct-series | 97.60 | 112,487 | 0 | 0.73 MiB / 197 records | 452.64 | 223.63 |
| origin-on v3 standard | main | 94.56 | 108,998 | 24 | n/a | 452.71 | 233.60 |
| origin-on v3 standard | Stage Q native, raw disabled | 96.31 | 111,005 | 86 | n/a | 452.42 | 226.11 |

Decision: the native v3 serializer is important architecturally because it gives
the columnar path a direct payload-construction API and preserves payload size,
but it did not produce a large incremental SMP win over Stage O by itself. The
observed `main` wins are therefore best attributed to the combined design
(columnar aggregation plus bounded compact raw ingress, with native/direct-series
serialization), not to the native serializer alone. The design-stage exploration
should freeze here and move to broader honesty gates, feature-cost comparisons,
memory profiling, and raw-ring lag/oldest-age/backpressure telemetry.

## Raw-ring lag/backpressure telemetry, 2026-05-19

After Stage Q, raw ingress rings gained the telemetry needed to make high-rate
results more honest about hidden backlog and listener backpressure.

New gauges under `dogstatsd_ingress_ring`:

- `consumer_lag_records{shard}`: committed records waiting for the worker.
- `consumer_lag_bytes{shard}`: committed retained bytes waiting for the worker.
  For direct-reserve compact mode this excludes the in-flight pre-read
  reservation, unlike the existing `bytes` gauge.
- `oldest_record_timestamp_ns{shard}`: commit timestamp of the oldest retained
  committed record.
- `oldest_record_age_ns{shard}`: sampled age of the oldest retained committed
  record, refreshed on commit/release and worker peek/drain.

Existing gauges remain:

- `bytes{shard}`: retained ring bytes, including direct in-flight reservation
  bytes when direct-reserve mode is active.
- `slots{shard}`: fixed slots or compact records retained/reserved.
- `packets{shard}`: committed packets currently retained.

Backpressure counters:

- `blocked_ns{shard}` now covers both fixed/direct reservation blocking and
  compact append blocking.
- `stats{shard,stat:blocked_reservations}` increments when fixed/direct
  reservation blocks.
- `stats{shard,stat:blocked_appends}` increments when compact scratch-copy
  append blocks.
- `stats{shard,stat:backpressure_events}` increments for either blocked shape.

Verification:

```bash
dda inv test --targets=./comp/dogstatsd/packets,./comp/dogstatsd/listeners,./comp/dogstatsd/server/impl --timeout=300
```

Result: `315 total`, all passed locally.

Next SMP analysis should graph throughput against `consumer_lag_*`,
`oldest_record_age_ns`, and `blocked_ns/backpressure_events` so higher ingress
rates are not accepted if the raw ring is permanently full or oldest age grows
without bound.

### Initial post-telemetry honesty gates

A new optimized Linux image was built after the telemetry commit:

- `datadog/agent-dev:smp-dsd-columnar-v3-raw-lag-telemetry`
- image ID `sha256:d6432a0a29cb88aa1a5918476debb1c0a722515a8d65ce597230563b1b9292bb`
- agent version `Agent 7.81.0-devel - Meta: git.76.b99255e - Commit: b99255eb31e`

First three-replicate local `main` honesty gates:

| Comparison | Case | Δ mean | Δ mean CI | Confidence | Read |
|---|---|---:|---:|---:|---|
| `main` -> Stage Q + raw lag telemetry | high-rate fixed-v3 local case | +3.62% | [+3.35%, +3.90%] | 100.0% | still positive, but lower than the earlier single-replicate +8.10%; ring applies backpressure |
| `main` -> Stage Q + raw lag telemetry | standard fixed-v3 local case | +2.25% | [+1.94%, +2.55%] | 100.0% | standard case positive, memory remains higher |

Selected three-replicate means:

| Case | Variant | Agent UDS MiB/s | processed/s | packet pool avg | raw consumer lag avg/max | oldest age avg/max | raw blocked ms/s | RSS MiB | heap MiB |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| high-rate v3 | main | 194.01 | 239,790 | 676 | n/a | n/a | n/a | 176.44 | 72.86 |
| high-rate v3 | Stage Q + telemetry | 200.96 | 248,349 | 0 | 9.16/15.87 MiB, 2,472/4,309 records | 89.7/196.3 ms | 406.1 | 179.38 | 73.22 |
| standard v3 | main | 96.38 | 111,089 | 117 | n/a | n/a | n/a | 459.05 | 226.88 |
| standard v3 | Stage Q + telemetry | 98.32 | 113,333 | 0 | 0.39/7.69 MiB, 103/2,118 records | 4.0/276.2 ms | 152.3 | 499.83 | 247.69 |

Read: the telemetry does its job. The high-rate case is now clearly a bounded
backpressure run: packet-pool backlog is gone, the raw ring averages ~9 MiB and
occasionally approaches the 16 MiB budget, and listener append blocking is
visible. This is production-shaped backpressure rather than a free throughput
win. The standard case remains throughput-positive but keeps the memory concern
front and center.

### Remaining three-replicate honesty matrix

The same telemetry image was then used for the remaining local honesty matrix.
Artifacts are under
`reports/smp/dogstatsd-agg-serde-20260516-143205/`:

- `honesty3_matrix_effects.csv`
- `honesty3_matrix_selected_metrics.csv`
- local-only case directories:
  - `local-experiment-final-v2/`
  - `local-experiment-final-origin-v3/`
  - `local-experiment-udp-v3-raw-disabled/`
  - `local-experiment-mixed-types-v3/`

Three-replicate results vs `datadog/agent-dev:smp-dsd-main`:

| Comparison | Δ mean | Δ mean CI | Confidence | Read |
|---|---:|---:|---:|---|
| v3 high-rate UDS, origin off | +3.62% | [+3.35%, +3.90%] | 100.0% | positive but backpressure-bounded |
| v3 standard UDS, origin off | +2.25% | [+1.94%, +2.55%] | 100.0% | positive; memory higher |
| v2/direct-series high-rate UDS, origin off | +3.39% | [+3.08%, +3.70%] | 100.0% | positive but backpressure-bounded |
| v2/direct-series standard UDS, origin off | +1.51% | [+1.22%, +1.80%] | 100.0% | positive; memory higher |
| v3 high-rate UDS, origin on | +2.32% | [+2.03%, +2.61%] | 100.0% | raw ring disables itself; columnar/native remains positive |
| v3 standard UDS, origin on | +1.40% | [+1.03%, +1.77%] | 100.0% | raw ring disables itself; columnar/native remains positive |
| v3 high-rate UDP, raw disabled | -0.06% | [-0.28%, +0.17%] | 25.4% | neutral; raw ring disables itself because UDP is enabled |
| v3 standard UDP, raw disabled | +0.06% | [-0.38%, +0.51%] | 14.3% | neutral; raw ring disables itself because UDP is enabled |
| v3 standard UDS, mixed metric types | -0.01% | [-0.07%, +0.06%] | 10.6% | neutral; unsupported metric types fall back to the legacy aggregator |

Selected means:

| Case | Variant | Agent ingress MiB/s | processed ok/s | packet pool avg | raw lag avg/max | oldest age avg/max | raw blocked ms/s | RSS MiB | heap MiB |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|
| v2 high-rate UDS | main | 195.76 UDS | 241,944 | 818 | n/a | n/a | n/a | 182.27 | 73.88 |
| v2 high-rate UDS | telemetry design | 202.45 UDS | 250,178 | 0 | 9.05/15.85 MiB | 88.8/190.4 ms | 410.1 | 184.32 | 74.10 |
| v2 standard UDS | main | 96.85 UDS | 118,473 | 45 | n/a | n/a | n/a | 456.21 | 223.13 |
| v2 standard UDS | telemetry design | 98.16 UDS | 120,070 | 0 | 0.77/7.67 MiB | 5.6/139.1 ms | 153.5 | 497.27 | 244.63 |
| origin-on v3 high-rate | main | 186.98 UDS | 231,100 | 378 | n/a | n/a | n/a | 165.86 | 65.35 |
| origin-on v3 high-rate | telemetry design, raw disabled | 191.33 UDS | 236,464 | 626 | n/a | n/a | n/a | 158.10 | 58.59 |
| origin-on v3 standard | main | 95.58 UDS | 116,915 | 142 | n/a | n/a | n/a | 491.91 | 251.04 |
| origin-on v3 standard | telemetry design, raw disabled | 97.06 UDS | 118,725 | 85 | n/a | n/a | n/a | 505.07 | 253.71 |
| UDP v3 high-rate | main | 8.84 UDP | 11,042 | 59 | n/a | n/a | n/a | 185.16 | 72.55 |
| UDP v3 high-rate | telemetry design, raw disabled | 12.03 UDP | 15,010 | 85 | n/a | n/a | n/a | 201.71 | 79.50 |
| UDP v3 standard | main | 4.05 UDP | 4,939 | 46 | n/a | n/a | n/a | 197.32 | 78.36 |
| UDP v3 standard | telemetry design, raw disabled | 4.67 UDP | 5,691 | 43 | n/a | n/a | n/a | 209.89 | 81.82 |
| mixed metric types v3 standard | main | 98.67 UDS | 115,764 | 18 | n/a | n/a | n/a | 191.43 | 76.01 |
| mixed metric types v3 standard | telemetry design | 98.79 UDS | 115,901 | 0 | 1.67/8.03 MiB | 11.4/34.9 ms | 140.9 | 198.19 | 77.44 |

Important caveats:

- The origin-on runs validate the columnar/native path with raw ingress disabled;
  they do not validate raw-ring origin metadata support.
- The UDP runs are raw-disabled compatibility checks. Lading still writes at the
  requested byte rate, while the Agent processes only a small fraction of the UDP
  datagrams under this local setup; do not use these as UDP throughput proof.
- The mixed-metric case is intentionally broader than the columnar fast path.
  The telemetry design falls back for unsupported metric types at about
  35k/s while remaining throughput-neutral; this is compatibility evidence, not
  a native mixed-type speedup.
- Standard UDS cases remain memory-negative. Memory profiling, descriptor expiry,
  row/buffer lifetime, and bounded metadata strategy are still blockers before a
  production switch.

## Stage R: memory hygiene and telemetry cost controls

Stage R turns the standard-UDS memory concern into an explicit optimization
stage. The main principle is that proof telemetry is valuable, but it must be
measurable and optional when it is not needed for the production-shaped hot path.

Implementation commits:

- `f05e7fa378f` — `dogstatsd: reduce columnar memory overhead`
- `cf85e2601ce` — `dogstatsd: make columnar descriptor interning optional`
- `ab6db258799` — `dogstatsd: reuse columnar flush buffers`

Final Stage R image:

- `datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-reuse`
- image ID `sha256:f0594de5eb3e9db47bfba9ef41ff3c999dc6cb58be2a396632de8237561ad394`

Changes:

- Direct-row proof telemetry is optional behind
  `DD_DOGSTATSD_EXPERIMENTAL_DIRECT_ROW_SHADOW_TELEMETRY=true`.
- Serializer segment-shadow proof telemetry is optional behind
  `DD_SERIALIZER_EXPERIMENTAL_SEGMENT_SHADOW_TELEMETRY=true`.
- Columnar descriptors expire/reuse slots using `dogstatsd_context_expiry_seconds`
  by default; local override:
  `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_DESCRIPTOR_EXPIRY_SECONDS`.
- Per-bucket descriptor maps are no longer allocated on the normal monotonic
  bucket path. They are built lazily only for non-monotonic descriptor/bucket
  fallback.
- Flush row merging now uses descriptor-ID generation arrays instead of a
  per-flush map keyed by columnar identity.
- First-pass descriptor string/tagset interning exists, but is opt-in via
  `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_INTERN_DESCRIPTORS=true`; SMP showed
  that naive default-on interning is not good for mostly-unique tagsets.
- Columnar bucket structs and flush row slices are reused to reduce churn.

Artifacts:

- `reports/smp/dogstatsd-agg-serde-20260516-143205/notes/stageR-memory-hygiene.md`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/stageR_memory_hygiene_effects.csv`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/stageR_memory_hygiene_selected_metrics.csv`

Three-replicate SMP results:

| Comparison | Case | Δ mean | Δ mean CI | Confidence | Read |
|---|---|---:|---:|---:|---|
| raw-lag telemetry -> Stage R interning-on | v3 standard UDS | +0.23% | [+0.03%, +0.43%] | 85.8% | throughput neutral/slightly positive, but RSS worse; naive interning is not default-worthy |
| raw-lag telemetry -> Stage R default-off interning | v3 standard UDS | +0.13% | [-0.11%, +0.38%] | 50.9% | throughput neutral; shadow telemetry removed; paired RSS lower |
| Stage R default-off -> Stage R buffer reuse | v3 standard UDS | -0.09% | [-0.21%, +0.03%] | 65.9% | throughput neutral; RSS noisy, heap-sys/allocation-rate signal mixed |
| raw-lag telemetry -> final Stage R reuse | v3 high-rate UDS | +1.50% | [+1.22%, +1.78%] | 100.0% | high-rate path improved, still backpressure-bounded |
| main -> final Stage R reuse | v3 standard UDS | +2.22% | [+1.92%, +2.52%] | 100.0% | throughput-positive vs main, but total RSS/heap remain materially higher |

Selected means:

| Comparison | Variant | ingress MiB/s | RSS MiB | heap alloc MiB | heap sys MiB | shadow rows/s | raw lag MiB | blocked ms/s |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| raw-lag -> Stage R default | raw-lag telemetry | 97.99 | 502.02 | 250.53 | 601.70 | direct 148.86 / segment 186.15 | 0.98 | 148.67 |
| raw-lag -> Stage R default | Stage R default | 98.15 | 491.09 | 243.22 | 588.36 | direct 0 / segment 0 | 0.84 | 139.30 |
| main -> final Stage R reuse | main | 96.38 | 453.27 | 224.92 | 579.00 | 0 / 0 | n/a | 0 |
| main -> final Stage R reuse | final Stage R reuse | 98.40 | 498.83 | 246.37 | 612.41 | 0 / 0 | 0.57 | 142.72 |
| raw-lag -> final Stage R reuse | raw-lag telemetry high-rate | 200.86 | 178.56 | 73.37 | 130.46 | direct 148.82 / segment 184.79 | 9.02 | 393.98 |
| raw-lag -> final Stage R reuse | final Stage R reuse high-rate | 203.85 | 180.12 | 74.63 | 134.59 | direct 0 / segment 0 | 9.12 | 395.10 |

Read:

- The expensive proof telemetry is now explicit cost, not hidden cost. It can be
  turned back on for equivalence/cost probes, but it is off by default for normal
  SMP honesty measurements.
- Naive database-style tagset interning is not enough. In this workload, tagset
  reuse was too low (`~7.68` created tagsets/s vs `~0.16` reused tagsets/s), so
  default-on interning simply added maps/key strings. Keep the idea, but require
  bounded/adaptive admission before making it part of the default path.
- The columnar bucket path is now more database-shaped: descriptor ID is the
  primary key, per-bucket maps are lazy fallback, and flush merging is by
  descriptor generation. SMP observed zero fallback bucket-index creation in the
  measured UDS runs.
- Final Stage R is still not a memory win vs main. Standard v3 UDS remains about
  `+45.6 MiB` RSS and `+21.5 MiB` heap alloc vs main in the paired run, despite
  removing legacy aggregator contexts/tagstore memory. The next memory work must
  profile the direct columnar/serializer path rather than assuming descriptor
  metadata is the whole delta.

## Stage S: heap profiling and v3 serializer allocation analysis

Stage S used pprof to explain the remaining Stage R memory gap and then fixed
low-risk direct serializer allocation artifacts.

Artifacts:

- `reports/smp/dogstatsd-agg-serde-20260516-143205/notes/stageS-heap-and-v3-serializer-profile.md`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/profiles/stageR-agent-heap/manual/`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/profiles/stageR-v3-payload-builder/`

Profile setup:

- Baseline image: `datadog/agent-dev:smp-dsd-main`
- Comparison image: `datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-reuse`
- Case: `uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch`
- One local standard-UDS replicate with `--total-samples 150`.
- Manual heap collection was required because SMP's pprof sidecar cannot reach the
  Agent's `127.0.0.1`-bound expvar server; profiles were collected via an
  `alpine` container sharing the target container network namespace.

Key heap findings:

| Variant | late pprof in-use heap |
|---|---:|
| main | `162.14 MiB` |
| Stage R reuse | `173.31 MiB` |
| delta | `+11.17 MiB` |

This pprof in-use delta is much smaller than the paired SMP RSS delta
(`~+45.6 MiB`). The remaining RSS gap is therefore not mostly retained live Go
objects from the direct serializer. It is more consistent with heap-sys/RSS
retention from transient allocation, payload/column buffers, allocator behavior,
and non-heap/runtime memory.

Late in-use shape:

- `main` retained about `32.74 MiB` in the legacy packet pool.
- Stage R retained about `18.72 MiB` in the compact raw ring.
- Stage R retained about `+5.39 MiB` more parser string-interner data and
  `+3.03 MiB` in the columnar sample pool.
- Direct v3 serializer objects were not a top retained-heap item.

Allocation-space shape:

- Whole-Agent allocation is dominated by parser string interning and tag parsing:
  `stringInterner.LoadOrStore` allocated `~17.64 GiB` in the Stage R late
  profile and `parseTags` accounted for `~18.38 GiB` cumulative allocation.
- Serializer-focused cumulative allocation over the same 150-sample run was much
  smaller: columnar-v3 flush to direct point rows accounted for `~64 MiB`, with
  `~21 MiB` in column input buffers, `~12 MiB` in v3 dictionary string maps, and
  `~8 MiB` in final payload buffers.

Code changes from the profile:

- `metrics.V3MetricPointRowSink` now accepts `*V3MetricPointRow`; the pointer is
  documented as call-scoped.
- Columnar flush passes pointers to scratch rows synchronously and then clears
  them after the sink returns.
- Direct serializer callback and sink paths no longer copy a point row by value
  and then take its address. The pre-fix Agent profile showed about `5.5 MiB`
  cumulative allocation in `directSeriesCallbackSink.AppendV3MetricPointRow` and
  about `5.0 MiB` in `DirectSeriesSink.AppendV3MetricPointRow` from those row
  escapes during the 150-sample run.
- `payloadsBuilderV3.writeSerie` now has a no-mutation fast path for the common
  no-special-resource-tags case, avoiding one temporary `SerieRow` escape per
  series. It keeps the `SerieRowFromSerie` compatibility fallback for `device:`
  and `dd.internal.resource:` tags.
- Added `BenchmarkV3PayloadBuilderAllocation` for future focused allocation
  checks.

Focused benchmark read for 8,192-row flush-shaped batches:

| Path | Identity shape | Before | After |
|---|---|---:|---:|
| `writeSerie` | reused identity | `~2.41 MiB`, `8,743 allocs/op` | `~704 KiB`, `551 allocs/op` |
| `writeSerie` | unique identity | `~6.95 MiB`, `9,123 allocs/op` | `~5.25 MiB`, `929 allocs/op` |
| `writeV3MetricPointRow` | unique identity | `~5.25 MiB`, `929 allocs/op` | unchanged; already at this benchmark's payload-builder floor |

Validation:

```bash
dda inv test --targets=./pkg/metrics,./pkg/aggregator,./pkg/serializer/internal/metrics,./pkg/serializer --timeout=300
```

Result: passed (`400` tests).

Next memory work:

- Build and SMP-test a post-Stage-S image.
- Run metric-only heap profiles to remove standard-case event/service-check noise.
- Focus on the parser string interner next: measure hit/miss/reset/bytes against
  workload cardinality and test bounded/adaptive admission for mostly-unique
  tags.
- Consider v3 builder capacity hints or a batched point-row sink so dictionary
  maps and column buffers can be sized from flush row estimates.

## Stage T: parser string/tagset interning

Stage T targets the dominant parser allocation identified in Stage S. In the
standard v3 UDS Stage R profile, `stringInterner.LoadOrStore` accounted for
roughly `17.64 GiB` allocation-space and `parseTags` allocated roughly
`2.40 GiB` for per-message `[]string` slices. Telemetry from the same run showed
both parser workers hit `~53M` interner misses and `12,936` full resets against a
`4096`-entry cache.

Implementation:

- Replace the parser string interner's full-reset map with a bounded SLRU-style
  dictionary:
  - first sightings enter a recent/probationary segment;
  - a hit in recent promotes the string to protected;
  - both segments use ring eviction;
  - hits keep the no-allocation `map[string]...` lookup form with
    `string([]byte)` directly in the index expression;
  - existing hit/miss/size/bytes telemetry remains;
  - new `dogstatsd.string_interner_evictions` counts individual evictions;
  - `dogstatsd.string_interner_resets` should stop increasing for the new path.
- Make `extractTagsMetadata` non-mutating for normal tagsets. It now rewrites the
  tag slice only when it actually removes metadata tags such as `host:`,
  `dd.internal.entity_id:`, `dd.internal.card:`, or `dd.internal.jmx_check_name:`.
- Add an opt-in exact raw-tagset interner:
  - `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true`
  - `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER_SIZE=<entries>`
  - The cache uses a doorkeeper hash so first sightings are not admitted; second
    sightings admit the parsed exact tagset.
  - Tagsets with metadata tags are not admitted.
  - Per-hit telemetry was intentionally avoided after benchmarking showed it
    reintroduced allocation; macro validation should use pprof/SMP metrics.

Focused microbenchmark:

```bash
dda inv test --targets=./comp/dogstatsd/server/impl \
  --test-run-name='^$' \
  --extra-args='-bench=BenchmarkParseTagsRepeatedTagset -benchmem -benchtime=3s' \
  --timeout=300
```

| Path | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| default repeated tagset parse | `145.9` | `112` | `1` |
| exact tagset interner hit | `15.64` | `0` | `0` |

Validation:

```bash
dda inv test --targets=./comp/dogstatsd/server/impl,./comp/dogstatsd/listeners,./comp/dogstatsd/packets --timeout=300
```

Result: passed (`318` tests).

Artifacts:

- `reports/smp/dogstatsd-agg-serde-20260516-143205/notes/stageT-parser-interning.md`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/profiles/stageT-parser-tagset-bench-nohottelemetry.txt`

Next SMP work:

1. Build a post-Stage-T image and compare default Stage T vs Stage R/S to isolate
   the SLRU string interner.
2. Run the same image with
   `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true` to isolate exact
   tagset-cache feature cost.
3. Re-run main comparisons only after those feature-cost probes show a favorable
   allocation/RSS/throughput tradeoff.

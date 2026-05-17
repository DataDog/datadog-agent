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
is not sufficient proof to switch output. Stage E remains gated on a semantic
row sink that either accounts for sink-level host tags or moves observation to a
post-enrichment boundary, and on a builder that can emit the actual v3 wire
payload from rows without falling back to the existing `metrics.Serie` path.

### Stage E: direct path as comparison output

Only after Stage D is semantically clean, allow the experiment image to send the
new path's payload while optionally keeping old path as shadow.

Current local status: **deferred after Stage D**. The direct row observer is
output-neutral and SMP-neutral, but it is not yet a semantically complete output
path because it still shadows around `metrics.Serie` and does not yet prove exact
post-sink enrichment/wire equivalence. Do not run Stage E until that design gap
is closed.

Run foundation vs experiment on the main cases again.

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

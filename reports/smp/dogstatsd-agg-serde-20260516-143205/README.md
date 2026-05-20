# DogStatsD Aggregator + Serializer SMP Experiment Report

Status: Stage A-U complete locally; raw-ring telemetry, the follow-up three-replicate honesty matrix, Stage R memory-hygiene probes, Stage S heap/profile analysis, Stage T parser interning, and Stage U compact identity hints are complete. Single-page report: [`report.html`](report.html).

## Experiment intent

Collect local SMP evidence for whether the DogStatsD streaming-foundation work can support a significant core aggregator + serializer improvement.

Hypotheses:

1. The foundation stack is neutral or beneficial under existing DogStatsD SMP cases.
2. Instrumentation needed to understand aggregator/serializer costs can stay neutral enough to guide future work.
3. Payload-aligned/semantic-row prototypes can remain output-neutral before any live serializer or aggregator hot-path switch.
4. A deliberately unsafe local direct serializer switch can estimate end-goal performance by bypassing `IterableSeries` / `IterableSketches` channels and writing producer rows directly into serializer pipeline builders.
5. A follow-on direct row switch can move DogStatsD time-sampler series handoff to serializer-visible rows without reintroducing SMP regressions.
6. A deliberately narrow v3-only, metric-only columnar vertical slice can test the fuller idea: parser path → shard-local columnar table → direct v3 row payload construction, bypassing TimeSampler/ContextMetrics/metrics.Metric/metrics.Serie for supported samples.
7. A final native serializer stage can test whether v3-aligned columnar aggregation makes payload construction cheap, while a direct-series/v2 bridge checks that the shape is not v3-only.

## Images under test

Images were built locally as optimized Linux/arm64 Agent images with `dda inv agent.hacky-dev-image-build --no-development` inside `datadog/agent-dev-env-linux:latest`.

- Baseline main: `datadog/agent-dev:smp-dsd-main` — commit `3ec880f14a3`
- Foundation: `datadog/agent-dev:smp-dsd-foundation` — commit `53f7e8fdc3d`
- Stage B instrumentation: `datadog/agent-dev:smp-dsd-experiment` — commit `538ae360d89`
- Stage C shadow segments: `datadog/agent-dev:smp-dsd-shadow` — commit `911b22716ca`
- Stage D direct row shadow: `datadog/agent-dev:smp-dsd-direct-row` — commit `e3f2f987056`
- Stage E direct active serializer: `datadog/agent-dev:smp-dsd-direct-active` — commit `9498d5fee95`
- Stage F direct series rows: `datadog/agent-dev:smp-dsd-direct-rows` — commit `c7327ad816c`
- Stage G direct metric rows: `datadog/agent-dev:smp-dsd-direct-metric-rows` — commit `638b79c3bba`
- Stage H unordered direct context rows: `datadog/agent-dev:smp-dsd-direct-context-rows` — commit `c989043ddcc`
- Stage I columnar v3 naive: `datadog/agent-dev:smp-dsd-columnar-v3` — commit `2ade84801cc`
- Stage I columnar v3 merged: `datadog/agent-dev:smp-dsd-columnar-v3-merged` — commit `9b04b0c6104`
- Stage I columnar v3 descriptor reuse: `datadog/agent-dev:smp-dsd-columnar-v3-descriptors` — commit `0e774a353cb`
- Stage I columnar v3 deferred telemetry: `datadog/agent-dev:smp-dsd-columnar-v3-lite-telemetry` — commit `6f3c5ae857a`
- Stage I columnar v3 batched workers: `datadog/agent-dev:smp-dsd-columnar-v3-batched` — commit `7a43a9d0dae`
- Stage I columnar v3 batched/no-lock: `datadog/agent-dev:smp-dsd-columnar-v3-batched-nolock` — commit `87fbbc1fbb5`
- Stage I columnar v3 bucket cache: `datadog/agent-dev:smp-dsd-columnar-v3-bucket-cache` — commit `e68c3c36110`
- Stage J columnar v3 shard-only identity: `datadog/agent-dev:smp-dsd-columnar-v3-shard-only` — commit `dfdb011f0d7`
- Stage J skip legacy flush probe: `datadog/agent-dev:smp-dsd-columnar-v3-skip-legacy` — commit `4a5bbb1fbce`
- Stage L columnar v3 + experimental ingress log code, env off: `datadog/agent-dev:smp-dsd-columnar-v3-ingress-log` — commits `eae5cb364ee` / follow-up `7630c423017` (SMP image was built before the follow-up)
- Stage L columnar v3 + experimental ingress log enabled: `datadog/agent-dev:smp-dsd-columnar-v3-ingress-log-enabled` — commits `eae5cb364ee` / follow-up `7630c423017` (SMP image was built before the follow-up), env `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG=true`, `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES=16777216`
- Stage M sharded packet-batch ingress log: `datadog/agent-dev:smp-dsd-columnar-v3-sharded-log` — commit `53a887497f6`, env `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_SHARDED=true`, `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES=16777216`
- Stage M raw UDS ingress ring: `datadog/agent-dev:smp-dsd-columnar-v3-raw-uds-ring` — commit `53a887497f6`, env `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS=true`, `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES=16777216`
- Stage N compact raw UDS ingress ring: `datadog/agent-dev:smp-dsd-columnar-v3-compact-raw-uds-ring` — commit `4b2c4b6b7e6`, env `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_COMPACT=true`, `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_LOG_MAX_BYTES=16777216`
- Stage O batched compact raw UDS ring drains: `datadog/agent-dev:smp-dsd-columnar-v3-compact-raw-uds-ring-batch-drain` — commit `dc29e8fff7c`, env `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_BATCH_DRAIN=true`, `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_BATCH_DRAIN_SIZE=32`
- Stage P direct-compact raw UDS ring: `datadog/agent-dev:smp-dsd-columnar-v3-direct-compact-raw-uds-ring` — commit `83d06509167`, env `DD_DOGSTATSD_EXPERIMENTAL_INGRESS_RING_UDS_DIRECT_COMPACT=true`
- Stage Q native columnar-v3 serializer: `datadog/agent-dev:smp-dsd-columnar-v3-native-columnar-v3` — commit `947dc3f2ec3`, env `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_NATIVE_SERIALIZER=true` or `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_DIRECT_SERIES_SERIALIZER=true` for the v2/direct-series bridge
- Post-Stage-Q raw-ring lag telemetry: `datadog/agent-dev:smp-dsd-columnar-v3-raw-lag-telemetry` — commit `b99255eb31e`, image ID `sha256:d6432a0a29cb88aa1a5918476debb1c0a722515a8d65ce597230563b1b9292bb`
- Stage R memory hygiene: `datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-reuse` — commit `ab6db258799`, image ID `sha256:f0594de5eb3e9db47bfba9ef41ff3c999dc6cb58be2a396632de8237561ad394`
- Stage T parser interning default: `datadog/agent-dev:smp-dsd-columnar-v3-parser-interning` — commit `d78e87470fd`, image ID `sha256:9bce78118472178a9d815f9bd7a8e50c80b46d4cf92f502969a2b29d66ab3012`
- Stage T parser interning + exact tagset cache: `datadog/agent-dev:smp-dsd-columnar-v3-parser-interning-tagset` — commit `d78e87470fd`, image ID `sha256:99c37921c6bed2145e1a848499a8e83885f5ee5bfff23601da0f47890c00b93b`, env `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true`
- Stage U compact identity hints, env off: `datadog/agent-dev:smp-dsd-columnar-v3-compact-identity-hints` — commit `9376b73a9c1`, image ID `sha256:ab07f3b9d15e12a13ba116484d5ec60587c9233bc8468977b809ad45dc11529d`
- Stage U exact tagset only: `datadog/agent-dev:smp-dsd-columnar-v3-compact-identity-hints-tagset` — commit `9376b73a9c1`, image ID `sha256:070cbafd961ff188741a94c2c0c5c16961c1fde0ff4d89eec1bf3a3bfda264b2`, env `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true`
- Stage U exact tagset + compact identities: `datadog/agent-dev:smp-dsd-columnar-v3-compact-identity-hints-enabled` — commit `9376b73a9c1`, image ID `sha256:3ebd8cc669b1f6c8c31a831e9972b81b6bd8d454fb9e77cf1fe1d665eabd6d05`, env `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true`, `DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES=true`

See [`images.txt`](images.txt) for image IDs and full version details.

## High-level results

Stage A-J completed comparisons used local SMP with `--replicates 3 --total-samples 270`; Stage L/M/N/O/P/Q ingress/backpressure/serializer probes are single-replicate local runs (`--replicates 1 --total-samples 150`). Post-telemetry honesty gates and Stage R/T/U probes used `--replicates 3 --total-samples 270`.

### Stage A — main vs foundation

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---:|---:|---:|---:|---:|---:|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.19% | [-0.46%, +0.07%] | 64.6% | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.18% | [-0.38%, +0.01%] | 76.4% | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | +0.70% | [+0.47%, +0.94%] | 100.0% | false | false |

Verdict: foundation is SMP-neutral for these primary DogStatsD cases.

### Stage B — foundation vs instrumentation-only

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---:|---:|---:|---:|---:|---:|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.29% | [-0.55%, -0.02%] | 83.2% | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.69% | [-0.96%, -0.41%] | 99.9% | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | +0.77% | [+0.56%, +0.99%] | 100.0% | false | false |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.04% | [-0.23%, +0.31%] | 13.6% | false | true |

Verdict: instrumentation is neutral enough to keep as the measurement substrate. The source `uds_dogstatsd_to_api_v3` case did not activate current metrics-v3 endpoint config, so a corrected local v3 endpoint case was added.

### Stage C — instrumentation vs shadow payload-aligned segment builder

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---:|---:|---:|---:|---:|---:|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.19% | [-0.38%, +0.01%] | 78.1% | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.35% | [-0.57%, -0.14%] | 96.3% | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | +0.72% | [+0.47%, +0.96%] | 100.0% | false | false |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | -0.04% | [-0.32%, +0.24%] | 13.3% | false | false |

Verdict: shadow segment construction is SMP-neutral and useful for payload-shape telemetry, but it still runs after the current path and does not prove CPU savings.

### Stage D — shadow segments vs direct aggregator row shadow

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---:|---:|---:|---:|---:|---:|
| `uds_dogstatsd_to_api_v3` | ingress throughput | +0.46% | [+0.18%, +0.74%] | 96.2% | false | true |
| `uds_dogstatsd_to_api` | ingress throughput | -0.43% | [-0.65%, -0.22%] | 99.1% | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | +0.30% | [+0.09%, +0.52%] | 93.4% | false | false |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.16% | [-0.04%, +0.36%] | 70.6% | false | true |

Verdict: direct row observation is neutral enough to keep. It is still only an observer and does not remove serializer traversal.

### Stage E — direct row shadow vs direct active serializer

Stage E enables `DD_DOGSTATSD_EXPERIMENTAL_DIRECT_SERIALIZER=true` in local-only case copies. The Stage E comparison image bypasses `IterableSeries` / `IterableSketches` channel traversal and writes producer rows directly into v2/v3 serializer pipeline builders; the Stage D baseline ignores the env var and uses the previous path.

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---:|---:|---:|---:|---:|---:|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.21% | [-0.43%, +0.01%] | 77.0% | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.07% | [-0.39%, +0.24%] | 23.6% | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | -0.94% | [-1.15%, -0.72%] | 100.0% | false | true |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.56% | [+0.27%, +0.84%] | 98.7% | false | true |

Verdict: active direct serialization remains non-regressing locally and improves the corrected current-v3 endpoint case and the memory case. It does not show a broad throughput win on the legacy/source v3-labeled or v2 API cases. This suggests the first end-goal switch is promising mainly where the current v3 payload builder is actually active, while additional aggregator-side work is still needed for a large general speedup.

### Stage F — direct active serializer vs direct series rows

Stage F adds `DD_DOGSTATSD_EXPERIMENTAL_DIRECT_ROWS=true` on top of Stage E. DogStatsD `TimeSampler.flushSeries` emits normalized `metrics.SerieRow` values to a direct row sink instead of fully populated `*metrics.Serie` objects for the time-sampler series path. Check-sampler series and sketches still use the older structs.

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---:|---:|---:|---:|---:|---:|
| `uds_dogstatsd_to_api_v3` | ingress throughput | +0.22% | [-0.03%, +0.47%] | 75.0% | false | true |
| `uds_dogstatsd_to_api` | ingress throughput | -0.13% | [-0.36%, +0.10%] | 54.0% | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | -0.21% | [-0.44%, +0.01%] | 77.6% | false | true |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | -0.12% | [-0.37%, +0.13%] | 45.7% | false | false |

Verdict: direct DogStatsD series rows are locally non-regressing and essentially neutral. This moves more core time-sampler handoff into the new row model, but it still does not remove metric flush/dedup materialization or convert sketches/check-sampler rows.

### Stage G — direct series rows vs direct metric rows

Stage G adds `DD_DOGSTATSD_EXPERIMENTAL_DIRECT_METRIC_ROWS=true` on top of Stage F. It bypasses `*metrics.Serie` allocation during scalar `Metric.flush` and emits lightweight row fragments into the direct row handoff.

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---:|---:|---:|---:|---:|---:|
| `uds_dogstatsd_to_api_v3` | ingress throughput | -0.08% | [-0.35%, +0.19%] | 29.3% | false | false |
| `uds_dogstatsd_to_api` | ingress throughput | -0.00% | [-0.24%, +0.24%] | 1.3% | false | false |
| `uds_dogstatsd_20mb_12k_contexts_20_senders` | memory utilization | -0.52% | [-0.76%, -0.28%] | 99.4% | false | true |
| `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.02% | [-0.23%, +0.26%] | 6.8% | false | true |
| `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -0.25% | [-0.48%, -0.01%] | 82.2% | false | false |

Verdict: direct metric rows reduce memory in the memory case but do not improve throughput. The high-rate metrics-only probe avoids the standard 100 MiB/s generator cap and is slightly negative, so `*metrics.Serie` allocation is not the main value lever.

### Stage H — direct metric rows vs unordered direct context rows

Stage H adds `DD_DOGSTATSD_EXPERIMENTAL_DIRECT_CONTEXT_ROWS=true` as an unsafe upper-bound probe. It bypasses context grouping/dedup and emits rows in timestamp/map iteration order, so it is not wire-equivalent.

| Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---:|---:|---:|---:|---:|---:|
| `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -0.51% | [-0.73%, -0.29%] | 99.7% | false | false |

Verdict: the upper-bound shortcut is worse. Removing grouping/dedup naively increases downstream row/payload work more than it saves.

### Stage I — direct metric rows vs columnar v3 vertical slice

Stage I adds `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3=true` as a local-only, v3-only, metric-only vertical slice. Supported on-time gauges/counters/counts/sets bypass the legacy DogStatsD time sampler and are flushed from a shard-local columnar table into direct v3 series rows. The slice still uses the normal parser/enrichment path and still keeps legacy fallbacks for unsupported samples/checks/sketches/events.

| Variant | Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---|---:|---:|---:|---:|---:|---:|
| naive parser direct insert | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -4.74% | [-5.12%, -4.35%] | 100.0% | false | false |
| merged flush rows | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -5.28% | [-5.63%, -4.93%] | 100.0% | false | false |
| descriptor reuse | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -6.68% | [-6.96%, -6.40%] | 100.0% | false | false |
| deferred insert telemetry | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -6.02% | [-6.28%, -5.77%] | 100.0% | false | false |
| batched shard workers | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -8.02% | [-8.27%, -7.76%] | 100.0% | false | false |
| batched/no-lock shard workers | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -8.61% | [-8.86%, -8.37%] | 100.0% | false | false |
| bucket-row cache | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -7.11% | [-7.44%, -6.78%] | 100.0% | false | false |

Proof telemetry confirmed the intended bypass in smoke/full runs: old aggregator contexts stayed effectively zero, `dogstatsd_columnar_v3.stats{stat:inserted_samples}` was >200k/s, and columnar flush rows were emitted directly to the v3 row serializer. However, every Stage I variant was slower than the Stage G direct-metric-row baseline, with much higher DogStatsD packet backlog/RSS under the 250MiB metrics-only load.

Verdict: this narrow columnar/v3 vertical slice does not expose a throughput win. It is valuable negative evidence: simply replacing the time-sampler metric state with a columnar table plus direct v3 row output is not enough, and parser/ingest backpressure dominates before any serializer-side savings can pay off.

### Stage J — root-cause probe: avoid unused debug identity in the columnar hot path

Stage J tested the leading Stage I bottleneck hypothesis: the columnar parser path was calling `ResolveHotPath` for every sample even when debug stats were disabled and `dogstatsd_pipeline_count=1`. That computed both the debug projection and the shard/backend key, including an unused debug key and `strings.Join` display-tags string. The old direct-metric-row baseline did not do that parser-side work in the one-pipeline case.

The Stage J fix computes only the shard identity when debug is off. This is still a precomputed identity, but it removes work that was not structurally required by the unified model.

| Comparison | Case | Goal | Δ mean | Δ mean CI | Confidence | Regression | Improvement |
|---|---|---:|---:|---:|---:|---:|---:|
| direct metric rows → columnar v3 shard-only | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | +13.57% | [+13.28%, +13.86%] | 100.0% | false | true |
| columnar bucket-cache → columnar shard-only | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | +24.07% | [+23.57%, +24.57%] | 100.0% | true* | true |
| direct metric rows → columnar v3 shard-only | `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +2.83% | [+2.44%, +3.22%] | 100.0% | false | true |
| columnar shard-only → skip legacy flush | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -0.22% | [-0.57%, +0.14%] | 56.7% | false | false |

`*` SMP mechanically marks `Regression=true` when absolute delta exceeds the ±20% threshold, even though this direction is an improvement for throughput.

Verdict: the theoretical performance case was not disproven by Stage I; the implementation had not actually collapsed identity work. It added an expensive unused debug projection on the parser hot path. Once removed, the narrow columnar slice is a real throughput win on the high-rate metrics-only probe and a small win on the standard corrected v3 endpoint case. The skip-legacy-flush upper bound was neutral, so the remaining hot-path limiter is ingest/aggregation work, not the empty fallback flush.

Open concern: memory is not solved. On the standard corrected v3 endpoint case, columnar shard-only improved throughput by +2.83% and reduced agent CPU by ~4%, but increased RSS by ~30% and heap by ~39%. On the high-rate overload probe, packet/channel backlog still grows substantially. This points to descriptor/bucket/GC/backlog costs that need a separate memory-focused pass before claiming an all-around production win.

### Stage K/L — bottleneck shift and experimental ingress log

Stage K showed that Stage J shifted the bottleneck: the direct-row baseline backpressures before the Agent packet channel, while columnar admits enough additional traffic to expose the large `packetsIn` channel as the overload absorber. Stage L added a local-only byte-bounded ingress log to replace that large channel with explicit backpressure.

Stage L is single-replicate local evidence only:

| Comparison | Case | Goal | Δ mean | Δ mean CI | Read |
|---|---|---:|---:|---:|---|
| columnar env-off → columnar ingress-log | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -7.79% | [-8.42%, -7.16%] | trades unconstrained overload admission for bounded memory |
| direct metric rows → columnar ingress-log | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | +4.90% | [+4.37%, +5.44%] | keeps a high-rate win with backlog/memory near baseline |
| direct metric rows → columnar ingress-log | `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +1.22% | [+0.53%, +1.92%] | standard case remains neutral/slightly positive |

High-rate direct metric rows vs columnar ingress-log selected metrics:

| Variant | Agent UDS MiB/s | processed/s | packet pool avg | ingress log avg | RSS MiB | heap MiB |
|---|---:|---:|---:|---:|---:|---:|
| direct metric rows | 193.23 | 238,830 | 528 | n/a | 247.12 | 63.16 |
| columnar ingress-log | 202.36 | 250,077 | 807 | 8.3 MiB | 228.91 | 58.09 |

Verdict: this supports the raw-ingress-log direction. The first prototype gives up some of Stage J's maximum overload throughput, but it converts hidden queue memory into explicit bounded backpressure and still beats the direct-row baseline in the high-rate probe.

### Stage M — sharded ingress log and raw UDS ingress ring

Stage M tested two ways to remove Stage L's prototype overhead. M1 lets listeners flush packet batches directly into per-worker ingress-log shards. M2 is the core architecture probe: a fixed-slot preallocated UDS datagram ring where listeners read directly into ring storage and workers parse/release ring records.

Stage M is single-replicate local evidence only:

| Comparison | Case | Goal | Δ mean | Δ mean CI | Read |
|---|---|---:|---:|---:|---|
| Stage L ingress-log → M1 sharded packet-batch log | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -3.77% | [-4.54%, -2.99%] | removing the pump/channel did not recover throughput in this run |
| direct metric rows → M1 sharded packet-batch log | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -0.09% | [-0.77%, +0.59%] | high-rate throughput neutral, lower RSS/heap/backlog |
| direct metric rows → M2 raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | +0.80% | [+0.14%, +1.46%] | small high-rate win with zero packet-pool backlog |
| direct metric rows → M2 raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +1.16% | [+0.77%, +1.55%] | standard case remains positive |

High-rate direct metric rows vs raw UDS ring selected metrics:

| Variant | Agent UDS MiB/s | processed/s | packet pool avg | ring avg | RSS MiB | heap MiB |
|---|---:|---:|---:|---:|---:|---:|
| direct metric rows | 197.56 | 244,007 | 1,450 | n/a | 185.29 | 81.39 |
| raw UDS ring | 199.74 | 246,846 | 0 | 3.34 MiB / 913 slots | 157.26 | 61.60 |

Important caveat: early raw runs were invalid until local raw cases set `dogstatsd_port: 0`; raw rows above come from logs where the raw-ring gate was active. A retained `sharded-log vs raw-uds-ring` run is not a clean M1-vs-M2 isolation because both local image tags point at the same raw-capable binary and the raw case env enabled raw in both variants.

Verdict: M2 did not recover the full Stage J overload-throughput win, but it proves the raw-ingress-ring shape can eliminate heap-backed packet-pool backlog for UDS datagrams while remaining slightly faster than direct metric rows. The next ceiling test should replace fixed slots with a compact variable-length/slabbed byte ring and batched notifications/cursors.

### Stage N — compact raw UDS ingress ring

Stage N replaces M2's fixed `dogstatsd_buffer_size` raw slots with a compact byte ring. The listener reads into a reusable scratch buffer, then commit copies exactly the bytes read into a preallocated per-worker byte ring plus metadata ring.

Stage N is single-replicate local evidence only:

| Comparison | Case | Goal | Δ mean | Δ mean CI | Read |
|---|---|---:|---:|---:|---|
| fixed-slot raw UDS ring → compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | +3.92% | [+3.19%, +4.65%] | compact ring beats fixed-slot M2 |
| direct metric rows → compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | +7.25% | [+6.52%, +7.97%] | best bounded-ingress high-rate result so far |
| direct metric rows → compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +2.17% | [+1.61%, +2.72%] | standard case remains positive |
| fixed-slot raw UDS ring → compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.05% | [+0.01%, +0.09%] | standard case neutral versus fixed-slot raw |

High-rate direct metric rows vs compact raw ring selected metrics:

| Variant | Agent UDS MiB/s | processed/s | packet pool avg | ring avg | RSS MiB | heap MiB |
|---|---:|---:|---:|---:|---:|---:|
| direct metric rows | 186.40 | 230,366 | 612 | n/a | 173.39 | 71.67 |
| compact raw ring | 200.10 | 247,216 | 0 | 8.15 MiB / 2,174 records | 163.09 | 63.32 |

Verdict: compact retained records matter. Even with one scratch-to-ring copy, the compact ring recovers a meaningful chunk of high-rate throughput while preserving the main M2 memory/backlog property: no heap-backed packet-pool backlog. Remaining work: batched compact-ring drains, size-class/direct-reservation variants to avoid the copy, origin/OOB support, and oldest-age/lag telemetry.

### Stage O — batched compact raw UDS ring drains

Stage O keeps Stage N's compact byte ring but lets workers peek and release up to 32 raw records per shard lock acquisition.

Stage O is single-replicate local evidence only:

| Comparison | Case | Goal | Δ mean | Δ mean CI | Read |
|---|---|---:|---:|---:|---|
| compact raw UDS ring → batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | +1.77% | [+1.23%, +2.31%] | batched worker drains help under high offered load |
| direct metric rows → batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | +7.39% | [+6.74%, +8.04%] | best bounded-ingress high-rate result so far |
| compact raw UDS ring → batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +0.18% | [+0.01%, +0.35%] | standard case neutral/slightly positive versus Stage N compact |
| direct metric rows → batch-drain compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +1.33% | [+1.00%, +1.66%] | standard case positive versus direct rows |

High-rate direct metric rows vs batch-drain compact raw ring selected metrics:

| Variant | Agent UDS MiB/s | processed/s | packet pool avg | ring avg | RSS MiB | heap MiB |
|---|---:|---:|---:|---:|---:|---:|
| direct metric rows | 196.70 | 243,054 | 879 | n/a | 172.41 | 72.18 |
| compact raw ring + batch drain | 211.66 | 261,559 | 0 | 8.29 MiB / 2,249 records | 164.14 | 67.83 |

Verdict: batch drains are a real but smaller high-rate ceiling step. The next ceiling should remove the listener scratch-to-ring copy with a direct reservation or size-class/slabbed compact ring, while adding oldest-age/lag telemetry.

### Stage P — direct compact raw UDS ring

Stage P tests a simple no-copy direct reservation: reserve one max-size contiguous ring span before `ReadFromUnix`, read directly into ring-owned storage, then reclaim unused bytes after commit.

Stage P is single-replicate local evidence only:

| Comparison | Case | Goal | Δ mean | Δ mean CI | Read |
|---|---|---:|---:|---:|---|
| compact batch-drain raw UDS ring → direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | -3.30% | [-3.90%, -2.69%] | simple direct reservation is worse at high rate |
| direct metric rows → direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | +4.46% | [+3.77%, +5.15%] | still positive vs direct rows, but worse than Stage O |
| compact batch-drain raw UDS ring → direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | -0.03% | [-0.11%, +0.06%] | standard case neutral versus Stage O |
| direct metric rows → direct-compact raw UDS ring | `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | +1.78% | [+1.03%, +2.52%] | standard case positive versus direct rows |

High-rate compact batch-drain vs direct-compact selected metrics:

| Variant | Agent UDS MiB/s | processed/s | packet pool avg | ring avg | RSS MiB | heap MiB |
|---|---:|---:|---:|---:|---:|---:|
| compact raw + batch drain | 210.06 | 259,521 | 0 | 8.48 MiB / 2,308 records | 162.51 | 68.83 |
| direct compact raw + batch drain | 203.06 | 250,883 | 0 | 8.61 MiB / 2,391 records | 163.77 | 67.15 |

Verdict: the simple no-copy hypothesis is false in this shape. Requiring a max-size contiguous pre-read reservation costs more than the saved copy under high offered load. Stage N/O remains the best bounded-ingress result; Stage Q adds a direct native v3 payload-construction API and a v2/direct-series bridge, but it should be treated as an architectural cleanup/compatibility step rather than a large standalone throughput win.

### Stage Q — native columnar-to-v3 serializer and v2/direct-series bridge

Stage Q adds a native `metrics.V3MetricPointRow` sink and lets the columnar DogStatsD flush merge points by descriptor across buckets before writing directly to the v3 payload builder. It also adds a v2/direct-series bridge where the same columnar state flushes to `metrics.SerieRow` through `SendDirectSeriesAndSketches`.

Stage Q is single-replicate local evidence only:

| Comparison | Case | Goal | Δ mean | Δ mean CI | Read |
|---|---|---:|---:|---:|---|
| Stage O compact/batch → native v3 | `uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only` | ingress throughput | +0.80% | [-0.10%, +1.71%] | native builder is neutral/slightly positive, below the confidence threshold |
| Stage O compact/batch → native v3 | `uds_dogstatsd_to_api_v3_endpoint_fixed` | ingress throughput | -0.65% | [-1.29%, -0.02%] | native builder is neutral/slightly negative in the standard case |
| main → Stage Q native v3 | high-rate fixed-v3 local case | ingress throughput | +8.10% | [+7.47%, +8.73%] | full design beats main and eliminates packet-pool backlog |
| main → Stage Q native v3 | standard fixed-v3 local case | ingress throughput | +1.67% | [+1.05%, +2.28%] | standard v3 is positive, but memory remains higher |
| main → Stage Q direct-series/v2 | high-rate v2 local case | ingress throughput | +9.77% | [+8.76%, +10.78%] | v2 bridge is positive in the high-rate probe |
| main → Stage Q direct-series/v2 | standard v2 local case | ingress throughput | +3.27% | [+2.50%, +4.03%] | v2 bridge is positive, with higher memory |
| main → Stage Q native v3, origin on | standard fixed-v3 local case | ingress throughput | +2.68% | [+1.69%, +3.66%] | raw ingress disables itself; this probes columnar/native without raw-ring support |

Selected final main-vs-Stage-Q metrics:

| Case | Variant | Agent UDS MiB/s | processed/s | packet pool avg | ring avg | RSS MiB | heap MiB |
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

Verdict: native v3 serialization is architecturally useful and payload-size preserving, but it did not produce a large incremental win over Stage O by itself. The honest `main` wins are best read as the combined columnar + compact raw ingress + native/direct-series serializer design, not as a serializer-only result. Freeze design-stage exploration here; the next work should be broader honesty gates, feature-cost comparisons, memory profiling, and raw-ring lag/oldest-age/backpressure telemetry.

### Post-Stage-Q raw-ring telemetry and honesty matrix

After adding raw-ring lag/oldest-age/backpressure telemetry, the Stage Q design was rebuilt as `datadog/agent-dev:smp-dsd-columnar-v3-raw-lag-telemetry` and re-run with three replicates against `main`.

| Comparison | Δ mean | Δ mean CI | Confidence | Read |
|---|---:|---:|---:|---|
| v3 high-rate UDS, origin off | +3.62% | [+3.35%, +3.90%] | 100.0% | positive but bounded by raw-ring backpressure |
| v3 standard UDS, origin off | +2.25% | [+1.94%, +2.55%] | 100.0% | positive; memory higher |
| v2/direct-series high-rate UDS, origin off | +3.39% | [+3.08%, +3.70%] | 100.0% | positive but bounded by raw-ring backpressure |
| v2/direct-series standard UDS, origin off | +1.51% | [+1.22%, +1.80%] | 100.0% | positive; memory higher |
| v3 high-rate UDS, origin on | +2.32% | [+2.03%, +2.61%] | 100.0% | raw ring disables itself; columnar/native remains positive |
| v3 standard UDS, origin on | +1.40% | [+1.03%, +1.77%] | 100.0% | raw ring disables itself; columnar/native remains positive |
| v3 high-rate UDP, raw disabled | -0.06% | [-0.28%, +0.17%] | 25.4% | neutral; raw ring disables itself because UDP is enabled |
| v3 standard UDP, raw disabled | +0.06% | [-0.38%, +0.51%] | 14.3% | neutral; raw ring disables itself because UDP is enabled |
| v3 standard UDS, mixed metric types | -0.01% | [-0.07%, +0.06%] | 10.6% | neutral; unsupported metric types fall back |

High-rate UDS wins are now explicitly backpressure-bounded: the raw ring averages roughly 9 MiB of consumer lag and blocks listener appends for roughly 400 ms/s. UDP and origin-on runs validate raw-disabled fallback behavior, not raw-ring support for those modes. Mixed metric types validate fallback compatibility, not native columnar support for timers/distributions/histograms. See [`notes/raw-ring-honesty-matrix.md`](notes/raw-ring-honesty-matrix.md).

### Stage R — memory hygiene and telemetry cost controls

Stage R made expensive proof telemetry optional, made naive descriptor interning opt-in after it measured poorly on mostly-unique tagsets, removed normal-path per-bucket descriptor maps, and reused columnar flush buffers. Final image: `datadog/agent-dev:smp-dsd-columnar-v3-memory-hygiene-reuse`.

| Comparison | Case | Δ mean | Δ mean CI | Confidence | Read |
|---|---|---:|---:|---:|---|
| raw-lag telemetry → Stage R interning-on | v3 standard UDS | +0.23% | [+0.03%, +0.43%] | 85.8% | naive interning increased RSS; not default-worthy |
| raw-lag telemetry → Stage R default-off interning | v3 standard UDS | +0.13% | [-0.11%, +0.38%] | 50.9% | throughput neutral; shadow telemetry removed; paired RSS lower |
| raw-lag telemetry → final Stage R reuse | v3 high-rate UDS | +1.50% | [+1.22%, +1.78%] | 100.0% | high-rate path improved, still backpressure-bounded |
| main → final Stage R reuse | v3 standard UDS | +2.22% | [+1.92%, +2.52%] | 100.0% | throughput-positive, but total RSS/heap remain higher |

Final Stage R standard UDS still used about `+45.6 MiB` RSS and `+21.5 MiB` heap alloc versus `main`, despite eliminating the legacy aggregator context/tagstore footprint. See [`notes/stageR-memory-hygiene.md`](notes/stageR-memory-hygiene.md).

### Stage S — heap profiling and v3 serializer allocation analysis

Stage S collected manual Agent heap profiles and added focused v3 payload-builder allocation benchmarks. Late pprof in-use heap was `162.14 MiB` for `main` and `173.31 MiB` for Stage R (`+11.17 MiB`), much smaller than the paired SMP RSS delta. The direct v3 serializer was not a top retained-heap item; the raw ring retained less live heap than the legacy packet pool (`18.72 MiB` vs `32.74 MiB`). Whole-Agent allocation was dominated by DogStatsD parser string interning/tag parsing (`~17.64 GiB` in `stringInterner.LoadOrStore` over the profile window), while serializer-focused columnar-v3 direct point-row allocation was about `64 MiB` over the same 150-sample run.

Low-risk fixes from the profile:

- `V3MetricPointRowSink` now passes row pointers end-to-end to remove row-by-value escape costs in the direct callback/sink path.
- `payloadsBuilderV3.writeSerie` now has a direct no-mutation fast path for common no-special-resource-tag series. In the focused 8,192-row benchmark, `writeSerie` dropped from `~6.95 MiB / 9,123 allocs/op` to `~5.25 MiB / 929 allocs/op` for unique identities, matching the point-row floor.

See [`notes/stageS-heap-and-v3-serializer-profile.md`](notes/stageS-heap-and-v3-serializer-profile.md).

### Stage T — parser string/tagset interning

Stage T attacks the dominant Stage S parser allocation. The parser string interner no longer resets the whole cache at capacity; it now uses bounded recent/protected segments with individual evictions, preserving hot names/tags across high-cardinality churn. `extractTagsMetadata` is non-mutating for normal tagsets, enabling immutable tagset sharing. An opt-in exact raw-tagset cache was added behind:

```bash
DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true
DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER_SIZE=<entries>
```

Focused repeated-tagset benchmark:

| Path | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| default parseTags | `145.9` | `112` | `1` |
| exact tagset interner hit | `15.64` | `0` | `0` |

Three-replicate SMP results:

| Comparison | Case | Δ mean | Δ mean CI | Read |
|---|---|---:|---:|---|
| Stage T default vs `main` | standard v3 UDS | `+2.98%` | `[+2.60%, +3.36%]` | default SLRU improves throughput but not standard RSS |
| Stage T default vs `main` | high-rate v3 UDS metrics-only | `+4.88%` | `[+4.52%, +5.23%]` | still bounded-backpressure wording |
| Stage T tagset cache vs Stage T default | standard v3 UDS | `+0.02%` | `[-0.03%, +0.08%]` | throughput-neutral; RSS lower in paired run |
| Stage T tagset cache vs Stage T default | high-rate v3 UDS metrics-only | `+21.17%` | `[+21.01%, +21.32%]` | parser/backpressure relief; SMP also prints `Regression=true` because abs(delta) >20% |
| Stage T tagset cache vs `main` | standard v3 UDS | `+1.60%` | `[+1.43%, +1.77%]` | lower paired RSS |
| Stage T tagset cache vs `main` | high-rate v3 UDS metrics-only | `+28.42%` | `[+28.21%, +28.64%]` | high-rate/parser-backpressure case |

The exact tagset cache reduced string-interner misses from tens/hundreds of millions per worker to about `47k` in these repeated-tagset SMP workloads. Keep it opt-in until mostly-unique/adversarial tagset feature-cost runs validate the admission policy. See [`notes/stageT-parser-interning.md`](notes/stageT-parser-interning.md).

### Stage U — compact identity hints

Stage U carries compact parser tagset IDs into a bounded worker-local compact identity cache keyed by `(name, host, tagset ID)`. Hits reuse precomputed shard context and carry a compact ID through DogStatsD batcher handoff. Columnar-v3 uses that compact ID only as a validated descriptor hint; the existing `ContextKey + metric type` descriptor map remains authoritative.

Feature gates:

```bash
DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true
DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES=true
```

Three-replicate SMP feature-cost results, comparing Stage U tagset-only vs Stage U tagset+compact identities:

| Case | Δ mean | Δ mean CI | Read |
|---|---:|---:|---|
| standard v3 UDS | `+0.00%` | `[-0.03%, +0.03%]` | throughput neutral |
| high-rate v3 UDS metrics-only | `+0.03%` | `[-0.01%, +0.07%]` | throughput neutral/slightly positive; CI crosses zero |

Focused benchmark showed a compact shard-context cache hit improves the small shard-identity microbenchmark (`63.27 ns/op` → `52.69 ns/op`, no allocations), but the macro repeated-tagset SMP workloads are neutral. Interpretation: Stage U proves compatible compact-ID plumbing, but Stage T already removed most repeated-tagset parser cost; the remaining shard-context/descriptor-map work is too small to move macro throughput by itself. See [`notes/stageU-compact-identity-hints.md`](notes/stageU-compact-identity-hints.md).

### Current value read

The evidence now says the architecture can expose throughput wins when we avoid computing projections that are not needed by the active views, and that moving the live packet queue toward a bounded ingress-log/ring abstraction can control the memory/backlog side effect. The original negative Stage I result was mostly an implementation artifact, not a fundamental refutation. Stage M's fixed-slot raw UDS ring did not recover the full Stage J overload-throughput win, but it removed the packet-pool backlog entirely for the UDS datagram path while staying slightly faster than direct metric rows. Stage N's compact byte ring recovered another high-rate step (+7.25% versus direct metric rows, +3.92% versus fixed-slot raw) while keeping packet-pool backlog at zero. Stage O's batched compact-ring drains add a smaller high-rate step (+1.77% versus Stage N compact, +7.39% versus direct metric rows in its single run). Stage P's simple no-copy direct reservation was worse at high rate (-3.30% versus Stage O), so the Stage N/O scratch-copy compact ring remains the best bounded-ingress design tested so far. Stage Q's native v3 serializer preserves payload size but is only neutral/slightly mixed versus Stage O; it mainly completes the architecture and enables direct v2/v3 honesty probes. Stage T shows parser/string interning was a real remaining bottleneck: SLRU interning safely removes reset churn, and exact raw-tagset caching can turn repeated tagsets into a v3-style in-memory dictionary with large high-rate wins. Stage U shows compact identity IDs can be carried safely from parser-side dictionaries through DogStatsD batching into columnar descriptor hints, but the first consumer is macro-neutral by itself. The strongest current framing is: promising throughput proof for the unified model, plus positive evidence that explicit raw ingress backpressure and parser dictionaries can make the win more production-shaped; the next work is broader compatibility, feature-cost/admission testing, origin/OOB support, and carrying compact IDs farther downstream into descriptor/payload dictionaries.

## Artifacts

- [`report.html`](report.html) — single-page HTML report.
- [`summary.csv`](summary.csv) — exact SMP result table extracted from completed logs.
- [`selected_metrics.csv`](selected_metrics.csv) — explanatory metric aggregates derived from parquet captures with DuckDB.
- `stageL_*.csv` — Stage L single-replicate ingress-log selected metrics.
- [`stageM_selected_metrics.csv`](stageM_selected_metrics.csv) — Stage M sharded-log/raw-ring selected metrics.
- [`stageN_selected_metrics.csv`](stageN_selected_metrics.csv) — Stage N compact raw-ring selected metrics.
- [`stageO_selected_metrics.csv`](stageO_selected_metrics.csv) — Stage O batch-drain compact raw-ring selected metrics.
- [`stageP_selected_metrics.csv`](stageP_selected_metrics.csv) — Stage P direct-compact raw-ring selected metrics.
- [`stageQ_selected_metrics.csv`](stageQ_selected_metrics.csv) — Stage Q native-v3-vs-Stage-O selected metrics.
- [`final_main_vs_stageQ_selected_metrics.csv`](final_main_vs_stageQ_selected_metrics.csv) — final main-vs-Stage-Q v3/v2/origin selected metrics.
- [`final_stageQ_effects.csv`](final_stageQ_effects.csv) — concise final SMP effect table.
- [`honesty3_matrix_selected_metrics.csv`](honesty3_matrix_selected_metrics.csv) — post-telemetry three-replicate matrix selected metrics.
- [`honesty3_matrix_effects.csv`](honesty3_matrix_effects.csv) — post-telemetry three-replicate matrix effect summary.
- [`stageR_memory_hygiene_selected_metrics.csv`](stageR_memory_hygiene_selected_metrics.csv) — Stage R selected metrics.
- [`stageR_memory_hygiene_effects.csv`](stageR_memory_hygiene_effects.csv) — Stage R effect summary.
- [`stageT_parser_interning_selected_metrics.csv`](stageT_parser_interning_selected_metrics.csv) — Stage T selected parser/ring/RSS metrics.
- [`stageT_parser_interning_effects.csv`](stageT_parser_interning_effects.csv) — Stage T effect summary.
- [`stageU_compact_identity_selected_metrics.csv`](stageU_compact_identity_selected_metrics.csv) — Stage U selected compact-identity feature-cost metrics.
- [`stageU_compact_identity_effects.csv`](stageU_compact_identity_effects.csv) — Stage U effect summary.
- [`notes/stageS-heap-and-v3-serializer-profile.md`](notes/stageS-heap-and-v3-serializer-profile.md) — Stage S heap/profile analysis and v3 serializer allocation read.
- `profiles/stageR-agent-heap/` — Stage S Agent heap profiles and pprof summaries.
- `profiles/stageR-v3-payload-builder/` — focused v3 payload-builder benchmark profiles and outputs.
- [`notes/stageT-parser-interning.md`](notes/stageT-parser-interning.md) — Stage T parser string/tagset interning analysis.
- [`notes/stageU-compact-identity-hints.md`](notes/stageU-compact-identity-hints.md) — Stage U compact identity hint analysis.
- [`selected_metrics.sql`](selected_metrics.sql) — DuckDB query used to generate selected metrics.
- `cases/` — raw SMP logs, including failed troubleshooting attempts.
- `captures/stageA-*`, `captures/stageB-*`, `captures/stageC-*`, `captures/stageD-*`, `captures/stageE-*`, `captures/stageF-*`, `captures/stageG-*`, `captures/stageH-*`, `captures/stageI-*` — copied parquet captures for completed runs.
- [`environment.txt`](environment.txt) — local Docker/Colima/SMP environment snapshot.
- [`images.txt`](images.txt) — local image metadata and Agent versions.
- [`notes/experiment-log.md`](notes/experiment-log.md) — chronological experiment log.

## Local caveats

- Stage E is intentionally unsafe/local: it changes the active DogStatsD flush path when `DD_DOGSTATSD_EXPERIMENTAL_DIRECT_SERIALIZER=true` and only supports v2/v3 protobuf series, not JSON v1 series.
- Stage E still serializes `metrics.Serie` / `metrics.SketchSeries` rows produced by existing samplers; it does not yet eliminate aggregator materialization itself.
- Stage F moves DogStatsD time-sampler series to `metrics.SerieRow`, but sketches, check-sampler rows, and the metric flush/dedup internals are not yet row-native.
- Stage G/H/I/L/M/N/O/P/Q/T/U are intentionally value probes, not safe migrations. Stage H is not wire-equivalent because it can emit extra rows for the same identity instead of merging points. Stage I is v3-only/metric-only and has legacy fallbacks for unsupported samples/checks/sketches/events. Stage L changes overload/backpressure behavior behind an env gate and uses a first-pass packet-batch ingress log. Stage M/N/O/P add sharded packet-batch and raw UDS-ring gates, but raw modes currently exclude UDP, stream sockets, named pipes, statsd forwarding, and origin detection. Stage Q adds gated native-v3 and direct-series/v2 serializer paths; the origin-on and UDP SMP runs intentionally show raw ingress disabling itself and therefore do not validate raw-ring origin/UDP support. Stage T's exact tagset cache and Stage U compact identities remain opt-in until admission/memory behavior is validated on mostly-unique/adversarial tagsets and more downstream consumers use the compact IDs.
- Initial runs failed before Colima was resized from 6 CPUs to 10 CPUs; those logs are retained but excluded from `summary.csv`.
- A `local-2cpu` experiment was attempted as troubleshooting only and is excluded from results.
- SMP analysis emitted local AWS config/IMDS warnings; these did not prevent completed local analysis.
- The source `uds_dogstatsd_to_api_v3` case uses an older v3 env var and did not emit current v3 payload telemetry; `uds_dogstatsd_to_api_v3_endpoint_fixed` is a local corrected case for current v3 endpoint config.
- Bazel verification remains separately blocked by the local `NONO_MEDIATION_SOCKET` issue and was not used for this SMP report.

# Stage V: compact ID downstream consumers

## Goal

Use DogStatsD compact identities as first-class downstream inputs, not only descriptor lookup hints, in a narrow vertical slice that can be macro-measured while preserving the normal path as the fallback.

## Prototype

Implemented behind explicit experimental gates:

- `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_DIRECT_PARSE=true`
  - Parses supported DogStatsD metric messages directly into the columnar-v3 row batcher.
  - Avoids materializing/parsing a full `MetricSample` batch for supported metric rows.
  - Stays disabled when mapper, extra tags, histogram-to-distribution, or debug stats are active.
  - Falls back to the legacy metric path for unsupported/timestamped/non-finite samples and unsupported metric types.
- `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_COMPACT_ROWS_OMIT_DESCRIPTOR=true`
  - Turns `DogStatsDColumnarV3Sample` into a scalar row with optional descriptor fields.
  - Once a compact identity + metric type has been observed by columnar-v3, later rows can omit name/tags/host/source/unit descriptor fields.
  - The columnar store resolves rows by `(context key, metric type)` and compact descriptor mappings.

Safety details added during the prototype:

- `DogStatsDCompactIdentityState` tracks columnar descriptor knowledge per metric type, not just per compact ID. This avoids mixed-type identities (same name/tagset used as gauge and counter) omitting a descriptor for the wrong type.
- Columnar descriptors remember the compact states that depend on them and clear those state bits on descriptor expiry, so parser workers include descriptors again after expiry.
- If an omitted-descriptor row still reaches columnar without an active descriptor, columnar clears the state bit and records `compact_descriptor_missing` fallback instead of creating a descriptor from incomplete data. This remains experimental-only and should be tightened before compatibility work.

## Images

Built optimized Linux/arm64 image from the local Stage V worktree:

| image | image ID | env |
|---|---:|---|
| `datadog/agent-dev:smp-dsd-columnar-v3-compact-consumers` | `sha256:8dfa1e6b4817d2566e41461779fa11c0d8eb6df6a9720f4440d09a343cdcf388` | code only |
| `datadog/agent-dev:smp-dsd-columnar-v3-compact-consumers-baseline` | `sha256:81c8db9c5df79d54335d84f7a98f9bc88a6c3e956bc590c0216e9b0e335eb5bd` | tagset interner + compact identities + compact hint sizing |
| `datadog/agent-dev:smp-dsd-columnar-v3-compact-consumers-enabled` | `sha256:865f8bfecabcf1a161cef0c810f0e830fe51937d6aba5cd4c5b878decf819485` | baseline env plus direct parse + descriptor omission |

## Validation

Unit tests:

```bash
dda inv test --targets=./pkg/aggregator,./pkg/metrics,./comp/dogstatsd/server/impl,./comp/dogstatsd/internal/identity --timeout=300
```

Result: all tests passed (`657` package tests / unified report `575` tests).

Macro SMP feature-cost runs compare the baseline image against the enabled image, with both images already running the Stage T exact tagset cache and Stage U compact identities.

```bash
smp local run \
  --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-stageO-batch-drain \
  --case uds_dogstatsd_to_api_v3_endpoint_fixed_compact_batch \
  --baseline-image datadog/agent-dev:smp-dsd-columnar-v3-compact-consumers-baseline \
  --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-compact-consumers-enabled \
  --replicates 3 \
  --total-samples 270

smp local run \
  --experiment-dir reports/smp/dogstatsd-agg-serde-20260516-143205/local-experiment-stageO-batch-drain \
  --case uds_dogstatsd_to_api_v3_endpoint_fixed_250mb_metrics_only_compact_batch \
  --baseline-image datadog/agent-dev:smp-dsd-columnar-v3-compact-consumers-baseline \
  --comparison-image datadog/agent-dev:smp-dsd-columnar-v3-compact-consumers-enabled \
  --replicates 3 \
  --total-samples 270
```

Results:

| workload | throughput delta | CI | conclusion |
|---|---:|---:|---|
| standard v3 UDS compact batch | `-0.02%` | `[-0.15%, +0.12%]` | neutral |
| high-rate v3 UDS metrics-only compact batch | `-0.00%` | `[-0.04%, +0.03%]` | neutral |

Selected metrics are in `stageV_compact_consumers_selected_metrics.csv`; effect summary is in `stageV_compact_consumers_effects.csv`.

## Interpretation

The Stage V vertical slice proves the plumbing is viable and safe enough for local experimentation, but it does not create a macro throughput win in these SMP cases. CPU moved slightly down in both final runs (`~0.6%` standard, `~0.9%` high-rate), while throughput stayed generator-limited/neutral. Standard-case RSS/heap averages were noisy and should not be treated as a memory conclusion.

The likely reason is that this slice still materializes name/tag strings and tag slices in the parser/enrichment path before compact IDs can help. Descriptor omission removes repeated row metadata after columnar observes a descriptor, but the remaining hot path still pays most of the string/tag parsing cost. Bigger wins likely require the next step: parse to IDs/offsets first and defer string/tag materialization unless mapper/origin/debug/fallback needs it.

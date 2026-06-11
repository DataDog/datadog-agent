# Stage U: compact DogStatsD identity hints

Date: 2026-05-20

## Goal

Convert repeated DogStatsD identity strings into bounded compact identifiers as early as possible, then carry those identifiers through the hot path without changing authoritative aggregation semantics.

The first production-safe slice is intentionally narrow:

- exact raw-tagset cache must already identify a repeated client tagset;
- worker-local identity cache maps `(name, host, parser tagset ID)` to a compact ID and precomputed shard context;
- DogStatsD batcher carries the compact ID to the columnar-v3 worker;
- columnar-v3 uses the compact ID only as a descriptor lookup hint and validates it against the existing `ContextKey + metric type` identity before reuse.

All new behavior is opt-in behind `DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES=true`. Parser exact-tagset IDs require `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true`.

## Implementation

Code commit:

```text
9376b73a9c1 dogstatsd: carry compact identity hints
```

Main changes:

- `parserTagsetInterner` now stores a compact monotonically increasing tagset ID with each admitted exact raw tagset.
- `dogstatsdMetricSample` and `metrics.MetricSample` carry `DogStatsDTagsetID` as an experimental sidecar.
- Mapper-added tags and global `extraTags` clear `DogStatsDTagsetID`, because the raw parser tagset no longer describes the final sample tagset.
- `identity.Builder` has an opt-in bounded worker-local compact identity cache:
  - env: `DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES=true`
  - size: `DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES_SIZE=<entries>` (default `4096`)
  - key: `(sample.Name, sample.Host, sample.DogStatsDTagsetID)`
  - value: compact ID + precomputed shard identity
  - compact IDs encode worker scope in the high bits so IDs can be sent to shared downstream workers.
- The debug-enabled path is unchanged; the compact cache is used by the shard-only hot path so it does not materialize debug display tags when debug is off.
- `aggregator.DogStatsDColumnarV3Sample` carries `CompactID` to the columnar worker.
- Columnar-v3 keeps a bounded lazy `compactID -> descriptorID` hint table:
  - env size: `DD_DOGSTATSD_EXPERIMENTAL_COLUMNAR_V3_COMPACT_HINT_SIZE=<entries>` (default `65536`)
  - allocated only after a non-zero compact ID is observed;
  - every hit is validated against active descriptor state (`ContextKey` and metric type) before reuse;
  - stale/colliding IDs fall back to the existing descriptor map.

## Focused validation

```bash
dda inv test --targets=./comp/dogstatsd/server/impl,./comp/dogstatsd/internal/identity,./comp/dogstatsd/listeners,./comp/dogstatsd/packets,./pkg/aggregator,./pkg/metrics --timeout=300
```

Result: passed (`624` tests).

Focused identity microbenchmark:

```bash
dda inv test --targets=./comp/dogstatsd/internal/identity \
  --test-run-name='^$' \
  --extra-args='-bench=BenchmarkCompactIdentityShardContext -benchmem -benchtime=3s' \
  --timeout=300
```

| Path | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| baseline shard context | `63.27` | `0` | `0` |
| compact shard cache hit | `52.69` | `0` | `0` |

Parser repeated-tagset benchmark stayed in the Stage T envelope after adding tagset IDs:

| Path | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| default repeated tagset parse | `145.9` | `112` | `1` |
| exact tagset interner hit | `15.45` | `0` | `0` |

## Images

- `datadog/agent-dev:smp-dsd-columnar-v3-compact-identity-hints`
  - image ID `sha256:ab07f3b9d15e12a13ba116484d5ec60587c9233bc8468977b809ad45dc11529d`
  - Agent `9376b73a9c1`
- `datadog/agent-dev:smp-dsd-columnar-v3-compact-identity-hints-tagset`
  - image ID `sha256:070cbafd961ff188741a94c2c0c5c16961c1fde0ff4d89eec1bf3a3bfda264b2`
  - env `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true`
- `datadog/agent-dev:smp-dsd-columnar-v3-compact-identity-hints-enabled`
  - image ID `sha256:3ebd8cc669b1f6c8c31a831e9972b81b6bd8d454fb9e77cf1fe1d665eabd6d05`
  - env `DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER=true`
  - env `DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES=true`

## SMP feature-cost runs

Feature-cost comparison isolates compact identities by comparing two images from the same Stage U binary:

- baseline: exact raw-tagset cache only;
- comparison: exact raw-tagset cache + compact identities.

Commands used the Stage O compact raw UDS batch-drain cases and `--replicates 3 --total-samples 270`.

| Comparison | Case | Δ mean | CI | Read |
|---|---|---:|---:|---|
| tagset cache vs compact identities | standard v3 UDS | `+0.00%` | `[-0.03%, +0.03%]` | throughput neutral |
| tagset cache vs compact identities | high-rate v3 UDS metrics-only | `+0.03%` | `[-0.01%, +0.07%]` | throughput neutral/slightly positive; CI crosses zero |

Selected memory/backpressure metrics were mixed:

- standard v3 UDS paired run: compact identities had lower average RSS/heap in this run (`RSS -4.13%`, heap alloc `-4.66%`) and lower max raw-ring lag;
- high-rate v3 UDS metrics-only paired run: compact identities had higher average RSS/heap (`RSS +1.04%`, heap alloc `+4.87%`) and higher max raw-ring lag, while reported blocked time was lower;
- string-interner misses/evictions were unchanged, as expected: compact identities start after parser tagset interning.

Interpretation: Stage U proves the compact-ID plumbing is compatible and macro-neutral in the repeated-tagset UDS/v3 envelope, but it does not yet produce a macro throughput win. The likely reason is that Stage T already removes most parser/tag materialization cost for this workload; remaining shard-context and descriptor-map work is smaller than SMP noise at current rates.

## Decision

Keep compact identities opt-in. The implementation is useful plumbing for carrying IDs downstream, but not enough by itself to claim a macro win. Next useful steps:

1. add no-allocation compact-cache hit/miss/eviction counters flushed off the hot path;
2. carry descriptor/tagset IDs farther into payload dictionary construction;
3. validate feature cost on mostly-unique/adversarial tagsets before considering default enablement;
4. rerun after parser no-materialization or downstream dictionary-ID reuse can consume the compact IDs more directly.

Artifacts:

- `reports/smp/dogstatsd-agg-serde-20260516-143205/stageU_compact_identity_effects.csv`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/stageU_compact_identity_selected_metrics.csv`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/cases/stageU-compact-identity-feature-cost-standard.log`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/cases/stageU-compact-identity-feature-cost-high.log`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/captures/stageU-compact-identity-feature-cost-standard/`
- `reports/smp/dogstatsd-agg-serde-20260516-143205/captures/stageU-compact-identity-feature-cost-high/`

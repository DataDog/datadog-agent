# DogStatsD Ingestion and Query Vision

Status: exploratory working draft.

This document captures a possible architecture-level direction for DogStatsD: treat ingestion as a streaming data system with reusable identities, bounded logs, and materialized views. The goal is to make debugging, capture/replay, recent lookback queries, and normal aggregation share core work instead of existing as separate hot-path side features.

## Motivation

Current DogStatsD functionality has overlapping concerns implemented in different places:

- normal aggregation parses and enriches samples, then the aggregator computes context keys;
- the batcher computes a separate shard key from sample name/host/tags;
- `serverDebug` recomputes a debug key and stores stats in a separate globally locked map;
- capture/replay taps raw listener buffers as a side path, with transport-specific behavior.

This creates duplicate CPU work, divergent semantics, and makes always-on observability expensive. A database-inspired design would model DogStatsD as a stream of facts plus queryable materialized views.

## Goals

- Compute normalized identity and tag state once, then reuse it across aggregation, debug, sharding, lookback, and replay.
- Support always-on low-cost troubleshooting views.
- Support capture/replay inclusively, including retrospective capture from a bounded raw lookback buffer.
- Support recent local queries with aggregation windows smaller or different from the normal Agent pipeline flush interval.
- Keep ingestion bounded and predictable under high volume and high cardinality.
- Preserve existing backend payload semantics unless explicitly changed.

## Non-goals

- Arbitrary SQL over all DogStatsD traffic.
- Unbounded raw packet retention.
- Per-sample text logging as the primary observability mechanism.
- Replacing the normal aggregator in one step.

## Core model

Use two first-class streams/logs:

1. **Raw ingress log**
   - Transport-faithful records: timestamp, source, listener, payload bytes, process/ucred/OOB metadata, peer/connection metadata when available.
   - Used for raw capture, raw replay, parse forensics, and exemplar links.

2. **Canonical semantic stream**
   - Parsed and enriched records: metrics, events, service checks, normalized tags, origin metadata, and precomputed identities.
   - Used for aggregation, debug stats, lookback windows, and local queries.

Everything else should be a materialized view over one of these streams.

```text
listeners
  -> ingress envelopes
  -> raw WAL/ring buffer  -----------------> capture export / raw replay
  -> parser / normalizer
  -> canonical records + lineage
  -> series resolver
  -> shard router by effective series identity
  -> per-shard materialized views
       - backend aggregation view
       - debug series stats view
       - recent lookback/window view
       - spike/top-k/cardinality views
       - parse/drop/error views
       - exemplar links back to raw ingress
  -> query APIs read snapshots, not hot mutable state
```

## Important identities

Different use cases need different identities. They should be explicit rather than accidental.

- `flow_id`: transport/client identity, such as listener plus peer/process/origin information.
- `client_series_id`: identity based on what the client effectively submitted after parsing/mapping, before runtime enrichment if desired.
- `effective_series_id`: identity used for aggregation/backend semantics after enrichment, tag filtering, host extraction, and other effective dimensions.
- `envelope_id` / `message_id` / `sample_id`: lineage IDs connecting semantic records back to raw ingress data.

A rough descriptor shape:

```go
type SeriesDescriptor struct {
    ClientSeriesID    uint64
    EffectiveSeriesID uint64

    NameRef           uint32
    HostRef           uint32
    MetricTagsRef     uint32
    TaggerTagsRef     uint32
    OriginRef         uint32
    Source            metrics.MetricSource
    NoIndex           bool
}
```

A resolved metric record can then carry reusable IDs instead of forcing each consumer to reconstruct them:

```go
type ResolvedSample struct {
    ProcessingTimeUnixNano int64
    TimestampUnixNano      int64 // zero for untimestamped samples

    ClientSeriesID         uint64
    EffectiveSeriesID      uint64
    SeriesRef              uint32

    MetricType             metrics.MetricType
    Value                  float64
    SampleRate             float64

    EnvelopeID             uint64
    MessageID              uint64
}
```

## Materialized views

### Backend aggregation view

This is the existing normal Agent pipeline behavior, keyed by effective backend series identity and flushed at the normal interval. It can initially remain implemented by the existing aggregator, but should eventually consume precomputed series identity/tag state instead of recomputing context keys.

### Debug series stats view

Always-on replacement for the current `serverDebug` map:

- count;
- first seen / last seen;
- recent rate;
- optional exemplars pointing to recent raw envelopes/messages;
- top-K series/metrics/origins;
- bounded TTL and memory budgets.

`agent dogstatsd-stats` becomes a query over this view.

### Recent lookback/window view

Maintain a bounded ring of small mergeable aggregate buckets, independent from the normal backend flush interval.

Example:

```text
series_id -> ring[1s bucket] -> aggregate state
```

Queries can merge buckets to answer:

- last 30s at 1s step;
- last 5m at 10s step;
- top spiking series over the last minute;
- metric rate/count by origin/listener over a recent window.

This separates two concepts that are currently easy to conflate:

- backend aggregation interval: what the Agent sends upstream;
- local query aggregation interval: what an operator wants to inspect locally.

The view should store mergeable aggregate state rather than only final emitted values. Metric-type semantics matter:

- counts/counters: sum, then rate if requested;
- distributions/histograms/timings: merge sketches or equivalent state;
- gauges: define query semantics explicitly, such as last/min/max/avg;
- sets: exact distinct values are expensive, so approximate modes may be needed.

### Raw ingress ring / capture view

Keep a bounded byte-faithful ring/WAL of recent ingress envelopes. This enables:

- retrospective capture: "save the last 30 seconds";
- trigger-based capture: "save raw traffic around a spike";
- exemplar drilldown: "show raw messages contributing to this series";
- raw replay with original cadence and transport metadata where possible.

### Error/drop views

Parse errors, filtered points, drops, and rate-limit events should also be materialized as queryable views instead of only logs/counters.

## Payload-aligned columnar storage

The metrics v3 payload format is directly relevant to this vision. It already optimizes for many of the same properties this design wants internally:

- dictionary-encoded metric names, tags, tagsets, resources, origin information, and units;
- columnar storage for series fields, timestamps, values, and sketches;
- delta encoding for repeated references and timestamps;
- independently compressed columns;
- compact representation of repeated series metadata.

The internal canonical stream and materialized views should therefore be designed to be **payload-aligned** with v3 or a close internal sibling format. The goal is not necessarily to store the exact wire payload everywhere, but to make the hot path produce an intermediate representation that the v3 payload builder can consume with minimal translation.

A useful split:

1. **Hot mutable state**
   - Per-shard maps and aggregate states optimized for fast updates.
   - Uses stable internal refs for names, tags, tagsets, resources, origins, and units.

2. **Sealed columnar segments**
   - Immutable recent-lookback blocks, likely bucketed by time and shard.
   - Columnar and dictionary-encoded, close to v3 payload structure.
   - Good for local scans, compression, retrospective capture metadata, and payload construction.

3. **Wire payload encoding**
   - Converts internal refs/segments into the exact v3 payload format.
   - Ideally streams from sealed segments or aggregation output without rebuilding strings/tagsets.

Important caveat: v3 payload dictionaries are payload-local today. Internal stores likely need longer-lived per-shard or per-segment dictionaries with epochs. The encoder may still need a lightweight remap from internal dictionary IDs to payload-local v3 IDs unless the wire format eventually supports reusable dictionary epochs.

This suggests a shared `SeriesDescriptor`/dictionary layer should sit below both DogStatsD views and serializer v3. In the best case, normal aggregation output is already represented as:

```text
series_descriptor_ref + time/value columns + aggregate state
```

and payload building mostly becomes column selection, delta/reference encoding, compression, and framing.

Open design question: columnar storage is excellent for sealed segments and scans, but not always ideal for the hottest per-sample update path. A hybrid design is likely best: row/event-shaped records at parse time, per-shard mutable aggregate state for updates, then sealed columnar segments for lookback/query/export.

## Replay modes

There should be two explicit replay products:

1. **Raw replay**
   - Replays raw ingress envelopes with original timing and transport metadata.
   - Best for listener, origin-detection, parser, and transport fidelity investigations.

2. **Semantic replay**
   - Replays canonical records into the downstream pipeline.
   - Best for aggregation correctness, performance testing, and deterministic reproduction independent of transport.

Raw replay needs captured bytes plus enough metadata to emulate the original transport. Semantic replay needs resolved descriptors or versioned enrichment state.

## Enrichment state and determinism

Tagger/workloadmeta, mapper profiles, filter lists, host extraction, and related enrichment inputs can change over time. Raw payload alone is not enough for deterministic semantic replay.

Possible approaches:

- store resolved `SeriesDescriptor` state with semantic records;
- persist versioned snapshots/changelogs of enrichment dimensions;
- treat replay mode explicitly: raw replay re-runs current enrichment, semantic replay reuses captured enrichment.

This is one of the core database-like design problems.

## Hot-path principles

- No global per-sample mutex.
- No unbuffered per-sample side channels.
- No mandatory per-sample text logging.
- Compute names/tags/keys once per logical record.
- Use per-shard owned mutable state.
- Query immutable snapshots or RCU-style published state.
- Bound every always-on store by time, bytes, series count, or both.
- Prefer approximate heavy-hitter/cardinality structures for long-tail visibility.
- Keep capture backpressure/drop policy isolated from core ingestion whenever possible.
- Align internal dictionaries/columns with metrics v3 payload concepts where it reduces translation and serialization cost.

## Current design gaps

1. **No canonical ingest model**
   - `Packet` is both transport-ish data and an execution batching container.
   - UDP assembly can merge datagrams for processing, while capture wants transport-faithful units.

2. **Identity is recomputed in multiple places**
   - batch sharding, aggregator context resolution, and `serverDebug` compute overlapping keys separately.

3. **Semantics differ accidentally**
   - debug series identity is not clearly the same as backend aggregation identity nor clearly a deliberate client-view identity.

4. **`serverDebug` is not suitable as an always-on core view**
   - global lock, separate map, unbounded cardinality, per-sample optional logging, synchronous spike-count channel.

5. **Capture is not a first-class stream consumer**
   - current capture is listener-side and transport coverage is uneven.
   - buffer lifetime is handled via pool-manager passthrough/reference accounting rather than a general multi-consumer data model.

6. **No local window store**
   - normal aggregation state is optimized for backend flush, not dynamic recent lookback queries.

7. **No snapshot query layer**
   - debug/status reads are not cleanly separated from hot mutable state.

8. **No explicit query planner or budgets**
   - always-on dynamic queries require allowed query shapes, indexes, retention, and memory/CPU limits.

9. **Serializer payload construction is downstream-only**
   - v3 payload encoding already has useful dictionary/columnar concepts, but the current DogStatsD/aggregator path does not expose a payload-aligned internal representation for reuse.

## Milestone roadmap

This work should be split into milestones that each deliver value independently. Every milestone should answer three questions:

1. **What gets better if the project stops here?**
2. **How do we prove that with artifacts reviewers can inspect?**
3. **If the full arc completes, what old path or complexity does this let us delete?**

Shared proof artifacts should include targeted benchmarks, CPU/heap profiles for hot-path changes, semantic/golden tests, race tests for concurrent stores, and before/after documentation of identity semantics.

### Milestone 0: Baseline semantics and performance guardrails

**Value delivered**

- Makes current behavior explicit before refactoring.
- Reduces risk of accidentally changing metric identity, tag handling, or backend payload output.
- Produces reusable benchmark scenarios for later PRs.

**Scope**

- Document listener -> parser -> enrich -> batcher -> aggregator dataflow.
- Add tests and benchmarks covering:
  - debug off/on;
  - high-cardinality tags;
  - multiple DogStatsD workers and aggregation pipelines;
  - capture off/on;
  - host extraction;
  - mapper-added tags;
  - origin/tagger-enriched tags;
  - metric tag filtering;
  - timestamped/no-aggregation samples;
  - histogram-to-distribution copy.

**Proof / acceptance criteria**

- Golden tests describe existing batch shard key, debug key, and aggregator context key behavior.
- Benchmarks can be run locally and in CI-like environments with stable enough inputs for `benchstat` comparison.
- No production behavior changes.

**Initial proof artifacts**

- Baseline tests:
  - `comp/dogstatsd/server/impl/identity_baseline_test.go`;
  - `comp/dogstatsd/serverDebug/impl/debug_baseline_test.go`;
  - `pkg/aggregator/context_resolver_baseline_test.go`.
- Baseline benchmarks:
  - `BenchmarkMilestone0ShardKeyGenerator`;
  - `BenchmarkMilestone0ParsePacketsGuardrails`;
  - `BenchmarkMilestone0StoreMetricStats`;
  - `BenchmarkMilestone0CaptureEnqueue`;
  - `BenchmarkMilestone0ContextResolverGuardrails`.
- Suggested verification commands:
  - `dda inv test --targets=./comp/dogstatsd/server/impl,./comp/dogstatsd/serverDebug/impl,./comp/dogstatsd/replay/impl,./pkg/aggregator --test-run-name='Milestone0'`;
  - `dda inv test --targets=./comp/dogstatsd/server/impl,./comp/dogstatsd/serverDebug/impl,./comp/dogstatsd/replay/impl,./pkg/aggregator --test-run-name='^$' --extra-args='-bench=BenchmarkMilestone0 -benchmem -count=1'`.

**Stop-safe state**

- If work stops here, the repo is better documented and safer to change.

**End-state cleanup enabled**

- These tests become the compatibility harness for deleting old duplicate key paths later.

### Milestone 1: Explicit sample identity contracts

**Value delivered**

- Turns accidental identity differences into named concepts.
- Creates the foundation for shared debug, lookback, replay, and serializer descriptors.

**Scope**

- Introduce an internal DogStatsD identity/descriptor model near the point where `MetricSample` is ready for batching.
- Name identities explicitly, for example:
  - client series and debug-view grouping key: what the client submitted before tagger enrichment, with the current stats view preserving its host-excluding projection;
  - shard identity: what currently selects the aggregation pipeline;
  - effective backend identity: what eventually determines backend aggregation;
  - lineage identity: listener/process/origin/capture metadata.
- Initially compute only fields already available in the DogStatsD worker.

**Proof / acceptance criteria**

- Tests prove new identity functions match existing behavior for the cases from Milestone 0.
- No backend payload or `agent dogstatsd-stats` output changes except where intentionally documented.
- CPU/allocation impact is neutral or better when the new code is wired but not yet reused broadly.

**Initial proof artifacts**

- Identity contract model:
  - `comp/dogstatsd/internal/identity/identity.go`.
- Contract tests:
  - `comp/dogstatsd/internal/identity/identity_test.go`;
  - `comp/dogstatsd/server/impl/identity_contract_test.go`;
  - `comp/dogstatsd/serverDebug/impl/identity_contract_test.go`.
- Baseline benchmark:
  - `BenchmarkMilestone1Builder`.
- Suggested verification commands:
  - `dda inv test --targets=./comp/dogstatsd/internal/identity,./comp/dogstatsd/server/impl,./comp/dogstatsd/serverDebug/impl --test-run-name='Milestone1'`;
  - `dda inv test --targets=./comp/dogstatsd/internal/identity --test-run-name='^$' --extra-args='-bench=BenchmarkMilestone1 -benchmem -count=1'`.

**Stop-safe state**

- If work stops here, future contributors have a clear identity vocabulary and tested helper functions.

**End-state cleanup enabled**

- Gives a single place to remove duplicate hashing/tag handling from batcher, debug, and eventually aggregator code.

### Milestone 2: Compute hot-path series, shard, and debug-view keys once

**Value delivered**

- Simple efficiency improvement with low semantic risk.
- Establishes the pattern of carrying resolved context alongside samples.

**Scope**

- Carry a hot-path context alongside each parsed sample through DogStatsD batching.
- Make batch shard selection consume the precomputed shard identity.
- Make `serverDebug` consume the precomputed debug-view key and tag display string, preserving the current host-excluding stats grouping as a compatibility projection rather than treating it as a separate series identity.
- Keep aggregator context resolution unchanged initially.

**Proof / acceptance criteria**

- Benchmarks show reduced or neutral CPU/allocation cost in DogStatsD parse/batch/debug paths.
- Golden tests prove shard selection and debug-view grouping keys are unchanged.
- Race tests pass with multiple workers.

**Initial proof artifacts**

- Hot-path context wiring:
  - `identity.Builder.ResolveHotPath`;
  - `batcher.appendSampleWithContext` / `appendLateSampleWithContext`;
  - `serverDebug.StoreMetricStatsWithDebugViewKey`;
  - worker-local identity builder passed through `parsePackets`.
- Contract tests:
  - `TestMilestone2BatcherUsesPrecomputedShardIdentity`;
  - `TestMilestone2ParsePacketsCarriesResolvedSampleContext`;
  - `TestMilestone2StoreMetricStatsWithDebugViewKeyMatchesLegacyKey`;
  - `TestMilestone2StoreMetricStatsWithDebugViewKeyConcurrent`.
- Benchmarks:
  - `BenchmarkMilestone2ResolvedContextReuse`;
  - `BenchmarkMilestone2StoreMetricStatsWithDebugViewKey`.
- Suggested verification commands:
  - `dda inv test --targets=./comp/dogstatsd/internal/identity,./comp/dogstatsd/server/impl,./comp/dogstatsd/serverDebug/impl --test-run-name='Milestone2'`;
  - `dda inv test --targets=./comp/dogstatsd/serverDebug/impl --test-run-name='Milestone2StoreMetricStatsWithDebugViewKeyConcurrent' --extra-args='-race'`;
  - `dda inv test --targets=./comp/dogstatsd/internal/identity,./comp/dogstatsd/serverDebug/impl --test-run-name='^$' --extra-args='-bench=BenchmarkMilestone2 -benchmem -count=1'`.

**Stop-safe state**

- If work stops here, DogStatsD does less duplicate key/tag work and the new context object has a concrete use.

**End-state cleanup enabled**

- Starts replacing ad hoc recomputation with an explicit descriptor pipeline.

### Milestone 3: Replace `serverDebug` with a bounded materialized view

**Value delivered**

- Makes `agent dogstatsd-stats` cheaper and safer to leave enabled.
- Removes current hot-path bottlenecks: global per-sample lock, unbuffered spike channel, unbounded stats map, live-map marshaling under lock.

**Scope**

- Add worker-local or shard-local debug stats ownership.
- Add TTL/cardinality/memory budgets.
- Replace per-sample spike channel with per-shard counters/buckets.
- Query immutable snapshots or merged copies.
- Preserve the existing runtime setting, endpoint, and command shape at first.

**Proof / acceptance criteria**

- With debug enabled, throughput/latency improves materially or at least avoids current pathological contention.
- Memory is bounded under high-cardinality traffic; tests cover eviction/TTL behavior.
- `agent dogstatsd-stats` output remains compatible or documented as intentionally changed.
- Snapshot reads do not block hot-path sample ingestion on a global lock.

**Initial proof artifacts**

- `serverDebug` now stores stats in `debugStatsView`, a 32-shard materialized view instead of one globally locked map.
- The view enforces bounded retention with a default 65,536-context budget and a 10-minute TTL. Under budget pressure, the oldest row in the target shard is evicted; stale rows are pruned on insert/snapshot.
- Spike detection uses sharded time buckets instead of a per-sample unbuffered channel.
- `GetJSONDebugStats()` marshals a merged snapshot and only locks one shard at a time.
- Compatibility is preserved for normal `dogstatsd-stats` output shape and grouping. The intentional bounded-view behavior change is that very old or over-budget rows may be absent instead of retained forever.
- Contract tests:
  - `TestMilestone3DebugStatsViewEvictsOldestWhenBudgetExceeded`;
  - `TestMilestone3DebugStatsViewExpiresStaleContexts`;
  - `TestMilestone3DebugStatsViewResetsExpiredContextCount`;
  - `TestMilestone3DebugStatsViewUsesShardLocalLocks`;
  - `TestMilestone3SpikeCountersUseTimeBucketsWithoutMetricChannel`.
- Benchmark:
  - `BenchmarkMilestone3StoreMetricStatsWithDebugViewKey`.
- Suggested verification commands:
  - `dda inv test --targets=./comp/dogstatsd/serverDebug/impl`;
  - `dda inv test --targets=./comp/dogstatsd/serverDebug/impl --test-run-name='Milestone3' --extra-args='-race'`;
  - `dda inv test --targets=./comp/dogstatsd/serverDebug/impl --test-run-name='^$' --extra-args='-bench=BenchmarkMilestone3 -benchmem -count=1'`.

**Stop-safe state**

- If work stops here, users get a safer DogStatsD stats feature and operators can enable it with more confidence.

**End-state cleanup enabled**

- Deletes the old `serverDebug` map/channel architecture rather than layering another debug system beside it.

### Milestone 4: Shard-local `SeriesStatsStore`

**Value delivered**

- Generalizes debug stats into a reusable materialized-view substrate.
- Provides a home for counts, first/last seen, recent rate, top-K, cardinality estimates, and exemplars.

**Scope**

- Move debug state into per-shard stores keyed by explicit series identity.
- Add snapshot merge APIs for status/debug endpoints.
- Add top-K and recent-rate summaries before adding fully dynamic windows.
- Expose store telemetry: series count, evictions, bytes, update cost, snapshot cost.

**Proof / acceptance criteria**

- Store update cost is O(1) expected on the hot path.
- Memory and series-count budgets are enforced in tests.
- Snapshot APIs have deterministic results for fixed inputs.
- Existing DogStatsD stats behavior is implemented on top of this store.

**Stop-safe state**

- If work stops here, there is already a useful bounded stats store and a cleaner implementation of DogStatsD stats.

**End-state cleanup enabled**

- Avoids future one-off maps for debug, rates, top-K, and local status by giving them a shared substrate.

### Milestone 5: Raw ingress envelopes and bounded raw ring

**Value delivered**

- Adds retrospective capture and trigger-based capture foundations.
- Makes capture a first-class stream consumer instead of a listener-specific side path.

**Scope**

- Define `IngressEnvelope` as the transport-faithful unit: timestamp, listener/source, payload bytes, origin/process/ucred/OOB metadata where available.
- Keep execution batching separate from envelope semantics.
- Add a bounded in-memory ring first.
- Support UDP, UDS, and named pipe consistently where platform metadata permits.
- Export from the ring using the existing capture/replay file shape where possible.

**Proof / acceptance criteria**

- Capture fidelity tests cover bytes and metadata for each supported transport.
- Capture backpressure/drop behavior is isolated from ingestion and visible via telemetry.
- Benchmarks show capture disabled has negligible overhead and capture enabled has bounded cost.
- Existing capture/replay workflows keep working.

**Stop-safe state**

- If work stops here, users get better capture coverage and retrospective capture potential.

**End-state cleanup enabled**

- Lets us remove uneven listener-side capture hooks once the ring-backed path is complete.

### Milestone 6: Fixed-shape recent lookback queries

**Value delivered**

- Delivers a new operator feature: inspect recent DogStatsD activity without waiting for backend flushes or enabling expensive debug modes.
- Validates micro-bucket ring materialized views.

**Scope**

- Add per-shard micro-bucket rings, initially for simple count/rate views.
- Support fixed query shapes first:
  - top series over last N seconds;
  - rate/count by metric name or debug-view grouping key;
  - group by listener/origin where available;
  - exemplar lookup into the raw ingress ring when enabled.
- Keep local query windows independent from backend flush interval.

**Proof / acceptance criteria**

- Query results match an offline reference implementation over captured test samples.
- Query CPU/memory budgets are enforced and documented.
- Hot-path update overhead is measured with lookback disabled and enabled.
- Query APIs return bounded results and safe errors for unsupported shapes.

**Stop-safe state**

- If work stops here, DogStatsD has a useful local observability feature backed by bounded data structures.

**End-state cleanup enabled**

- Moves status/debug functionality toward queryable views instead of bespoke endpoint logic.

### Milestone 7: Payload-aligned columnar segments

**Value delivered**

- Tests whether recent lookback/export/storage can share work with metrics v3 payload construction.
- Reduces risk that internal descriptors drift away from serializer needs.

**Scope**

- Prototype sealed per-shard/per-time columnar segments aligned with metrics v3 concepts: dictionaries, tagsets, resources, origin info, timestamps, values, and sketches.
- Keep exact wire v3 encoding at the serializer boundary initially.
- Add a lightweight remap from internal dictionary IDs to payload-local v3 IDs if needed.

**Proof / acceptance criteria**

- Payloads generated from segments are semantically equivalent to existing serializer output.
- Benchmarks show whether payload building CPU/allocations improve enough to justify the format.
- Compressed payload size does not regress beyond an agreed threshold.
- If results are poor, the prototype can be removed without affecting prior milestones.

**Stop-safe state**

- If work stops here, we either have evidence-backed adoption of columnar segments or a clean decision not to carry the complexity.

**End-state cleanup enabled**

- Prevents a permanent second storage format unless it demonstrably replaces downstream payload-building work.

### Milestone 8: Semantic replay

**Value delivered**

- Adds deterministic reproduction for aggregation/debug/lookback behavior independent of transport parsing.
- Complements raw replay, which remains the transport-fidelity path.

**Scope**

- Persist canonical semantic records and descriptors, or persist enough versioned enrichment state to rebuild them deterministically.
- Make replay mode explicit: raw replay re-runs current parse/enrichment, semantic replay reuses captured semantic descriptors.
- Reuse the same descriptor identities as debug/lookback/serializer paths.

**Proof / acceptance criteria**

- Semantic replay over a captured corpus produces the same aggregate/debug/lookback outputs as the original run within documented tolerances.
- Tests cover changed tagger state to prove deterministic replay does not accidentally depend on current enrichment.
- Raw replay remains available for listener/parser/origin-detection investigations.

**Stop-safe state**

- If work stops here, replay becomes more useful for performance and correctness investigations.

**End-state cleanup enabled**

- Reuses the canonical descriptor pipeline instead of adding a replay-only representation.

### Milestone 9: Aggregator convergence and deletion of duplicate paths

**Value delivered**

- Completes the simplification arc: backend aggregation, debug stats, lookback, replay, and serialization share identity/descriptors where practical.
- Removes duplicated context-key generation and ambiguous semantics.

**Scope**

- Teach aggregator context resolution to consume pre-resolved descriptors/tag buffers where safe.
- Move effective backend identity into the shared identity model.
- Retire old batch/debug key generators once compatibility is proven.
- Decide whether backend aggregation itself becomes another materialized view over the canonical stream.

**Proof / acceptance criteria**

- Backend series output is equivalent to the old path across the Milestone 0 compatibility corpus.
- Performance is better or neutral under normal DogStatsD workloads.
- Old code paths, feature flags, and temporary adapters are deleted or have explicit removal tickets.
- Architecture docs show one canonical dataflow and name the remaining intentional identity distinctions.

**Stop-safe state**

- This is the point where the system should become simpler than today, not just more capable.

**End-state cleanup enabled**

- Removes transitional components and leaves one canonical ingest/descriptor/view architecture.

## Near-term concrete next steps

1. Map the current key computations:
   - batch shard key in `comp/dogstatsd/server/impl/batch.go`;
   - debug key in `comp/dogstatsd/serverDebug/impl/debug.go`;
   - aggregator context key in `pkg/aggregator/context_resolver.go`.

2. Add tests/benchmarks that expose differences between:
   - client tags only;
   - host extraction;
   - origin/tagger-enriched tags;
   - mapper-added tags;
   - tag filtering.

3. Prototype a `ResolvedSampleContext` carried alongside `metrics.MetricSample` through DogStatsD batching.

4. Convert `serverDebug` to consume that context behind the existing runtime setting.

5. Only after that, introduce the raw ingress ring and lookback window stores.

6. In parallel, compare `SeriesDescriptor`/lookback-bucket shapes against the metrics v3 serializer columns so the internal representation does not drift away from payload construction needs.

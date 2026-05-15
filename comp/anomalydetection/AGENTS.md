# comp/anomalydetection — AI Agent Guide

## What This Subtree Does

The `anomalydetection` subtree implements the agent-side anomaly detection
pipeline. Data flows through it as:

```
Handle → Storage → Detect → Correlate → Report
```

- **Handles** (non-blocking, copy-on-send) accept observations from the rest
  of the agent.
- The **observer engine** stores observations, runs detectors, and emits
  correlations.
- A **reporter** turns each detection-cycle output into outbound effects
  (developer trace, change events to Event Management).
- A **recorder** can capture scenarios to parquet for replay.

## Components

```
comp/anomalydetection/
  observer/      observer engine — pipeline orchestration, detectors, correlators
    def/ fx/ impl/
  reporter/      anomaly-event reporter (stdout trace + Event Management publisher)
    def/ fx/ fx-noop/ impl/ impl-noop/ mock/
    reporter.allium       ← behavioural specification
  recorder/      parquet recorder for scenario capture
    def/ fx-noop/ impl-noop/
  logssource/    container log source feeding the observer
    def/ fx/ impl/
  hfrunner/      high-frequency check runner
    def/ fx-noop/ impl-noop/
```

Directories that only ship `-noop` variants in this checkout have their
live implementations carried in downstream forks (the q-branch
testbench). Treat the `def/` package as the contract; treat any `-noop`
as "compiles and silently does nothing".

## Allium Specifications

Components in this subtree document their behaviour as **Allium**
specifications (`*.allium` files) written in the language documented at
[`.claude/skills/allium/`](../../.claude/skills/allium/SKILL.md).

The spec is authoritative for behaviour. If the code disagrees with the
spec, one of them is wrong. Open questions in the spec are unresolved
design ambiguities, not documentation — resolving them requires a
decision, not just code. Deferred specifications point at behaviour
governed by another component or another spec; they are not TODOs.

A change that alters component behaviour should change the spec in the
same PR — not as a follow-up.

**Litmus test for what belongs in a spec:** does it map directly to
code? Routing constants, payload shapes, deduplication semantics,
graceful-degradation rules — yes. Project-planning context (SME
coordination, rollout schedules, ticket numbers, ownership negotiations)
— no. Those live in PR descriptions, RFCs and AGENTS.md files, not in
the spec.

Validate a spec with the Allium CLI:

```bash
allium check comp/anomalydetection/reporter/reporter.allium
allium analyse comp/anomalydetection/reporter/reporter.allium
```

`check` reports parse/structural errors; `analyse` reports completeness
and reachability findings. Both emit JSON. A spec is considered
"green" when both report zero errors; remaining warnings (e.g.
`externalEntity.missingSourceHint` for entities whose governing spec
hasn't been written yet) are informational.

**Reading order when picking up a component:**

1. The component's `README.md`, if any — pipeline overview and extension
   guide.
2. The component's `*.allium` — behavioural spec (rules, contracts,
   invariants).
3. `def/component.go` — public interfaces.
4. `impl/` — implementation of the spec.

The Allium spec sits between the README and the implementation: it is
stricter than prose docs (machine-checkable rules, named invariants)
but intentionally less detailed than code (no string formatting, no
iteration order unless it's part of the contract).

## Key Design Decisions (subtree-wide)

### Data-driven scheduling ("complete seconds" rule)

Detection is NOT on a timer. When data at time T arrives, the engine
advances analysis to T-1. This ensures deterministic replay: same
data → same anomalies.

### Read-time aggregation

Storage keeps full summary stats (sum/count/min/max) per 1-second
bucket. Aggregation kind (avg, sum, count, min, max) is chosen when
reading, not when writing. Detectors can pick any aggregation without
re-ingesting data.

### Non-blocking ingestion

Handles do non-blocking sends to a buffered channel. If the channel is
full, observations are silently dropped. Analysis never
back-pressures data ingestion.

### One Report per advance cycle

The observer calls `Report(active_correlations, new_anomalies)` exactly
once per advance cycle on every subscribed reporter. Reporters must
not block the calling goroutine. The active-correlation set is
authoritative: reporters do not query observer internals to compute
deltas.

## Common Pitfalls

1. **Don't call engine methods from multiple goroutines.** The engine
   assumes single-threaded advance.

2. **Reporters and event sinks must not block.** Report and emit are
   synchronous; a blocking implementation stalls the entire ingestion
   loop.

3. **Detectors must not mutate storage.** They receive `StorageReader`
   (read-only). Violating this breaks deterministic replay.

4. **Extractor names must be unique.** The name is the storage namespace
   for derived metrics. Duplicates cause silent data collision.

5. **Routing identity of change events is locked.** integration_id,
   source_type_id, changed_resource_type, author_name, author_type and
   category are SME-frozen constants enforced by the
   `RoutingIdentityLocked` invariant in `reporter.allium`. Changing
   them at the code level without updating the spec — or vice versa —
   is a bug.

## Testing

```bash
dda inv test --targets=./comp/anomalydetection/...

# Per component
dda inv test --targets=./comp/anomalydetection/observer/...
dda inv test --targets=./comp/anomalydetection/reporter/...

# Benchmarks for the observer engine
dda inv test --targets=./comp/anomalydetection/observer/impl/ -- -bench=.
```

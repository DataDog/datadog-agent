---
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
external_references:
  - path: https://datadoghq.atlassian.net/wiki/spaces/~602449d8f3d296006864db68/pages/6495210537/Property+testing+Logs+Agent+Adaptive+Sampling
    why: Owner's design doc; defines sampling correctness + read-liveness properties.
  - path: https://datadoghq.atlassian.net/wiki/spaces/~712020006700eab4c247639d448c47103cd8b7/pages/6273073381/Logs+to+Disk+-+Payload+Journaling+Design
    why: Documents auditor offset tracking, backpressure drop points, and the catch-up problem.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/4437541188/RFC+Logs+Agent+Distributed+Senders
    why: Per-pipeline concurrency model; shared-sender proposal.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/6782419701/RFC-+Logs+Agent+Backpressure+Status
    why: Pipeline stages, backpressure propagation, rotation-related log loss.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/6505529378/Adaptive+Sampling+Architecture+and+Overview
    why: Adaptive sampling credit/token design.
lens: Coverage Balance
---

# Coverage Balance Evaluation — Logs Pipeline Property Catalog

36 properties across 9 categories. This document goes section by section through the
SUT analysis risk areas, flags gaps and imbalances in the catalog **as a portfolio**,
and assesses the property-type mix and cross-cutting coverage.

---

## 1. SUT-section-by-section gap analysis

### 1.1 Journald launcher — cursor recovery (UNCOVERED)

**SUT risk (sut-analysis §8, §11 item 4):** The journald launcher
(`pkg/logs/launchers/journald/`) fails gracefully on `setup()` error but has no
reconnect logic on mid-session failure. The sut-analysis explicitly names
"journald cursor recovery after ungraceful kill (no-gap/no-duplicate)" as a gap
in existing test coverage that Antithesis should fill. Cursor-based offset tracking
is structurally different from the file-byte-offset path covered by Categories B and C.

**Catalog coverage:** Zero properties mention journald. The deployment topology
explicitly defers journald to a later expansion ("No fourth container is needed:
container runtimes (Docker socket) and journald are secondary sources"). The
auditor-offset properties (B-cluster) cover the file-offset path but do not generalize
to journald cursors — the registry key format, the cursor semantics, and the failure
mode (mid-session socket disconnect, not file rotation) are all different.

**Gap severity:** Medium-high. Journald is a production input on every systemd host.
The mid-session reconnect gap means a journald source goes dark with no auto-restart —
a liveness violation. Bug history includes `55c63957d9f` (journald drops first entry
on restart). No catalog property catches this.

**Suggested action:** Add a `journald-cursor-recovery-no-gap` Liveness property:
assert `Sometimes("journald-reconnect-attempted")` after a mid-session socket
disconnect fault, and workload `Always(no sequence-number gaps after reconnect)`.
Requires adding a journald source container to the topology, or a Unix-socket fault
that severs the journald connection mid-stream.

---

### 1.2 TCP destination path — transport-specific offset and backoff rules (UNDERWEIGHTED)

**SUT risk (sut-analysis §7, §8, assumption A1 in catalog):** TCP is a separate code
path with different backoff constants (hardcoded `[2^(n-1), 2^n)` s, cap n=7 vs. HTTP's
configurable 1–120 s), a different offset-advance rule (TCP advances only on success,
not on permanent 4xx), and a known goroutine hazard (`handleServerClose` goroutine leak
on connection replacement). The catalog assumption A1 explicitly flags that
"TCP has different backoff constants and a different offset-advance rule."

**Catalog coverage:** One property nominally covers TCP: `transport-switch-no-loss`
(Category H), which tests the *switch* between TCP and HTTP. No property directly
tests TCP-only operation, the TCP backoff behaviour, or the TCP goroutine leak
(`sut-discovery/failure-modes.md F8.13`). Protocol properties in Category G are
all framed around HTTP status codes, which are inapplicable to TCP.

**Gap severity:** Medium. The topology defaults to HTTP (assumption A1), so TCP
properties are vacuous unless the topology enables TCP or the transport-switch is
exercised. But the TCP-specific offset rule (no 4xx advance) is a meaningful
correctness variant, and the `handleServerClose` goroutine leak (`connection_manager.go:125`)
is not a regression target in any property. The defer-cancel accumulation in the
TCP retry loop (`F8.12`) is also uncovered.

**Suggested action (targeted):** Add a `Reachable("tcp-connection-goroutine-exited-cleanly")`
sentinel to `no-goroutine-leak-after-stop` (E-cluster) conditioned on TCP mode. Add
a note to `auditor-offset-safety` that its current phrasing (4xx advances offset) is
HTTP-only; TCP does not advance on permanent error, so the invariant differs by
transport. A separate `tcp-permanent-error-no-offset-advance` Safety property is
warranted if TCP is ever in scope.

---

### 1.3 UDP/TCP socket listener sources — zero coverage

**SUT risk (sut-analysis §8):** The listener launcher
(`pkg/logs/launchers/listener/`) manages per-port TCP/UDP listener tailers.
These are structurally different from file tailers: no file offset, no registry
entry, no rotation concept. Connection loss or listener restart creates a different
loss/gap profile than file tailing.

**Catalog coverage:** Zero properties cover the listener launcher path. The topology
uses file tailing exclusively. The listener's per-connection tailers share the same
pipeline egress path (Categories B–G apply on the egress side), but the ingress
half — listener accept/close, per-connection lifecycle, UDP datagram loss on full
buffer — has no coverage.

**Gap severity:** Low-medium for the current topology (file tailing is the primary
path). Medium-high if the topology is later extended to syslog-over-TCP inputs.
UDP datagrams are inherently lossy and have no at-least-once guarantee — any
at-least-once property asserted over the full pipeline silently changes semantics
for a UDP source.

**Suggested action:** Document in the catalog's cross-cutting assumptions (A6 or
new A8) that all at-least-once, ordering, and offset properties apply only to file
sources. Add a `Reachable("listener-connection-teardown-clean")` Reachability
sentinel if the topology adds a syslog source. Until then this is a documented
topology scope decision, not a missing property.

---

### 1.4 Docker-socket tailer mid-session loss — no auto-restart (UNCOVERED)

**SUT risk (sut-analysis §8):** "mid-session socket loss exits the container tailer
goroutine with no auto-restart until the next launcher scan." This is a liveness
violation: after a Docker socket fault, container logs stop until the next scan
period (default 10 s). The scan gap can be longer under CPU throttle.

**Catalog coverage:** `container-collect-all-startup-race` (H) covers the
startup-race scenario. `no-goroutine-leak-after-stop` (E) targets goroutine
termination. But neither covers the case where the Docker socket drops mid-session,
the container tailer goroutine exits, and no new goroutine is started until the next
scan tick.

**Gap severity:** Medium. This is a liveness property that only fires in container
environments. The current topology is file-only so this is deferred topology scope.
But the failure is silent — no metric, no health-check failure.

**Suggested action:** If the topology is extended with a Docker-socket source, add a
`docker-socket-loss-tailer-recovers` Liveness property: after a Docker socket fault
clears, assert `Sometimes("container-tailer-restarted-after-socket-recovery")` within
one scan period. The absence of this property currently leaves a complete gap in
container-ingress liveness.

---

### 1.5 Decoder and parser malformed-input handling — UNCOVERED

**SUT risk (sut-discovery/failure-modes.md §F8.18; sut-analysis §7 item 6):**
`processor.go:198–215` silently drops messages when `msg.Render()` or
`encoder.Encode()` returns an error, with only a `log.Error` and no metric. The
sut-discovery explicitly identifies this as a silent drop path. Similarly, framer
malformed input (e.g., a line longer than the framer's hard limit produces a
truncated emit) is handled by `oversized-line-truncation-safe`, but structured
parse errors (JSON decode failure on a JSON-formatted log source) have no coverage.

**Catalog coverage:** `oversized-line-truncation-safe` (A) covers truncation.
`batch-encode-failure-no-silent-batch-loss` (G) covers batch-level encode failures.
But **processor-level render/encode drop** is not covered. The distinction matters:
a batch encode failure preserves replay (offset not advanced); a processor render
failure drops the message *before* it enters the batch, so neither `BytesMissed` nor
the offset is affected — the message vanishes with no audit trail.

**Gap severity:** Medium. Render/encode errors are rare under normal faults (they
need malformed UTF-8 or JSON marshalling bugs), but under Antithesis memory pressure
or crafted workload inputs they are reachable. The observability gap (no metric) makes
this a quiet drop that no other property catches.

**Suggested action:** Add a `processor-render-error-no-silent-loss` Safety property
(or extend `batch-encode-failure-no-silent-batch-loss` to cover the processor layer):
`AlwaysOrUnreachable("processor-render-error-drop")` with a SUT-side `Unreachable`
at `processor.go:199,213` in default operation; and a workload-side counter that any
render/encode drop is surfaced in a metric (currently absent — the workload cannot
detect it). The invariant is structural: render-drop never advances the auditor offset
(the message never reaches the strategy), so the offset safety is preserved but the
data is permanently gone.

---

### 1.6 Wildcard file ordering FIXME — `applyReverseLexicographicalOrdering` (UNCOVERED)

**SUT risk (sut-discovery/wildcard.md §6; sut-analysis §9 item 8):** A FIXME in
`file_provider.go:362` acknowledges that `applyReverseLexicographicalOrdering`
assumes `filepath.Glob` returns lexicographic results — an undocumented behaviour.
The `doublestar` library used for recursive globs makes no sorting guarantee. The
`flakes.yaml` entry for "Multiple Directories - Out of order input" names this exact
ordering bug as a *known skipped test* (`file_provider_test.go:645`).

**Catalog coverage:** Zero. No property in the catalog tests wildcard file priority
ordering, glob result ordering, or the correctness of `applyReverseLexicographicalOrdering`.

**Gap severity:** Low-medium. The impact is wrong-priority selection when at the
`filesLimit` cap — the wrong files get tailed, others are silently skipped. In
Antithesis this is reachable via a workload that creates many wildcard-matching files.
The existing skipped test confirms this is a known ordering bug with no guard, making
it a candidate for an Antithesis-discovered regression.

**Suggested action:** Add a `wildcard-file-priority-stable` Safety property:
workload creates N files matching a wildcard, always observes the top-priority K
files (by reverse-lexicographic name, or by mtime depending on the strategy) are
tailed. Add a `Reachable("applyReverseLexicographicalOrdering-invoked")` sentinel.
This doubles as a sentinel for the `filesLimit` cap being hit — without it, wildcard
properties are vacuous when the count is under the cap.

---

### 1.7 Worker-pool dynamic scaling correctness — underweighted

**SUT risk (sut-analysis §9 item 1; sut-discovery/failure-modes.md §F8.16,A5):**
The HTTP worker pool scales between `minWorkers` (= numPipelines) and
`maxWorkers` (= 10 × numPipelines) based on an EWMA of per-request latency vs.
a 150 ms target. Two failure modes: (a) backward clock jump freezes the EWMA
window (`time.Since(virtualLatencyLastSample) < 0` → no EWMA update → worker count
stuck at min); (b) forward clock jump causes a `time.Since` overshoot → single
over-large EWMA sample → worker count swings to max.

**Catalog coverage:** `clock-jump-no-backoff-underflow` (I) covers (a) with a
`Sometimes(l.inUseWorkers != l.minWorkers)` liveness sentinel. But the Safety
side — that a forward clock jump does not spike workers to max causing a stampede
of simultaneous sends — is not covered. The property's invariant clause focuses on
the backoff guard (`blockedUntil.After(time.Now())`), not worker-pool sizing.

**Gap severity:** Low-medium. Worker-pool over-scaling under a forward clock jump
doesn't lose data, but it can cause a retry storm against the intake when backpressure
clears. The missing invariant is `Always(inUseWorkers <= maxWorkers)` after a forward
jump — a soft correctness property that also protects the intake from O(N×M) request
bursts.

**Suggested action:** Extend `clock-jump-no-backoff-underflow` with a Safety
invariant: `Always(inUseWorkers <= maxWorkers)` and `Always(inUseWorkers >= minWorkers)`
at each EWMA update point in `worker_pool.go`. Also add `Sometimes(inUseWorkers > minWorkers)`
as a liveness sentinel to confirm the pool actually scales up (currently `Sometimes(l.inUseWorkers != l.minWorkers)` covers this, but the wording doesn't distinguish scale-up from scale-down).

---

### 1.8 Processor render/encode → diagnostic/stream-logs path (UNCOVERED)

**SUT risk (sut-discovery/failure-modes.md §F8.18; sut-analysis §8):** The
`diagnosticMessageReceiver.HandleMessage()` at `processor.go:205` and the
stream-logs / diagnostic path bypass the normal auditor offset tracking. The
`dae81c1a82e` bug history entry is "nil-guard stream-logs on shutdown." The
catalog property `logs-not-modified-in-transit` mentions this path as an
Antithesis angle (the diagnostic receiver race), but there is no property that
directly guards the **stream-logs endpoint** delivering correct output or not
panicking on shutdown.

**Catalog coverage:** `logs-not-modified-in-transit` (A) notes the diagnostic
path as a risk but its invariant focuses on content checksums in the main pipeline.
No property tests the stream-logs path directly.

**Gap severity:** Low in terms of data integrity (stream-logs is a debug/UI
feature, not a production delivery path). Medium in terms of shutdown safety
(nil-guard panic at shutdown is a correctness issue). One prior production bug
(`dae81c1a82e`) was exactly in this path.

**Suggested action:** Add `Unreachable("stream-logs-nil-dereference-on-shutdown")`
to `graceful-degradation-on-startup` or as a sub-point of `no-send-on-closed-on-shutdown`.
The diagnostic receiver race noted in `logs-not-modified-in-transit` should be promoted
to a SUT-side `AlwaysOrUnreachable` at `processor.go:205` (concurrent write to a
shared handler field under CPU throttle).

---

### 1.9 `KeepAlive`-for-beyond-limit files and registry bloat (UNCOVERED)

**SUT risk (sut-discovery/wildcard.md §7, §15):** `KeepAlive` is called for every
file matching a wildcard on each scan — including files beyond the `filesLimit` cap
that are never tailed. This causes the registry to accumulate stale entries without
bound (bounded only by the 23 h TTL). Separately, `KeepAlive` is a no-op for files
that have *never* been processed (`KeepAlive` only updates `LastUpdated` if the key
already exists), so files promoted from beyond the limit to within the limit start
tailing from end-of-file, silently skipping historical content.

**Catalog coverage:** Zero. No property covers registry entry count, TTL behavior,
or the `filesLimit` promotion semantics.

**Gap severity:** Low for a short-lived test run (TTL is 23 h). Medium for a
long-running Antithesis run with high file churn. The registry can grow to O(N × scan_period)
in the worst case, slowing flush and increasing crash-recovery time.

**Suggested action:** Add a `Reachable("registry-entry-count-at-filesLimit-cap")`
sentinel that fires when the number of unique registry keys exceeds `filesLimit`,
confirming the workload reaches this scenario. The promotion-from-beyond-limit
semantics should be documented as a known loss vector in `at-least-once-no-loss`'s
exceptions list.

---

## 2. Property-type mix analysis

### 2.1 Safety / Liveness / Reachability distribution

| Type | Count | % |
|------|-------|---|
| Safety (pure or Safety+) | 22 | 61% |
| Liveness (pure) | 5 | 14% |
| Reachability (sentinel, pure) | 1 | 3% |
| Mixed Safety+Liveness | 3 | 8% |
| Mixed Safety+Reachability | 3 | 8% |
| Liveness+Reachability | 2 | 6% |

**Finding: Liveness is underweighted.**

Five of the six SUT liveness guarantees listed in sut-analysis §5 (L1–L6) have
corresponding catalog coverage:
- L1 (every line eventually read): `at-least-once-no-loss` (B), `backpressure-no-rotation-loss` (F).
- L2 (queued payloads eventually sent): `queued-payloads-eventually-sent` (F).
- L3 (registry replay on restart): `registry-recovers-after-crash` (C).
- L4 (backpressure eventually clears): `queued-payloads-eventually-sent` (F).
- L5 (all lines reach intake): partly covered by `at-least-once-no-loss`.
- L6 (tailer eventually advances): **zero direct Liveness coverage.**

L6 ("tailer eventually advances; downstream stall propagation is unbounded in
duration but clears when downstream unstalls") has no direct property. It is
assumed to be implied by `queued-payloads-eventually-sent` and `backpressure-before-drop`
together, but those focus on the downstream (sender recovery) not the upstream
(tailer making forward progress after downstream clears).

**Finding: Reachability sentinels are thin in egress-half paths.**

The catalog has strong Reachability coverage for the auditor (`registry-recovers-after-crash`)
and the sampler (`sampling-reachable-under-load`), but no sentinel confirms that:
- The batch strategy's `batchWait` flush timer is actually exercised (not all flushes
  happen at size limit).
- The worker pool's non-blocking-send drop path (`NonBlockingSend` → secondary
  destination drop) is exercised.
- The TCP connection `handleServerClose` goroutine is exercised.
- The file provider's `filesLimit` cap is exercised.

Without these sentinels, Antithesis cannot confirm those code paths are live in the
workload — safety properties layered on top may pass vacuously.

---

### 2.2 Reachability deficit — sentinels needed to guard against vacuousness

The catalog has 4–5 pure `Reachable`/`Sometimes` sentinels:
- `sampling-reachable-under-load` — gates Category D.
- `registry-recovers-after-crash` (partial sentinel).
- `backpressure-no-rotation-loss` (partial reachability).
- `clean-shutdown-completes` (liveness, not a sentinel).

Missing gate sentinels for:
1. **Batch flush via timer** (vs. size limit) — without this, tests that assume timer
   expiry behavior pass vacuously if all batches hit the size limit first.
2. **Non-blocking send drop** — the secondary-reliable-destination drop path
   (`worker.go:160-164`) needs `Reachable("noop-sink-drop-executed")` to confirm
   the topology has multiple destinations configured.
3. **Rotation under active tailing** — `backpressure-no-rotation-loss` includes a
   `Reachable("bytes-missed-on-rotation")` but that is the *outcome* sentinel, not
   a *path* sentinel. Need `Reachable("closeTimeout-goroutine-fired")` separately to
   confirm the 60s timer goroutine executed.
4. **Failover-routing-triggered** — `per-source-ordering-preserved` mentions
   `Sometimes("failover-routing-triggered")` but A4 notes failover is off by default.
   Without this sentinel, the ordering-break path is exercised only when the custom
   fault flips the config.

---

## 3. Cross-cutting concerns that fall between focuses

### 3.1 Rotation + pipeline-reassignment + offset (the three-way interaction)

The catalog covers each pair:
- Rotation + offset: `container-identifier-no-collision` (B), `backpressure-no-rotation-loss` (F).
- Pipeline-reassignment + ordering: `per-source-ordering-preserved` (A).
- Rotation + multiline: `multiline-not-split-across-pipelines` (A).

But the **three-way interaction** — rotation causes pipeline reassignment AND the
old tailer's remaining messages go to pipeline N while the new tailer's go to
pipeline M, AND the auditor receives acks from both tailers interleaved with clock-skew
timestamps — is not captured by any single property. This is the exact compound
scenario that `wildcard.md §1` and `wildcard.md §9` both flag as high probability
under fault injection.

**Suggested action:** Add a cross-cluster bridge property `rotation-pipeline-reassignment-no-interleaving`:
Safety that after a rotation boundary, the intake sees all pre-rotation lines before
any post-rotation lines from the same logical file (workload-side reconciliation).
This is currently only partially covered by `per-source-ordering-preserved` which
focuses on within-pipeline order, not cross-pipeline cross-rotation order.

---

### 3.2 Adaptive sampling + clock + pattern-eviction (the three-way interaction)

The catalog covers each pair:
- Sampling + clock: `clock-jump-no-extra-sampling` (D/I).
- Sampling + pattern: `adaptive-sampler-no-aliasing` (D).
- Clock + pattern: implicitly covered by `sampling-exact-count`.

But pattern eviction *under a clock jump* is not covered: a forward clock jump
grants a credit windfall to all patterns simultaneously, and if the windfall causes
the sort order to flip (hot pattern moves to position 0, cold patterns rise), the
eviction picks a different victim than expected. This is the clock+aliasing combined
mode that neither `clock-jump-no-extra-sampling` nor `adaptive-sampler-no-aliasing`
tests alone.

**Suggested action:** Note in both properties' open questions that the combined
clock-jump + sort-reorder scenario is not covered by either property individually
and requires a joint test or a composed invariant.

---

### 3.3 Container-churn + identifier-collision + offset-regression (the compounding loop)

The catalog covers each:
- Container churn: `container-addremovesource-ordering` (E), `container-collect-all-startup-race` (H).
- Identifier collision: `container-identifier-no-collision` (B).
- Offset regression: `offset-no-regression-on-seek-error` (B).

But the **compound scenario** from wildcard.md §2 and §10 is: rapid container
churn → two tailers with the same identifier → clock skew → wrong offset wins →
on next restart the surviving tailer seeks to the wrong offset (triggering F8.6 if
Seek fails, or a duplicate storm if Seek succeeds but returns wrong bytes). This
compound loop — churn → collision → clock → seek → duplicate/loss — spans Clusters
2, 6, 7 and is not captured as a single property.

This is the most dangerous multi-cluster gap: each individual property can pass while
the compound scenario fails. Given that container-identifier collision is an explicit
FIXME and container churn is the dominant K8s path, the compound scenario deserves
its own Safety property.

**Suggested action:** Add `container-rotation-no-offset-revert-under-clock-skew`:
compound Safety property asserting that after a container rotation + clock fault,
the auditor offset for the surviving tailer is monotonically non-decreasing. This
directly tests the FIXME path with the clock fault that defeats the `IngestionTimestamp`
guard.

---

## 4. Component blind spots vs. the deployment topology

The topology uses **file tailing only** as the primary input. Given the catalog's
comprehensive coverage of the file-tailer egress path, the remaining blind spots
in the topology-as-designed are:

| Component | Catalog coverage | Gap type |
|---|---|---|
| File tailer (readForever, forwardMessages) | Strong | None for current topology |
| Decoder/framer | `oversized-line-truncation-safe`, `multiline-not-split-across-pipelines` | Processor-layer render/encode drop uncovered (§1.5) |
| Processor | `logs-not-modified-in-transit`, `secrets-redacted-before-send` | Diagnostic/stream-logs path partial (§1.8) |
| Batch strategy | `bounded-memory-under-backpressure`, `batch-encode-failure-no-silent-batch-loss` | Batch flush-via-timer sentinel missing (§2.2) |
| HTTP sender / worker pool | `clock-jump-no-backoff-underflow`, `permanent-error-no-retry`, `retryable-no-retry-after` | Worker-count Safety bound missing (§1.7), NonBlockingSend sentinel missing (§2.2) |
| TCP sender | `transport-switch-no-loss` (only) | TCP-specific offset rule, goroutine leak not covered (§1.2) |
| Auditor | Full cluster B+C | Strong; KeepAlive bloat sentinel missing (§1.9) |
| Adaptive sampler | Full cluster D | Strong; clock+eviction compound missing (§3.2) |
| Journald launcher | None | Complete gap (§1.1) |
| Listener launcher (TCP/UDP) | None | Deferred; documented scope decision |
| Container/Docker tailer | `container-*` properties | Mid-session socket loss uncovered (§1.4) |
| File provider (wildcard) | None | Ordering FIXME, filesLimit sentinel missing (§1.6, §1.9) |
| Diagnostic / stream-logs path | Partial (in `logs-not-modified-in-transit`) | Shutdown nil-guard uncovered (§1.8) |

---

## 5. Operational scenario coverage

### 5.1 Upgrade / version migration

`registry-format-migration-safe` (C) covers format-version migration. **Pass.**

### 5.2 Config-toggle failover

`per-source-ordering-preserved` (A) and `transport-switch-no-loss` (H) cover the
failover and transport-switch scenarios, gated by A4 and the `pipeline_failover`
config toggle. **Pass**, but note: both require a custom fault to flip the config;
the sentinels `Sometimes("failover-routing-triggered")` and `Reachable("transport-switch-TCP-to-HTTP")`
must be confirmed live before the Safety properties mean anything.

### 5.3 Fargate non-atomic write path

`registry-survives-crash` (C) explicitly covers the non-atomic (Fargate) path via
`DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false`. **Pass**, but gated on A3.

### 5.4 Partition → recovery → resumed delivery

`queued-payloads-eventually-sent` (F) and `backpressure-before-drop` (F) together
cover this scenario. **Pass.**

### 5.5 Node termination → registry recovery → resume tailing

Categories B and C together cover this. **Pass**, but requires node-termination fault
(A7) which is commonly off by default.

### 5.6 Agent rollback after failed transport switch

`transport-switch-no-loss` (H) covers this, including the rollback path
(`Reachable("transport-rollback-initiated")`). **Pass.**

### 5.7 Rapid container churn (Kubernetes pod recycling)

`container-addremovesource-ordering` (E), `container-identifier-no-collision` (B),
and `container-collect-all-startup-race` (H) cover the churn scenario piecemeal. The
compound three-way interaction (§3.3) is the gap.

### 5.8 High-frequency log rotation (log rotation under sustained network outage)

This is the headline scenario (§12.1 in sut-analysis, Cluster 1 in property-relationships).
`backpressure-no-rotation-loss` and `backpressure-before-drop` cover it. **Pass**,
but the `closeTimeout` sentinel (`Reachable("closeTimeout-goroutine-fired")`) is missing
from the catalog, meaning the exact loss-trigger code path may not be confirmed live.

---

## 6. Imbalance summary

### 6.1 Category D (Adaptive Sampling) is overweighted relative to its topology gate

Category D has 5 properties, all gated on `AdaptiveSampler` being enabled (A2). With
`NoopSampler` as the default, all 5 properties are vacuous unless the topology
explicitly enables the sampler. The `sampling-reachable-under-load` sentinel was
designed to surface this misconfiguration, but there is no catalog-level gate that
makes all of Category D automatically non-vacuous if A2 is not met. Given 5/36
properties (14%) behind a single non-default config toggle, any run without explicit
A2 confirmation wastes 14% of the catalog.

**Recommended mitigation:** In the run harness, make A2 (AdaptiveSampler) a
precondition check that fails the run setup if not confirmed, rather than a caveat
buried in the open questions of each property.

### 6.2 Category C (Crash Recovery) is gated on an off-by-default fault

Category C (3 properties: `registry-survives-crash`, `registry-recovers-after-crash`,
`registry-format-migration-safe`) plus several Category B properties all require the
node-termination fault (A7), which "is commonly disabled by default." If A7 is off,
8+ properties are vacuous. The deployment topology notes this and flags it, but the
catalog has no explicit count of how many properties become vacuous per fault type.

**Recommended mitigation:** Add a fault-availability table to the catalog preamble:

| Fault off | Properties vacuous |
|---|---|
| Node termination | `registry-survives-crash`, `registry-recovers-after-crash`, `registry-format-migration-safe`, `at-least-once-no-loss`, `auditor-offset-safety`, `no-loss-and-duplicate-same-line` = 6+ |
| Clock fault | `clock-jump-no-backoff-underflow`, `clock-jump-no-extra-sampling`, `multiline-not-split-across-pipelines` (clock variant), `container-identifier-no-collision` (clock skew variant) = 4 |
| AdaptiveSampler off | All of Category D = 5 |
| `pipeline_failover` off | `per-source-ordering-preserved` (failover path), partial `transport-switch-no-loss` = 2 |

Total: if both node-termination and clock faults are off (their typical default),
**10+ of 36 properties (28%) are vacuous**. This is the single largest portfolio risk.

### 6.3 Category E (Concurrency & Shutdown) — `no-services-store-deadlock` is dormant

`no-services-store-deadlock` (E) is explicitly noted as "dormant today — zero
production subscribers." The catalog is honest about this, calling it a "regression
guard." However, including a dormant-by-design property in a live test run consumes
coverage credit and may never produce a non-vacuous result. As a portfolio choice,
this property should either be marked `AlwaysOrUnreachable` explicitly (because the
deadlock loop iterates over empty slices) or moved to a "future coverage" annex until
a subscriber is reintroduced.

---

## 7. Findings summary

### GAP-1: Journald cursor recovery has zero properties
- Affected: No slug (catalog-wide gap for journald)
- Concern: Liveness and at-least-once gaps for mid-session journald disconnect
- Scope: Deferred by topology design, but named in sut-analysis §11 as an explicit Antithesis value-add
- Evidence: sut-analysis §8, §11 item 4; bug `55c63957d9f`
- Action: Add `journald-cursor-recovery-no-gap` Liveness + Reachability property; requires topology extension

### GAP-2: TCP transport path underweighted (offset rule differs, goroutine leak uncovered)
- Affected: `transport-switch-no-loss` (only), `no-goroutine-leak-after-stop` (partial)
- Concern: TCP-specific offset-advance rule (no 4xx advance), `handleServerClose` goroutine leak
- Scope: Medium; current topology uses HTTP only; TCP path is exercised only via transport-switch
- Evidence: sut-discovery/failure-modes.md §F8.12, §F8.13; catalog A1
- Action: Extend `no-goroutine-leak-after-stop` with a TCP-mode Reachable sentinel; note TCP offset rule asymmetry in `auditor-offset-safety`

### GAP-3: Processor render/encode drop — silent loss without offset advance, no metric
- Affected: No slug (gap between `oversized-line-truncation-safe` and `batch-encode-failure-no-silent-batch-loss`)
- Concern: `processor.go:198–215` drops messages silently with no counter; distinct from batch-level encode failure
- Scope: Structurally reachable under memory pressure; silent — no other property catches it
- Evidence: sut-discovery/failure-modes.md §F8.18; sut-analysis §7 item 6
- Action: Add `processor-render-error-no-silent-loss` Safety property or extend batch-encode property to cover processor layer

### GAP-4: Wildcard file ordering FIXME is an uncovered, known bug with a skipped test
- Affected: No slug (catalog-wide gap for file provider wildcard path)
- Concern: `applyReverseLexicographicalOrdering` assumes lexicographic glob output (FIXME); skipped test `file_provider_test.go:645`
- Scope: Reachable via workload creating many wildcard-matching files near `filesLimit`
- Evidence: sut-discovery/wildcard.md §6; flakes.yaml comment
- Action: Add `wildcard-file-priority-stable` Safety property + `Reachable("filesLimit-cap-reached")` sentinel

### GAP-5: Docker-socket mid-session loss has no liveness property
- Affected: No slug (topology deferred; gap documented)
- Concern: Container tailer goroutine exits on socket loss with no auto-restart; silent dark period
- Scope: Deferred by topology design; medium severity in container environments
- Evidence: sut-analysis §8 (Docker socket dependency); deployment-topology.md "No fourth container is needed"
- Action: Document as topology-scope deferral in catalog; add `docker-socket-loss-tailer-recovers` Liveness when topology expands

### GAP-6: Compound rotation+pipeline-reassignment+offset interaction — falls between clusters
- Affected: `per-source-ordering-preserved`, `container-identifier-no-collision`, `backpressure-no-rotation-loss` (each covers a pair, none covers the triple)
- Concern: Three-way interaction (rotation → pipeline reassignment → interleaved auditor acks with clock skew) not captured by any single property
- Scope: High — "any rotation" probability under Antithesis; wildcard.md §1 and §9 flag this as high-impact
- Evidence: sut-discovery/wildcard.md §1, §9; property-relationships.md Cluster 1 + Cluster 2
- Action: Add `rotation-pipeline-reassignment-no-interleaving` Safety property (workload reconciliation of pre/post-rotation line ordering at intake)

### GAP-7: Container-churn+identifier-collision+clock compound — multi-cluster loop
- Affected: `container-identifier-no-collision`, `container-addremovesource-ordering`, `offset-no-regression-on-seek-error`
- Concern: Rapid churn → collision → clock skew defeats `IngestionTimestamp` guard → wrong offset survives → Seek-to-wrong-offset on restart
- Scope: High in K8s environments; the FIXME at `tailer.go:260` is explicit
- Evidence: sut-discovery/wildcard.md §2, §10; sut-analysis §9 item 6
- Action: Add `container-rotation-no-offset-revert-under-clock-skew` compound Safety property

### GAP-8: 28% of catalog properties vacuous when default faults are off
- Affected: catalog-wide (node-termination off: 6+; clock off: 4; AdaptiveSampler off: 5; failover off: 2)
- Concern: Portfolio-level risk — a run with default fault config silently skips more than a quarter of the catalog
- Scope: Structural/portfolio; the topology flags this but no summary table makes the exposure concrete
- Evidence: catalog assumptions A1–A7; deployment-topology.md "Fault dependencies"
- Action: Add a fault-availability table to the catalog preamble quantifying vacuousness per fault type; treat A2 (AdaptiveSampler) and A7 (node-termination) as run-time precondition checks, not open questions

### GAP-9: Reachability sentinels missing for key egress-path code branches
- Affected: `backpressure-no-rotation-loss` (missing `closeTimeout-goroutine-fired`), `bounded-memory-under-backpressure` (missing `nonblocking-send-drop-executed`), Category D (batch-flush-via-timer not confirmed live)
- Concern: Safety properties over branches that may never be reached in the workload → vacuous pass
- Scope: Medium; affects 3–4 properties
- Evidence: property-relationships.md §2.2 (sentinels); sut-analysis §3 (channel buffer sizes)
- Action: Add `Reachable("closeTimeout-goroutine-fired")` to `backpressure-no-rotation-loss`; add `Reachable("noop-sink-nonblocking-drop")` to `bounded-memory-under-backpressure`; add `Reachable("batch-flush-via-timer")` to Category F

### GAP-10: Liveness for tailer forward progress (L6) has no direct property
- Affected: No slug (falls between `queued-payloads-eventually-sent` and `at-least-once-no-loss`)
- Concern: No property asserts the tailer itself makes forward progress (advances `lastReadOffset`) after downstream unblocks; L6 is implied but not tested
- Scope: Low-medium; indirectly covered by `at-least-once-no-loss` end-to-end
- Evidence: sut-analysis §5 (L6); property-relationships.md Cluster 1
- Action: Add a SUT-side `Sometimes("tailer-lastReadOffset-advanced-after-backpressure-clear")` to `backpressure-before-drop` as a tailer-progress sentinel

### IMBALANCE-1: Category D (Adaptive Sampling) — 14% of catalog behind a non-default config gate
- Affected: `sampling-exact-count`, `high-value-never-sampled`, `sampling-reachable-under-load`, `clock-jump-no-extra-sampling`, `adaptive-sampler-no-aliasing`
- Concern: Disproportionate investment behind A2 (AdaptiveSampler); vacuous if topology omits sampler config
- Scope: Portfolio; 5/36 properties
- Evidence: catalog A2; `sampling-reachable-under-load` open question
- Action: Treat A2 as a hard run precondition; document as such in the topology config

### IMBALANCE-2: `no-services-store-deadlock` is a dormant-API property with no current live path
- Affected: `no-services-store-deadlock`
- Concern: Zero production subscribers → property is vacuous in any realistic topology; provides false coverage confidence
- Scope: Single property; the catalog is transparent about it but does not flag it as non-billable coverage
- Evidence: catalog E, property evidence file; "zero production subscribers" note
- Action: Move to a "regression guard — currently vacuous" annex; or rephrase as `AlwaysOrUnreachable("services-store-subscriber-blocked")` with explicit note that it fires only when subscribers are reintroduced

---

## 8. Passes

The following SUT risk areas are adequately covered by existing catalog properties:

- **Auditor offset safety and durability** (Cluster 2 + Cluster 3): strong coverage with appropriate Safety/Liveness mix and Reachability sentinels (`registry-recovers-after-crash`, `registry-survives-crash`).
- **Backpressure and rotation-loss as the headline scenario** (Cluster 1): `backpressure-no-rotation-loss` + `backpressure-before-drop` together partition the rotation/no-rotation fault space correctly.
- **Shutdown concurrency hazards H1–H6** (Cluster 5): each named hazard has a corresponding property (`no-send-on-closed-on-shutdown`, `idempotent-stop`, `no-goroutine-leak-after-stop`, `auditor-drains-on-stop`, `clean-shutdown-completes`).
- **Protocol error classification** (Cluster 9): `permanent-error-no-retry` + `retryable-no-retry-after` + `batch-encode-failure-no-silent-batch-loss` form a complete trio.
- **Sampling cluster gate** (`sampling-reachable-under-load` as sentinel): correctly identified as the gate property for Category D.
- **Clock faults and sampler/backoff** (Cluster 7 + D): `clock-jump-no-backoff-underflow` and `clock-jump-no-extra-sampling` address the two named failure modes.
- **Registry format migration** (`registry-format-migration-safe`): operational upgrade scenario explicitly covered.
- **Secret redaction**: `secrets-redacted-before-send` correctly uses `Unreachable` type for the bypass path.
- **Property-relationships (clusters and bridges)**: the cross-cluster bridge analysis in `property-relationships.md` is sound and matches the SUT risk picture; the auditor offset as the central hub is correctly identified.

---

## 9. Uncertainties

1. **Does the Antithesis clock fault move Go's monotonic clock?** This is flagged in
   the catalog (Cluster 7 open question) but the answer gates 4 properties.
   Resolution changes the effective vacuousness count.

2. **Is the Docker socket or journald container included in the actual test topology?**
   The deployment topology explicitly excludes them, but if they are later added,
   GAP-1 (journald) and GAP-5 (Docker socket) become immediately high priority.

3. **Can the `filesLimit` cap be hit in the planned workload?** If the workload does
   not create enough wildcard-matching files, GAP-4 (wildcard ordering) and the
   registry-bloat variant (§1.9) remain unreachable.

4. **Is `processor.go:198–215` render/encode failure reachable under Antithesis
   standard faults (network/CPU-pause), or only under OOM-level pressure?** The
   sut-analysis suggests it needs OOM; if so, GAP-3 has the same `AlwaysOrUnreachable`
   character as `batch-encode-failure-no-silent-batch-loss` and should be phrased
   accordingly rather than as a gap.

5. **Adaptive sampling warmup period and per-interval re-evaluation semantics**
   (catalog open question in Category D): the owner's design doc has not clarified
   these. If there is a warmup period, `sampling-exact-count` must exclude the
   warmup window from its invariant, or it will produce false failures at run start.

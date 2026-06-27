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
---

# Property Relationships

Suspected clusters, shared mechanisms, and dominance among the 42 active cataloged
properties (plus 2 DROPPED clock properties, retained but out of scope per B-CLOCK). Lightweight — flags connections noticed during synthesis, not a formal
analysis. "Dominance" = if property X holds, Y is likely implied (so a failing Y is
the more interesting signal, and X can sometimes be derived from Y's machinery).

## Cluster 1 — Backpressure & the rotation-loss chain (THE headline)

Shared mechanism: sender slows → bounded channels fill → tailer blocks → behavior
diverges depending on whether a file rotates and whether `closeTimeout` expires.

- `backpressure-no-rotation-loss` (loss happens, bounded, observable via `BytesMissed`)
- `backpressure-before-drop` (the inverse: *no* loss when there's no rotation)
- `queued-payloads-eventually-sent` (recovery progress after the partition clears)
- `clean-shutdown-completes` (the same full-channel block, but triggered at shutdown)
- `at-least-once-no-loss` (umbrella; the rotation path is its documented exception)

Dominance: `backpressure-before-drop` + `backpressure-no-rotation-loss` together
characterize the loss boundary; `at-least-once-no-loss` is the umbrella both feed.
The same full-`outputChan` block underlies `clean-shutdown-completes`. Same fault
recipe (network partition ± file rotation) drives all five — share the workload.

## Cluster 2 — Auditor offset correctness & duplicate/loss semantics

Shared state: the persisted registry offset; shared hazard: offset wrong-direction.

- `auditor-offset-safety` (offset never *ahead* of durable data → no silent loss)
- `container-identifier-no-collision` (offset never *regresses* → no duplicate storm)
- `offset-no-regression-on-seek-error` (a specific regression cause: seek→0)
- `auditor-drains-on-stop` (undrained `inputChan` at Stop → stale offset → duplicates)
- `no-loss-and-duplicate-same-line` (compound: the worst-case where both directions fail)

Dominance: `no-loss-and-duplicate-same-line` is the strongest/compound property — it
can only pass if `auditor-offset-safety` and the regression properties all hold under
a combined fault. A failure there localizes to one of the others. All five read the
same registry; the workload reconciles fakeintake-delivered sequence numbers against
the recovered offset.

## Cluster 3 — Crash recovery & registry durability

Shared dependency: node-termination fault (A7); shared artifact: `registry.json`.

- `registry-survives-crash` (file integrity after kill — atomic vs Fargate non-atomic)
- `registry-recovers-after-crash` (correct reload; missing/corrupt fallback reachability)
- `registry-format-migration-safe` (v0/v1→v2 migration preserves entries)

Dominance: `registry-survives-crash` is a precondition for `registry-recovers-after-crash`
(can't recover correctly from a corrupt file). `registry-format-migration-safe` shares
the same write/recover machinery on an upgrade path. These feed `at-least-once-no-loss`
(Cluster 1) and `auditor-offset-safety` (Cluster 2) on the restart path.

## Cluster 4 — Adaptive sampling correctness

Shared subsystem: `pkg/logs/internal/decoder`/sampler (`sampler.go`); gated on A2.

- `sampling-reachable-under-load` (sentinel: the drop path is exercised at all)
- `sampling-exact-count` (≤ N low-value per interval)
- `high-value-never-sampled` (protected logs always pass)
- `adaptive-sampler-no-aliasing` (pattern-table resort doesn't corrupt accounting)
- ~~`clock-jump-no-extra-sampling`~~ — **DROPPED (B-CLOCK)**, no longer an active member.

Dominance: `sampling-reachable-under-load` dominates the cluster — if it fails (drop
path never hit), the others pass vacuously. Run it as a gate.

## Cluster 5 — Shutdown concurrency safety

Shared bug class: shutdown-time channel/lifecycle races (CPU-pause/thread faults).

- `no-send-on-closed-on-shutdown` (panic guardrail)
- `idempotent-stop` (double-close guardrail)
- `no-goroutine-leak-after-stop` (everything exits)
- `auditor-drains-on-stop` (also in Cluster 2 — bridges concurrency and offset)
- `clean-shutdown-completes` (also in Cluster 1 — the liveness side of shutdown)

Dominance: `clean-shutdown-completes` is the liveness umbrella; the three guardrails
(`no-send-on-closed-on-shutdown`, `idempotent-stop`, `no-goroutine-leak-after-stop`) are safety conditions
that a clean shutdown implies. A guardrail failure is a sharper localization than a
generic "shutdown hung." Same fault lever (pause a goroutine mid-shutdown) drives all.

## Cluster 6 — Container/source lifecycle & autodiscovery races

Shared mechanism: async source/service add-remove, `Services` store mutex, AD timing.

- `no-services-store-deadlock` (mutex held during blocking send → deadlock/starvation)
- `container-addremovesource-ordering` (fire-and-forget Add/Remove ordering holes)
- `container-collect-all-startup-race` (wrong-metadata / gap window during reschedule)
- `per-source-ordering-preserved` (rotation reassignment is one of its break paths — bridges to Cluster 8)

Dominance: `no-services-store-deadlock` is the most severe (silently stops all new
container tailing); the other two are correctness/metadata issues. All three need a
container-churn workload and CPU-pause faults; share that setup.

## Cluster 7 — Clock-sensitive timing  *(DROPPED — user decision B-CLOCK)*

This cluster is **dropped**. `clock-jump-no-backoff-underflow` and
`clock-jump-no-extra-sampling` are removed from the active catalog; the clock
*variants* of `multiline-not-split-across-pipelines` (now rotation-only) and
`container-identifier-no-collision` (now scheduling-driven) are removed. Reason: Go
timing uses the monotonic clock, and the Antithesis clock fault is likely
wall-clock-only — leaving these vacuous. Both survivors moved fully into Clusters 8
and 2 respectively.

## Cluster 8 — Data fidelity (content, order, structure)

Shared concern: the bytes/structure the user wrote must arrive intact.

- `per-source-ordering-preserved` (order; also Cluster 6 via rotation)
- `logs-not-modified-in-transit` (content bytes)
- `secrets-redacted-before-send` (content must be transformed *before* leaving)
- `oversized-line-truncation-safe` (structure: truncation correctness)
- `multiline-not-split-across-pipelines` (structure: event boundaries; also Cluster 7)

Dominance: largely independent invariants (no strong dominance), but they share the
same workload instrumentation — every emitted line carries a per-source sequence
number + checksum + structural markers so the intake can check order, content,
redaction, truncation, and multiline integrity from one stream.

## Cluster 9 — Protocol contract (intake error handling)

Shared subsystem: HTTP destination retry/classify (`http/destination.go`).

- `permanent-error-no-retry` (4xx → drop once, don't loop)
- `retryable-no-retry-after` (429 → retry, don't reclassify to permanent)
- `batch-encode-failure-no-silent-batch-loss` (encode error → reset, don't lose silently)
- `transport-switch-no-loss` (lifecycle, but shares the destination/sender machinery)

Dominance: `permanent-error-no-retry` and `retryable-no-retry-after` are complementary
halves of correct status-code classification; both feed `auditor-offset-safety`
(Cluster 2) since misclassification corrupts when the offset advances.

## Cross-cluster bridges (single mechanism, multiple clusters)

- **The auditor offset** is the hub: Clusters 1 (loss), 2 (offset), 3 (recovery),
  5 (drain-on-stop), 9 (when offset advances) all converge on it. It is the most
  instrumentation-worthy single component.
- **`closeTimeout` / rotation** links Cluster 1 (loss), Cluster 8 (multiline split),
  and Cluster 2 (identifier collision).
- **The clock fault** links Cluster 7, Cluster 4 (sampling), and Cluster 2 (timestamp
  guard).
- **CPU-pause/thread-scheduling faults** drive Clusters 5 and 6 and widen the windows
  in 1, 2, and 8 — the cheapest fault to exercise broadly.

## Addendum — evaluation gap-fill + scope-decision properties

Where the 8 properties added after the evaluation pass attach:

- **`rotation-pipeline-reassignment-no-interleaving`** → bridges Cluster 1
  (rotation/backpressure), Cluster 2 (offset regression), and Cluster 8 (ordering).
  It is the explicit *triple* (rotation × pipeline-reassignment × offset) that the
  pairwise properties left uncovered; dominated-by-none, dominates none.
- **`processor-render-error-no-silent-loss`** → Cluster 2 (offset-not-advanced
  guarantee) + Cluster 9 sibling of `batch-encode-failure-no-silent-batch-loss`
  (same "drop without advancing offset" shape, different layer).
- **`log-metadata-not-corrupted`** → Cluster 8 (data fidelity); corrects the catalog's
  loss-bias. Shares the workload instrumentation (per-line tags) with the other
  fidelity properties.
- **`wildcard-file-ordering-stable`** → Cluster 6 (file/source selection) + Cluster 8;
  shares the file-provider machinery with the rotation/ordering properties.
- **`journald-cursor-recovery-no-gap`** → Cluster 3 (crash recovery) analogue for the
  cursor-based (non-byte-offset) source; needs the container/journald topology.
- **`mrf-unreliable-destination-drop-bounded`** → Cluster 1/9 (the unreliable-destination
  `NonBlockingSend` drop path, S5); dominated-by `auditor-offset-safety` for the
  primary at-least-once guarantee. Needs the second-intake topology.
- **`tcp-permanent-error-no-offset-advance`** → Cluster 2 + Cluster 9; the TCP twin of
  `auditor-offset-safety` / `permanent-error-no-retry`, exposing the HTTP/TCP
  offset-advance asymmetry. Needs the TCP topology variant.
- **`tcp-connection-goroutine-no-leak`** → Cluster 5 (leaks/lifecycle); TCP analogue of
  `no-goroutine-leak-after-stop`. Needs the TCP topology variant.

## Suggested test-suite gates (run-order implications)

1. `sampling-reachable-under-load` gates all of Cluster 4 (else vacuous).
2. `backpressure-before-drop` + `backpressure-no-rotation-loss` should be measured
   together — they partition the same fault space (rotation vs no-rotation).
3. `registry-survives-crash` is a precondition for the rest of Cluster 3.

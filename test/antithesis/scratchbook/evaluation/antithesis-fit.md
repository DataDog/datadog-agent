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

# Antithesis Fit Evaluation — Logs Pipeline Property Catalog

Adversarial review through the Antithesis Fit lens. Bias is finding problems.
A property "passes" when it genuinely requires state-space exploration that
deterministic tests cannot reach, the assertion type matches how Antithesis will
observe the property, and its value is not vacuous under the planned fault set.

---

## Structured Summary

### Findings

---

#### F1 — `adaptive-sampler-no-aliasing`: Antithesis adds less value than claimed; better fit is a fuzz test

| Field | Detail |
|---|---|
| **Slugs** | `adaptive-sampler-no-aliasing` |
| **Concern** | Assertion-type mismatch + wrong tool for the job |
| **Scope** | Property-specific |

The evidence file confirms `Process()` is single-goroutine by design (no mutex,
no concurrent access). The aliasing hazard is intra-goroutine pointer aliasing
through slice element value-swaps inside a deterministic sequential loop. The
bug is a pure sequential algorithm correctness issue: "all field mutations must
happen before the bubble loop." Antithesis's CPU-pause and thread-scheduling
faults are irrelevant to a single goroutine. The state space that needs
exploring is sequences of token patterns that drive specific bubble configurations
(entry at position 2+, certain matchCount relationships), and the existing
`sampler_test.go:264` already covers the canonical `sampled` aliasing case.

The catalog says "Antithesis adds value by exploring many interleaving patterns
(even within a single goroutine, via CPU scheduling and branch exploration)."
This is not accurate: CPU scheduling does not affect single-goroutine algorithm
correctness. What would find new aliasing bugs is a property-based fuzz test
over `AdaptiveSampler.Process()` input sequences — exactly what Go's
`testing/fuzz` or `rapid` is designed for.

The `Always("sampler-sampled-count-tag-matches-dropped-count")` workload
assertion is a good property, but it's checkable with a deterministic test that
drives specific pattern orderings. Antithesis can run it, but it is not
exercising anything the Antithesis search strategy specifically finds that a
focused fuzz test would miss.

**Suggested action:** Downgrade the "Antithesis angle" claim. Keep the
workload-side assertion as a regression guard (low cost, genuine correctness
value), but deprioritize this property for search budget. File a parallel
recommendation to add a fuzz test over `AdaptiveSampler.Process()` sequences.

---

#### F2 — `batch-encode-failure-no-silent-batch-loss`: `AlwaysOrUnreachable` is the honest call, but the catalog undersells the structural observation it protects

| Field | Detail |
|---|---|
| **Slugs** | `batch-encode-failure-no-silent-batch-loss` |
| **Concern** | Assertion-type label correct but the reachability story is understated; the catalog's framing obscures the property's real value |
| **Scope** | Property-specific |

The evidence file confirms (and the catalog correctly states) that the
encode-error path is unreachable under standard Antithesis faults — it requires
OOM-level memory pressure that is not part of the planned topology's fault set.
The catalog's `AlwaysOrUnreachable` choice is therefore honest.

However, the catalog buries the property's only real defensive value — the
structural `Always` that after an encode error, `outputChan <- payload` is NOT
called, so offsets are not advanced and messages will replay on restart. This is
an invariant worth instrumenting regardless of reachability, because it is the
firewall preventing a silent loss mode. Instrumenting `AlwaysOrUnreachable`
at the encode-error drop site protects that structural guarantee; if a future
code change accidentally sends the payload to `outputChan` on an error path,
the `AlwaysOrUnreachable` would turn into a failing `Always`.

The current description ("value is the structural guarantee that a failed batch
doesn't advance offsets") is correct, but the property entry lists this as
mostly self-contained with low urgency. It should be flagged as "instrument for
regression defense, not for fault exploration" and the `Reachable` assertion
suggestion in the evidence file's "Assertion design" section should be removed
— it would be a permanently failing assertion under the planned topology.

**Suggested action:** Remove the `Reachable("batch-encode-error-drop")`
suggestion from the evidence file (it is unreachable under planned faults).
Retain and promote the structural `AlwaysOrUnreachable` at each encode-error
drop site as a regression guard. Add a note: this property is not a search-budget
consumer; it costs nothing to instrument and protects a silent-loss boundary.

---

#### F3 — `sampling-reachable-under-load`: Correctly identified as a sentinel, but the "vacuous cluster" problem is understated catalog-wide

| Field | Detail |
|---|---|
| **Slugs** | `sampling-reachable-under-load`, `sampling-exact-count`, `high-value-never-sampled`, `clock-jump-no-extra-sampling`, `adaptive-sampler-no-aliasing` |
| **Concern** | Entire Category D is conditionally vacuous under the topology default; catalog documents this but doesn't flag the severity |
| **Scope** | Catalog-wide (Category D) |

The evidence file and the catalog (cross-cutting assumption A2, open question
in `sampling-reachable-under-load`) confirm that `NoopSampler` is the default
and no config key auto-enables `AdaptiveSampler`. The topology document
(deployment-topology.md) says `AdaptiveSampler` must be "explicitly
constructed" — but it is not clear that this construction is actually planned
or that the topology document treats it as a hard prerequisite rather than an
aspiration.

If the topology ships with `NoopSampler` (the path of least resistance), all
five Category D properties — covering the sampler's core credit logic, the
important-log protection contract, the clock-negative-credit bug, and the
aliasing regression — pass vacuously because the sampler never runs. The
`Reachable` assertions in `sampling-reachable-under-load` would fire as
failures ("assertion never reached"), which is the right sentinel — but only
if it is instrumented before the run. No SUT-side assertions exist yet (clean
slate per `existing-assertions.md`).

**The catalog is honest but insufficiently alarmed.** The note about A2 is buried
in a cross-cutting assumptions table, not visually flagged as a prerequisite
gate that renders a fifth of the catalog vacuous. The open question in
`sampling-reachable-under-load` uses `(needs human input)` without urgency.

**Suggested action:** Elevate A2 to a blocking prerequisite in the catalog
header. Add a pre-flight check: before any Antithesis run, assert that
`tlmAdaptiveSamplerDropped` increments at least once within the first N minutes
of workload execution. If it doesn't, abort and fix the topology. This check
should be a `Reachable` on the drop path placed as the first assertion installed.

---

#### F4 — `no-services-store-deadlock`: Antithesis value is UNDERestimated in the wrong direction — the property is latent/dormant, not live

| Field | Detail |
|---|---|
| **Slugs** | `no-services-store-deadlock` |
| **Concern** | Property marked as "dormant" but the catalog hedges about its value; in fact it is not a live property at all under the default topology |
| **Scope** | Property-specific |

The catalog body correctly states the deadlock loop iterates over empty slices
today (zero production subscribers). The evidence file confirms: "The deadlock
path described in the property body is currently unreachable." Under the planned
topology (file-based log sources, no container runtime), `Services` subscribers
are zero. The deadlock requires a subscriber goroutine to stall — and there are
none in the planned topology.

The catalog's "Note: dormant today (no production subscribers)" is accurate, but
the property is still listed as an active catalog entry with full assertion
machinery, consuming reviewer attention and potentially SUT instrumentation
effort. The correct categorization is: this is a **code smell / API hazard
regression guard** — valuable to document but not a runtime Antithesis property
under the current topology. It should not appear in an Antithesis run unless the
topology adds container-runtime sources with `Services` subscribers.

The catalog also undersells the genuinely useful secondary finding in the merged
evidence: the **duplicate-service-delivery via `GetAddedServicesForType` goroutine
race** (services.go:70-87). That race does involve concurrent goroutines and
could produce real observable effects (two tailers for one container). This is
the finding worth keeping as an active property, with scope narrowed to the
goroutine-race variant.

**Suggested action:** Split this property. Move the deadlock variant to a
"latent/future" section. Promote the duplicate-delivery goroutine race
(`GetAddedServicesForType` replay goroutine vs. concurrent `AddService`) to an
active property if the topology uses container sources; otherwise archive it.

---

#### F5 — `registry-format-migration-safe`: Primarily a unit/integration test; Antithesis adds minimal marginal value

| Field | Detail |
|---|---|
| **Slugs** | `registry-format-migration-safe` |
| **Concern** | Integration-test territory; the interesting paths are deterministic |
| **Scope** | Property-specific |

The evidence file confirms that the v0/v1/v2 migration logic is in-memory,
idempotent, and has no logic bugs. The version dispatch is a plain switch
statement. The `Reachable("v1-migration-path-taken")` assertion requires the
topology to pre-seed a v1 registry file — a deterministic setup step. Once
seeded, the migration runs once, deterministically, at startup. The only
timing-sensitive interaction is the "node termination during the first
post-migration flush on the Fargate non-atomic writer" — but this interaction is
already covered by `registry-survives-crash` (which addresses the non-atomic
write corruption under kill-9 for all flushes, not just the first). There is no
unique state-space this property explores that `registry-survives-crash` doesn't
already cover.

The `Unreachable("unknown-version-empty-registry")` assertion is genuinely
valuable as a regression guard (protects the switch's default branch), but this
is a one-line instrumentation that belongs alongside the existing migration code
regardless of Antithesis.

The two topology open questions (`(needs human input)`) about running two agent
versions sequentially are architectural prerequisites that the planned topology
document does not address. Pre-seeding a v1 registry is a workload-setup concern,
not an Antithesis state-space concern.

**Suggested action:** Deprioritize for search budget. Add the
`Unreachable("unknown-version-default-branch")` assertion unconditionally as a
cheap regression guard. The `Reachable` assertions require topology pre-seeding
that is not guaranteed. Document that this property is "integration-test shaped"
and should be covered by a standard unit/integration test first.

---

#### F6 — `per-source-ordering-preserved`: The `Sometimes("failover-routing-triggered")` assertion is vacuous if `pipeline_failover.enabled` stays false

| Field | Detail |
|---|---|
| **Slugs** | `per-source-ordering-preserved` |
| **Concern** | Config-gated `Sometimes` assertion will never fire under the default topology |
| **Scope** | Property-specific |

The evidence file confirms `pipeline_failover.enabled` defaults to false. The
topology document (deployment-topology.md) lists it as "toggled per scenario via
custom faults / config variants" — meaning it is not on by default in the
topology. The `Sometimes("failover-routing-triggered")` assertion requires the
failover path to be exercised; if `pipeline_failover.enabled=false`, the
`forwardWithFailover` goroutine is never started and the assertion can never be
reached, causing the Antithesis run to report it as a failing reachability
assertion ("never reached").

The rotation-boundary ordering break (Break #2) is reachable without enabling
failover — file rotation on `pipeline count ≥ 2` lands old and new tailers on
different pipelines. This is the more reliably triggerable of the two break
paths. The topology pins `pipelines: 2`, so this path is live.

**Suggested action:** Gate the `Sometimes("failover-routing-triggered")`
assertion behind a compile-time or runtime condition that checks
`pipeline_failover.enabled`. Alternatively, mark it as a separate topology
variant (a scenario explicitly enabling failover). Ensure the baseline topology
uses the rotation-boundary path as the primary ordering-break surface. The
overall property remains high-value for Antithesis — the rotation-break path
needs CPU scheduling to produce the reorder window.

---

#### F7 — `clock-jump-no-extra-sampling` and `clock-jump-no-backoff-underflow`: Both may be fully vacuous if the Antithesis clock fault only affects wall-clock

| Field | Detail |
|---|---|
| **Slugs** | `clock-jump-no-extra-sampling`, `clock-jump-no-backoff-underflow` |
| **Concern** | Critical unresolved uncertainty: Go's `time.Sub` uses monotonic component; if Antithesis clock faults only move wall-clock, both properties are unreachable |
| **Scope** | Property-specific (shared uncertainty) |

Both properties depend on the Antithesis clock fault affecting `time.Now()` in
a way that causes `elapsed` (a `time.Duration` computed via `now.Sub(lastSeen)`)
to be negative (`clock-jump-no-extra-sampling`) or causes `blockedUntil.After(time.Now())`
to be false when it should be true (`clock-jump-no-backoff-underflow`).

Go's `time.Now()` on Linux VDSO includes a monotonic clock reading. The
`time.Sub` and `.After()` methods use the monotonic component when both operands
have it, ignoring wall-clock. This means:

- A backward wall-clock jump (e.g., via `CLOCK_REALTIME` manipulation) does NOT
  make `now.Sub(lastSeen)` negative if the monotonic component advances normally.
- Only a backward monotonic clock jump (via `CLOCK_MONOTONIC` manipulation or
  a hypervisor-level tick rollback) would produce a negative elapsed time.

The catalog acknowledges this uncertainty at both properties with `(needs human
input)`, and the `clock-jump-no-extra-sampling` evidence file's investigation log
confirms the production code uses `time.Now()` directly (not overridable at
runtime). The topology document flags clock faults as "often default-OFF."

This is not just a "confirm with tenant" uncertainty — it is a binary gate:
if the Antithesis clock fault only moves `CLOCK_REALTIME` and not
`CLOCK_MONOTONIC`, neither property is reachable, and their `Always` assertions
pass vacuously (no triggering condition). Conversely, if monotonic clock is
affected, the properties are high-value (the `clock-jump-no-extra-sampling`
negative-credit bug is confirmed and unmitigated).

**Suggested action:** Make resolving the monotonic-clock question a blocking
prerequisite for including either property in a run. Add a pre-flight test:
inject a backward clock and measure `time.Since(startTime)` in a small probe
process — if it returns a negative duration, monotonic clock is affected. Until
resolved, mark both properties as "pending clock fault characterization" and do
not include them in the default run configuration.

---

#### F8 — `at-least-once-no-loss`: The `Sometimes` assertion is a liveness check — it needs an explicit recovery window protocol

| Field | Detail |
|---|---|
| **Slugs** | `at-least-once-no-loss` |
| **Concern** | Liveness assertion type on a property that requires a fault-quiet recovery window; catalog mentions this but the mechanics are underspecified |
| **Scope** | Property-specific |

The property's invariant is `Sometimes("all-sequence-numbers-received-after-quiet-period")`.
This is a liveness property that only becomes observable after faults stop and
the pipeline drains. The catalog notes "Needs a recovery window
(`eventually_` or `ANTITHESIS_STOP_FAULTS`)."

The problem: `Sometimes` fires as soon as the condition is true once during the
run. If the sequence-number reconciliation assertion runs continuously in
parallel while faults are active, it will never be satisfied (some sequence
numbers will always be in-flight). If it only runs after `ANTITHESIS_STOP_FAULTS`,
the assertion succeeds on every run that has any quiet window — not specifically
after the fault-induced rotation-loss scenario.

The property needs a **structured two-phase protocol**:
1. Phase 1 (faults active): workload writes N lines, rotates the file under
   network partition, records which lines were written before the rotation.
2. Phase 2 (after `ANTITHESIS_STOP_FAULTS`): reconcile against fakeintake.
   Assert lines 1..N either appear in fakeintake or are documented-lost via
   `BytesMissed > 0` (rotation loss) or 4xx permanent drop.

Without the two-phase protocol, the `Sometimes` assertion is either always
satisfied (trivially, during a no-fault window) or always unsatisfied (under
continuous faults). Neither is useful.

Note also: `BytesMissed` resets on restart (in-memory `expvar.Int`) — the
workload must read it before each agent kill, or use the Prometheus
`TlmBytesMissed` counter instead. The evidence file resolves this but the
catalog's invariant statement does not reflect it.

**Suggested action:** Replace the bare `Sometimes` with a structured
`eventually_` command in the workload test that: (a) injects rotation under
partition, (b) stops faults, (c) waits for quiet-period, (d) reconciles. Add
explicit documentation that `BytesMissed` must be read before any agent kill
if used as a loss-confirmation signal.

---

#### F9 — `multiline-not-split-across-pipelines`: The rotation-path split is a confirmed deterministic bug, not a timing race — the `Sometimes` and `Always` framing is misleading

| Field | Detail |
|---|---|
| **Slugs** | `multiline-not-split-across-pipelines` |
| **Concern** | Incorrect assertion type for the primary failure path |
| **Scope** | Property-specific |

The evidence file investigation confirms: `stopForward()` fires before
`readForever`'s deferred `decoder.Stop()` runs. This means any partial
multiline event at rotation time is **deterministically discarded** — it is not
a race but a structural ordering invariant. The conclusion is in the catalog's
open questions: "in `StopAfterFileRotation()`, `stopForward()` cancels
`forwardContext` before `readForever`'s deferred `decoder.Stop()/Flush()` runs,
so a partial multiline event at rotation is deterministically discarded."

The catalog lists `SUT-side Sometimes` that the multiline Flush() path during
rotation is exercised. But if the partial event is deterministically discarded
(not flushed), this `Sometimes` fires precisely when the bug is not triggered
(when no partial event is in-flight at rotation). The `Sometimes` assert does
not catch the bug; the workload-side `Always` (every event has both BEGIN and
END markers) is the right assertion, and it will fail deterministically on
a topology that rotates files under any conditions.

The clock-jump timer-flush split path (the merged content from
`multiline-flush-timeout-no-split-events`) is genuinely timing-sensitive and
benefits from Antithesis — but only if the clock fault affects Go's monotonic
timer (same uncertainty as F7).

**Suggested action:** Split the rotation path from the clock-jump path explicitly.
For the rotation path: replace the `Sometimes("multiline-flush-exercised")` with
`Reachable("multiline-flush-at-rotation")` to confirm the path is taken; the
workload `Always` over BEGIN/END markers is the actual correctness check and
will surface the structural bug. For the clock-jump timer-flush path: gate on
the monotonic-clock uncertainty resolution (same as F7).

---

#### F10 — `offset-no-regression-on-seek-error`: Fault injection may not be injectable via the planned topology's shared-volume

| Field | Detail |
|---|---|
| **Slugs** | `offset-no-regression-on-seek-error` |
| **Concern** | The fault required (injecting `Seek()` errors on an OS file) may be uninjectable in the planned container topology |
| **Scope** | Property-specific |

The evidence file notes: "Can `Seek` errors be injected in the planned topology?
(partial: needs a real/faultable FS; afero MemMapFs may not inject seek errors)."
The planned topology uses a shared volume for log files (deployment-topology.md).
Standard Antithesis filesystem faults operate at the block/device level or via
LD_PRELOAD interceptors. Whether `Seek` syscalls can be made to return errors on
a shared container volume depends on the Antithesis fault implementation — this
is distinct from the more commonly supported `open()/read()` fault injection.

If `Seek` errors cannot be injected, this property is unreachable despite the
`Unreachable` assertion implying the bad path should never happen (which it
won't — vacuously). The property would appear to pass but provides no testing
value.

The bug itself (discarded error in `tailer_nix.go:36`) is confirmed and real,
and the fix is straightforward. This suggests the right action may be to fix
the bug first (handle the `Seek` error by aborting the tailer or using offset 0
with an explicit warning), then use an Antithesis filesystem fault to verify the
handling — but only after confirming the fault is injectable.

**Suggested action:** Classify this property as "fault-injection feasibility
unclear." Escalate the question to the Antithesis tenant: can `lseek()` syscalls
be made to return EINVAL/EIO on the shared volume? If yes, this property is
high-value (confirms a real silent bug). If no, file a code fix for the
discarded error and remove the property from the active catalog.

---

#### F11 — Catalog-wide: Clock-fault-dependent properties lack a unified gating mechanism

| Field | Detail |
|---|---|
| **Slugs** | `clock-jump-no-extra-sampling`, `clock-jump-no-backoff-underflow`, `multiline-not-split-across-pipelines` (timer path), `clock-jump-no-backoff-underflow` |
| **Concern** | Four properties across categories C, D, I depend on clock faults being (a) enabled and (b) affecting Go's monotonic clock; no catalog-level gate exists |
| **Scope** | Catalog-wide |

Cross-cutting assumption A7 mentions "clock faults often disabled by default —
confirm with tenant." But A7 does not distinguish between the two failure modes:
(a) clock fault disabled entirely → all clock-dependent `Always` assertions pass
vacuously; (b) clock fault enabled but only affects wall-clock → same vacuity for
properties using `time.Sub` (which uses the monotonic component).

The catalog has three distinct clock-dependent property clusters: adaptive
sampling credit overflow (D), backoff guard bypass (I), and multiline
timer-flush (A). All three have the same open question about monotonic clock
behavior and all three would produce vacuous passes if the fault is absent or
affects only wall-clock.

**Suggested action:** Add a "Clock Fault Pre-flight" section to the catalog (or
topology document). Before any run that includes clock-dependent properties,
execute a probe: inject a backward clock, measure `time.Since()` and
`time.Now().Sub(savedTime)` in the SUT, and verify negative values are
observable. If the probe fails, exclude all clock-dependent properties from the
run's assertion set. This prevents silently vacuous passes.

---

#### F12 — `registry-survives-crash` and `registry-recovers-after-crash`: Strong Antithesis fit, but the non-atomic path requires an explicit topology variant that is not confirmed

| Field | Detail |
|---|---|
| **Slugs** | `registry-survives-crash`, `registry-recovers-after-crash` |
| **Concern** | The most dangerous failure mode (non-atomic Fargate path) requires `DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false`; the planned topology document is ambiguous about whether this variant is implemented |
| **Scope** | Property-specific |

The catalog notes (A3) that the non-atomic path requires
`DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false`. The topology document says "toggled
per scenario via custom faults / config variants." The evidence file for
`registry-survives-crash` opens with "Will the topology simulate Fargate (force
non-atomic)? `(needs human input)`." The existing assertions file confirms clean
slate — no existing instrumentation.

If the topology ships with the atomic writer (the default), the `Reachable("non-atomic-writer-path-taken")`
assertion fails ("never reached") and the most dangerous crash scenario is not
tested. Meanwhile, the atomic-rename path is itself crash-safe by design; the
node-termination fault against the atomic writer only exercises the less
dangerous "last flush may be missing" recovery path, which is lower severity.

The registry-related properties have strong Antithesis fit (node termination +
timing-sensitive 1-second flush window). But the Fargate variant is the one
that can cause mass data loss — it must be an explicit, confirmed topology
variant, not a deferred "(needs human input)."

**Suggested action:** Treat the non-atomic topology variant as a required test
scenario, not optional. Add it as a separate row in the topology document with
explicit configuration: `DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false`, and note
that `Reachable("non-atomic-writer-path-taken")` is a canary assertion that must
be reached or the scenario is misconfigured.

---

#### F13 — `high-value-never-sampled`: Unit-test-shaped; Antithesis value is modest

| Field | Detail |
|---|---|
| **Slugs** | `high-value-never-sampled` |
| **Concern** | The property's failure scenario is primarily a tokenizer correctness issue, which is fully covered by existing deterministic tests |
| **Scope** | Property-specific |

The evidence file confirms: `TestAdaptiveSampler_ImportantLogBypassesRateLimit`
directly covers the "zero remaining credits + important tokens" case. The
tokenizer is case-insensitive (confirmed by code). The `isImportant()` check
precedes the credit path — the protection is not clock-sensitive (confirmed). The
only realistic Antithesis scenario ("tokenizer misclassification under CPU fault")
is described as hypothetical: "CPU is throttled by Antithesis... concurrent GC
pause corrupts the token slice — not a realistic Go GC scenario." The token slice
is a plain `[]Token` allocated and consumed within a single `Process()` call with
no shared mutable state.

This is a well-designed feature with robust unit test coverage. Antithesis would
need to find a tokenizer bug that deterministic inputs miss, but the tokenizer has
no concurrency, no shared state, and no timing-sensitive paths. The `Sometimes`
("protection path reached at least once") is meaningful as a run canary confirming
the `AdaptiveSampler` is active, but the actual correctness check (`Always`: every
FATAL line reaches fakeintake) is verifiable with a simple deterministic workload.

**Suggested action:** Keep this property as a workload-side `Always` correctness
assertion (zero cost once the sampler is active). But downgrade the Antithesis
unique-value claim. Treat the `Sometimes("tlmAdaptiveSamplerProtected.Inc()"`
as a sampling-cluster canary (same role as `sampling-reachable-under-load`),
not an independently valuable property.

---

### Passes

The following properties are well-fit for Antithesis exploration and have no
material evaluation concerns:

**`no-send-on-closed-on-shutdown`** — Shutdown races (H4) are timing-driven and
the specific interleaving (forwarder blocked on InputChan while Stop() closes it)
requires scheduling precision that Antithesis's CPU-pause provides. The panic is
confirmed reachable; the `Unreachable` assertion type is correct. Confirmed clean.

**`auditor-drains-on-stop`** — The Go `select` non-determinism between a closed
channel with buffered items and competing timer ticks is exactly the class of bug
Antithesis's scheduler exploration finds. The H5 race is confirmed present in the
current code (not drained on `Stop()`). The `Sometimes("auditor-run-loop-exited-with-buffered-payloads-remaining")`
is the right assertion type — it will fire when the race triggers. Confirmed clean.

**`backpressure-no-rotation-loss`** — The rotation-under-network-partition loss
path is the highest-impact unexercised failure mode. It requires a compound fault
(partition + rotation under backpressure) with precise timing (close_timeout
window). This is exactly what Antithesis's fault composition explores. The
`Reachable("bytes-missed-on-rotation")` + workload sequence-gap check is the
right structure. Confirmed clean.

**`no-goroutine-leak-after-stop`** — The `WrappedSource` fire-and-forget Add/Remove
goroutines (confirmed by evidence: no join mechanism) under container churn create
genuine goroutine leaks that accumulate over long runs. The combination of CPU-pause
faults, workload churn, and pprof-based goroutine count verification is appropriate
for Antithesis's long-run model. Confirmed clean.

**`container-identifier-no-collision`** — Two tailers sharing the same registry
key during rotation (FIXME confirmed in code) with IngestionTimestamp as the only
conflict resolver creates a real data-race at the auditor. The scheduling of which
tailer's last payload arrives at the auditor first is exactly what CPU-pause faults
control. The `AlwaysOrUnreachable` assertion noting that offset must not regress
unless rotation is in-progress is correctly typed. Confirmed clean; note topology
must use container sources or accept that this is exercised via the file-rotation
path (same code, same bug).

**`no-loss-and-duplicate-same-line`** — The compound fault (4xx + CPU throttle +
SIGKILL) creating a window where a 4xx-dropped payload has its in-memory offset
advanced but the registry hasn't flushed is confirmed (evidence: "there is no
flush-before-drop path — the ≤1s crash window applies to every `output <- payload`,
including 4xx drops"). This specific compound fault is exactly the class of
multi-fault interaction Antithesis finds. Confirmed clean; gated on node-termination
fault being enabled (A7).

**`clean-shutdown-completes`** — The circular wait (saturated outputChan +
shutdown arrival while the pipeline stops draining) confirmed by evidence and
bug history (94d7ccbfc35, 7041f901670). Network partition filling outputChan
then shutdown is a fault composition requiring timing; Antithesis's network
fault + CPU scheduling together exercise it. Liveness assertion
`Sometimes("shutdown-completed-within-30s")` is correctly typed (liveness,
not safety). Confirmed clean.

**`secrets-redacted-before-send`** — This property has genuine Antithesis value
as a code-path exploration property. Its purpose is to confirm no fault-induced
code path bypasses `applyRedactingRules()`. The bypass paths are all confirmed
safe by code inspection, so the realistic Antithesis value is regression defense
against a future code change that adds a new send path. The workload-side
`Always` (no unredacted secret in fakeintake) is the correct outer check; the
SUT-side `Unreachable` at any bypass would be the inner guard. Confirmed clean.

**`backpressure-before-drop`** — The `Always(BytesMissed == 0)` during a
no-rotation sustained partition is a clean safety property with clear triggering
(network partition) and a concrete observable (BytesMissed). Antithesis's network
fault exercising the steady-state backpressure path is the right fit.
Confirmed clean.

**`auditor-offset-safety`** — The 1-second flush window between `output <- payload`
and the registry flush creates a real timing window where node termination can
cause offset-ahead-of-durable-data. This needs the node-termination fault (A7);
gated on that. Confirmed clean when that fault is enabled.

**`bounded-memory-under-backpressure`** — The zstd C-heap leak (confirmed past
bug `0d9dfc76f46`) is only detectable in a long-run soak under repeated
`resetBatch()` cycles — exactly Antithesis's strength (long run with fault
repetition). The RSS-monitoring `Always` is appropriately workload-side since
Go GC stats don't show C-heap growth. Confirmed clean; note requires CGO build.

**`permanent-error-no-retry`** — The `AlwaysOrUnreachable` at the
permanent-error classify site is correct. Fakeintake returning 4xx exercising
the classify path is straightforward fault injection. The pipeline-stall risk
from a misclassified 4xx→retry is the real Antithesis find. Confirmed clean.

**`container-addremovesource-ordering`** — The fire-and-forget Add/Remove
goroutines with no join (confirmed by evidence) create a real ordering hole
under CPU-pause faults. The interleaving (pause Add goroutine, let Remove run,
resume Add → permanent orphan) is reachable via Antithesis scheduling.
Confirmed clean.

**`idempotent-stop`** — Confirmed latent hazard: no `sync.Once` guard on
`Sender.Stop()` or `provider.Stop()`. The concurrent signal-handler + API-stop
path is plausible. The `Unreachable` assertion around `close()` calls is the
right type. Confirmed clean; the double-Stop path is narrow but Antithesis
scheduling can widen it.

**`queued-payloads-eventually-sent`** — Retry + recovery after partition is a
liveness property that Antithesis's fault-stop model (`ANTITHESIS_STOP_FAULTS`)
directly supports. The `Sometimes("destination-transitions-retrying-to-not-retrying")`
is correctly typed. Confirmed clean.

**`transport-switch-no-loss`** — `partialStop()` confirmed to drop in-flight
payloads (evidence). The CPU throttle during `partialStop` widening the H2 flush
race is timing-sensitive and benefits from Antithesis scheduling. The `Reachable`
assertions for switch and rollback paths are correctly typed. Confirmed clean.

---

### Uncertainties

**U1 — Node-termination fault availability.** Seven properties across categories
B, C require kill-9 and registry recovery to be testable (A7: "node-termination
and clock faults are commonly disabled by default"). The catalog flags this but
does not provide a mechanism to detect if they are enabled. If node-termination
is off, approximately 7 properties pass vacuously. Recommended: add a
`Reachable("agent-process-restarted-after-kill")` assertion in the workload that
will fail the run if node-termination is never triggered.

**U2 — Monotonic clock fault behavior.** Affects F7 (two properties) plus the
timer-flush variant of multiline-not-split. Resolution is a blocking prerequisite
for those properties. See F7 above.

**U3 — `container_collect_all` and container-source topology.** Two category H
properties (`container-collect-all-startup-race`, `no-services-store-deadlock`,
`container-identifier-no-collision`) require container sources. The planned
topology uses file-based sources only. If container sources are added later,
these properties activate. Currently they should be marked topology-gated.

**U4 — fakeintake per-payload status code recording.** Multiple properties (A6)
need fakeintake to record HTTP response codes per payload. The topology document
notes this as a "prerequisite work" item. If not implemented, `no-loss-and-duplicate-same-line`
and `auditor-offset-safety` cannot detect 4xx-offset-advance cases at the
workload layer.

**U5 — `close_timeout` and workload run duration.** `backpressure-no-rotation-loss`
requires the pipeline to be blocked for 60 seconds (the default `close_timeout`)
to trigger the loss path. If the Antithesis run duration is short, the timeout
may never fire. The topology document does not pin `close_timeout`; recommend
pinning to a lower value (10–15s) to make the loss path reachable within a
typical Antithesis scenario duration.

**U6 — CGO vs. nocgo zstd build.** `bounded-memory-under-backpressure` requires
the CGO zstd build to reproduce the C-heap leak. The planned SUT build may use
the nocgo fallback (`pkg/util/compression/impl-zstd-nocgo`), in which case the
specific leak path is unreachable and the property only tests Go-heap RSS
behavior (lower severity). The catalog's open question on this is unresolved.
Recommend pinning `CGO_ENABLED=1` in the SUT Dockerfile and verifying
`pkg/util/compression` selects the CGO strategy.

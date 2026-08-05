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

# Logs Pipeline — Antithesis Property Catalog

**42 active properties** (+2 retained-but-DROPPED clock properties), synthesized from
a 5-agent property-discovery ensemble (focuses: data integrity, idempotency/replay,
concurrency, distributed coordination, failure recovery, lifecycle, resource
boundaries, protocol contracts, wildcard) over the Datadog Agent logs pipeline, then
stress-tested by a 4-lens evaluation pass (`evaluation/`) and revised per the user's
scope decisions (Categories J/K added; clock properties dropped; container + TCP +
adaptive sampling in-scope). Every SUT-side assertion is **net-new** — there is no
Antithesis SDK in the repo (see `existing-assertions.md`). Each property has an
evidence file at `properties/{slug}.md`.

## Cross-Cutting Assumptions

A reviewer changing any of these must re-audit the affected properties.

- **A1 — Transport.** The default test topology uses the **HTTP** destination. TCP
  has different backoff constants and a different offset-advance rule (TCP advances
  the auditor offset only on success; HTTP advances on success *and* permanent 4xx).
  Properties citing HTTP codes are TCP-inapplicable unless noted.
- **A2 — Sampling.** `NoopSampler` is the default; the adaptive-sampling properties
  (Category D) require the topology to explicitly enable `AdaptiveSampler`. Without
  it they are vacuous.
- **A3 — Registry write mode.** `atomic_registry_write` is assumed default-true; the
  non-atomic (Fargate) corruption path requires `DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false`.
- **A4 — Failover.** `pipeline_failover.enabled` is off by default; the failover
  ordering break requires enabling it.
- **A5 — Pipeline count.** Cross-pipeline ordering/duplication surface only exists
  when `logs_config.pipelines > 1`. Pin the topology to ≥2 to exercise it.
- **A6 — Fakeintake.** The workload embeds a per-source **sequence number** + interval
  clock + content checksum + multiline markers in each line so
  order/sampling/loss/redaction/structure are reconstructable at the intake; the
  **sequence number is the dedup/correlation key** (not body equality). **Prerequisite
  (eval finding):** the existing `test/fakeintake` stores each payload *before*
  applying its `ResponseOverride` status and does not record the code returned per
  payload, so today "retried" vs "dropped" vs "benign replay" are indistinguishable at
  the query layer. Fakeintake must be extended to (a) record the response code per
  payload and (b) preserve store-vs-respond ordering. Until then the
  4xx/offset/at-least-once family must use SUT-side telemetry (e.g.
  `DestinationLogsDropped`) as the delivery oracle.
- **A7 — Faults required.** Crash/recovery properties require the **node-termination**
  fault; clock properties require the **clock** fault. Both are commonly disabled by
  default — confirm with the tenant (see `faults.md`).

## Fault / Config Gate Matrix (vacuity exposure)

A run with default faults/config silently skips a large fraction of the catalog.
Treat the rows below as **hard run preconditions**, not soft notes — assert each
gate (or a canary) before relying on the gated properties.

| Gate | Default | Properties gated (vacuous if unmet) |
|---|---|---|
| **node-termination fault** | often OFF | registry-survives-crash, registry-recovers-after-crash, registry-format-migration-safe, auditor-offset-safety, at-least-once-no-loss, no-loss-and-duplicate-same-line |
| ~~clock fault~~ **(DROPPED — B-CLOCK)** | n/a | clock-jump-no-extra-sampling and clock-jump-no-backoff-underflow are dropped; the clock *variants* of container-identifier-no-collision and multiline-not-split are removed (both survive via scheduling/rotation, not the clock fault) |
| **`AdaptiveSampler` enabled** (no config key auto-enables it) | OFF (NoopSampler) | sampling-exact-count, high-value-never-sampled, sampling-reachable-under-load, adaptive-sampler-no-aliasing (clock-jump-no-extra-sampling is DROPPED) |
| **`pipeline_failover.enabled`** | false | per-source-ordering-preserved (failover-reorder reachability), no-goroutine-leak-after-stop (H4 variant) |
| **`logs_config.atomic_registry_write=false`** | true | registry-survives-crash (Fargate non-atomic path) |
| **container/journald source + `container_collect_all`** | **IN SCOPE (B-CONTAINER)** — topology provides it | container-collect-all-startup-race, container-addremovesource-ordering, journald-cursor-recovery-no-gap |
| **MRF / second intake** | **IN SCOPE (B-CONTAINER)** — topology provides a 2nd intake | mrf-unreliable-destination-drop-bounded |
| **TCP transport variant** | **IN SCOPE (B-TRANSPORT)** — topology runs a TCP variant | tcp-permanent-error-no-offset-advance, tcp-connection-goroutine-no-leak |
| **fakeintake response-code recording** (A6 prereq) | not present — must extend | permanent-error-no-retry, retryable-no-retry-after, auditor-offset-safety, no-loss-and-duplicate-same-line |

---

## Category A — Data Integrity & Ordering

The bytes a user wrote must reach the intake unmodified, in order, redacted, and
neither silently split nor silently dropped. Several are *documented* guarantees
with *undocumented* break paths.

### per-source-ordering-preserved — Per-Source Order Preserved Within a Pipeline

| | |
|---|---|
| **Type** | Safety |
| **Property** | Log lines from a single tailing *session* (one tailer over one file generation) arrive at the intake in write order. Cross-session order (across a rotation, or under failover) is **not** guaranteed and is tested only for reachability. |
| **Invariant** | `Always`: within one session, per-source sequence numbers are non-decreasing at fakeintake. `Sometimes("failover-routing-triggered")` + `Sometimes("rotation-pipeline-reassignment-taken")` confirm the (config-gated / rotation) reorder paths are *reached* — documenting the weaker real guarantee rather than asserting an `Always` the code structurally violates. All missing. (Reframed per eval R-ORDERING.) |
| **Antithesis Angle** | Network partition → backpressure → `forwardWithFailover` (`provider.go:372`) spreads consecutive messages across pipelines; file rotation → new tailer round-robins to a different pipeline (`launcher.go:663`). CPU throttle exposes the non-atomic routing window. |
| **Why It Matters** | Per-source ordering is a stated guarantee (README:156). Two undocumented paths (failover mode, rotation boundary) break it silently, corrupting multiline assembly and trace/log correlation. |

**Open Questions:**
- **Confirmed:** `pipeline_failover.enabled` defaults to **false**; `router_channel_size` defaults to 5 (reorder window ≤5 msgs). Is failover ever enabled in any *deployed* config? `(needs human input)`
- Does the HTTP intake preserve submission order for sequential payloads on one connection? `(needs human input)`
- What pipeline count will the topology use (≥2 needed to exercise cross-pipeline reorder)? `(needs human input)`

### logs-not-modified-in-transit — Content Not Modified After Processing

| | |
|---|---|
| **Type** | Safety |
| **Property** | A log line's byte content at the intake is identical to its content after the processor's redaction/render/encode step. |
| **Invariant** | `Always`: a workload-embedded checksum validates in the received payload at fakeintake. SUT-side `Always` at `processor.go:218` (`outputChan <- msg`). Missing. |
| **Antithesis Angle** | CPU throttle exposes a race between `diagnosticMessageReceiver.HandleMessage()` (`processor.go:205`) and the strategy reading `msg`. Compression-context corruption (cf. zstd bug `0d9dfc76f46`) corrupts encoded bytes after they're set. |
| **Why It Matters** | Corruption is invisible at delivery — the intake accepts corrupted bytes without error. |

**Open Questions:**
- None. (Investigated: the buffered message receiver only enqueues a pointer, never writes `msg` fields; `SetContent` stores a slice ref but the pre-redaction bytes become unreachable; the batch copies `MessageMetadata` by value. The workload CRC check remains the right test approach.)

### secrets-redacted-before-send — Secrets/PII Redacted Before Transmission

| | |
|---|---|
| **Type** | Safety |
| **Property** | No log line matching a `MaskSequences` rule reaches the intake with its original (unredacted) content. |
| **Invariant** | Workload-side `Always`: no received body at fakeintake contains the sentinel secret pattern. SUT-side *positive* `Always` at the processor output that, when the source has `MaskSequences` configured, emitted content differs from raw input. (Dropped the earlier `Unreachable("any bypass path")` per eval R-SECRETS — a runtime assertion can't prove the absence of a bypass; Antithesis value is catching a *new* bypass introduced during exploration.) Missing. |
| **Antithesis Angle** | Code-path exploration finds any send route that bypasses the processor (a launcher variant, an encode-error early-exit, the diagnostic-receiver race). |
| **Why It Matters** | Secret leakage is the highest-severity correctness failure here (GDPR/HIPAA). A silent bypass under a rare interleaving is exactly what Antithesis can surface. |

**Open Questions:**
- **Confirmed:** the channel launcher and serverless `Flush` both route through the processor's `applyRedactingRules`. Do per-source `MaskSequences` rules apply to channel-sourced messages (depends on `Origin` setup at `buildMessage`)? `(partial: global processor path confirmed; per-source rules Origin-dependent)`

### oversized-line-truncation-safe — Oversized Lines Truncated, Never Silently Dropped

| | |
|---|---|
| **Type** | Safety |
| **Property** | Lines exceeding `max_message_size_bytes` are truncated (flagged `"...TRUNCATED..."`), `LogsTruncated` increments, and the message is always forwarded — never silently dropped. |
| **Invariant** | `AlwaysOrUnreachable("truncated-message-forwarded")` at the `shouldTruncate` branch in `single_line_handler.go`. Workload-side: every oversized line yields a fakeintake message containing `"...TRUNCATED..."`. Missing. |
| **Antithesis Angle** | CPU throttle interleaves multiline aggregation and truncation (regression target `60c521b9e7d`). Edge case: a truncated message exceeding `batch_max_content_size` is dropped at the batch layer (`tlmDroppedTooLarge`) with no `BytesMissed` and possibly an offset advance. |
| **Why It Matters** | Truncated/lost stacktraces corrupt observability; worse, a truncated line that still exceeds `batch_max_content_size` is dropped at the batch layer without advancing the offset → the tailer re-reads it forever on restart (a silent stall/loop, confirmed). |

**Open Questions:**
- **Confirmed (new hazard):** a truncated message exceeding `batch_max_content_size` is dropped at `batch.go:117` (`tlmDroppedTooLarge`) *before* it enters the buffer, so the auditor never sees it — the offset is **not** advanced and `BytesMissed` is **not** incremented. On restart the tailer re-reads the same oversized line → an **infinite re-read loop** with no mitigation. (Property scope extends to restart-loop safety; see Why It Matters.)
- Is `lineLimit` always ≤ `batch_max_content_size`? `(partial: not verified; if false, the truncate-then-batch-drop loop is reachable)`

### multiline-not-split-across-pipelines — Multiline Events Not Split

| | |
|---|---|
| **Type** | Safety |
| **Property** | A multiline event is delivered as a single message — never split across two messages or lost — at a file-rotation boundary or a multiline flush-timer fire. |
| **Invariant** | `Always`: every workload multiline event (BEGIN/END markers) appears at fakeintake as one message with both markers. SUT-side `Sometimes` that the multiline `Flush()` path during rotation / timer is exercised. Missing. |
| **Antithesis Angle** | (a) Rotation: `StopAfterFileRotation()` cancels `forwardContext` (`tailer.go:332`) *before* `decoder.Stop()` — the decoder's flushed partial event may be discarded. (b) Clock: a forward monotonic jump fires `flushTimer` early, emitting a partial event with no truncation flag (continuation then arrives separately). |
| **Why It Matters** | Silent multiline splitting is indistinguishable from "the app only logged 2 lines"; corrupts error analysis. History: `046241bfc73`, `15b1c1c8ae2`, `60c521b9e7d`. |

**Open Questions:**
- **REPRODUCED (2026-05-29):** `pkg/logs/tailers/file/antithesis_multiline_rotation_demo_test.go`. A control test confirms multiline aggregation engages (RegexAggregator combines the event); the rotation test then shows the buffered event is **discarded — 0 messages delivered, 3/3 runs**. Mechanism confirmed: `StopAfterFileRotation` cancels `forwardContext` (`stopForward()`) before the deferred `decoder.Stop()`/`Flush()` emits the aggregated event, and `forwardMessages`' select takes the `forwardContext.Done()` arm, dropping it. (My first attempt was inconclusive because aggregation hadn't engaged; corrected.)
- **Dropped (B-CLOCK):** the clock-timer-flush variant is out of scope; this property now covers the **rotation-boundary** split only (the confirmed deterministic discard above).
- Default multiline `flushTimeout` (affects the rotation-drain timing)? `(needs human input)`
- Auto-multiline `DetectingAggregator` flush semantics at rotation. `(partial)`

---

## Category B — Auditor Offset, Delivery & Replay

The auditor's persisted offset is the single source of truth for what's been
delivered. Offset errors cause silent loss (offset too far ahead) or duplicate
storms (offset regresses). At-least-once is the contract; exactly-once is not.

### auditor-offset-safety — Offset Only Advances Past Delivered or Permanently-Rejected Data

| | |
|---|---|
| **Type** | Safety |
| **Property** | The committed registry offset for a source never exceeds the offset of the last payload that was either delivered (2xx) or permanently rejected (4xx) — never ahead of durably-handled data. |
| **Invariant** | `Always`: `offset_in_registry[id] <= max_handled_offset[id]` (workload reconciles against fakeintake delivery + observed 4xx). SUT-side `Unreachable("registry-offset-ahead-of-durable-send")`; structural `Always` that `output <- payload` in `sendAndRetry()` runs exactly once, after the retry loop. Missing. |
| **Antithesis Angle** | CPU throttle between `output <- payload` (`http/destination.go:318`) and auditor consumption; node termination during the 1-second flush window. |
| **Why It Matters** | An offset ahead of durable data is silent loss, indistinguishable from normal operation on restart. The HTTP "4xx advances offset" rule deliberately drops un-deliverable data — phrasing treats that as an allowed exception. |

**Open Questions:**
- **Confirmed from code:** HTTP advances the offset after *every* send (2xx *and* permanent 4xx); TCP advances only on success. Is the 4xx-advances-offset behavior intentional? `(needs human input)` Is the HTTP-vs-TCP asymmetry intentional? `(needs human input)`
- Does fakeintake expose per-payload byte offsets for correlation, or only line counts? `(needs human input)`
- Dual-shipping `IngestionTimestamp` guard: confirmed for file tailers; journald cursor offsets unclear. `(partial)`

### container-identifier-no-collision — Rotation Does Not Regress the Registry Offset

| | |
|---|---|
| **Type** | Safety |
| **Property** | When a file/container rotates and old+new tailers are briefly both active, the registry offset for their shared identifier never regresses to a lower value. |
| **Invariant** | `Always`: for each identifier, the stored offset is monotonically non-decreasing (`updateRegistry()`, `auditor.go:374`). Today the only guard is an `IngestionTimestamp` "newer wins" check (`auditor.go:386-389`), defeatable by clock skew. Missing. |
| **Antithesis Angle** | CPU throttle / scheduling inversion during the 60s rotation drain window: the old tailer's late `updateRegistry` arrives after the new tailer committed a higher offset → regression → duplicate storm (or loss) on next restart. (Driven by scheduling, **not** the clock fault — survives the B-CLOCK drop; clock skew is merely an *additional* way to defeat the `IngestionTimestamp` guard.) FIXME at `tailer.go:260` acknowledges the shared `"file:"+path` identifier. |
| **Why It Matters** | Container rotation is the dominant Kubernetes path; each collision is a duplicate storm proportional to file size. |

**Open Questions:**
- **Confirmed:** `use_fingerprint=true` does **not** change `file.Identifier()` (always `"file:"+path`), so rotated tailers share a key; `cleanUpRotatedTailers()` has no ordering guarantee with registry writes.
- Which runtime/config actually drives two simultaneously-active tailers on one identifier? `(needs human input)`
- Is `IngestionTimestamp` resolution enough to order their messages? `(partial: guard exists but defeatable by clock skew/scheduling)`
- Will the topology use container-based sources? `(needs human input)`

### offset-no-regression-on-seek-error — Seek Failure Does Not Reset Offset to Zero

| | |
|---|---|
| **Type** | Safety |
| **Property** | If `f.Seek()` fails during tailer init, the tailer aborts or uses a safe fallback — it never silently uses offset 0 and re-reads the whole file. |
| **Invariant** | `Unreachable`: the path where `f.Seek()` errors and the stored offset is < the expected resume offset (`tailer_nix.go:36`, currently `ret, _ := f.Seek(...)` discards the error). Workload-side: after a seek-error injection, duplicate count is bounded, not O(file size). Missing. |
| **Antithesis Angle** | Filesystem I/O fault makes `Seek` fail; current code stores the zero return and re-reads from the start. |
| **Why It Matters** | Silent offset regression → unbounded duplicate delivery. The codebase's own FIXME flags the discarded error. |

**Open Questions:**
- **Investigated:** the Windows tailer has the *same* setup-seek discard bug (`tailer_windows.go:37`), though its in-loop seek does check errors — so Windows is not a clean fix model; `ret=0` is confirmed stored on failure.
- Can `Seek` errors be injected in the planned topology? `(partial: needs a real/faultable FS; afero MemMapFs may not inject seek errors)`

### at-least-once-no-loss — Every Written Line Delivered At Least Once

| | |
|---|---|
| **Type** | Liveness |
| **Property** | After a fault-quiet recovery window, every line written before the tailer read it is delivered at least once, except lines explicitly dropped by the 4xx-permanent path or the rotation-`closeTimeout` path. |
| **Invariant** | `Sometimes("all-sequence-numbers-received-after-quiet-period")` at fakeintake (duplicates OK; absent ones are not, modulo documented exceptions). Needs a recovery window (`eventually_` or `ANTITHESIS_STOP_FAULTS`). Missing. |
| **Antithesis Angle** | Node termination (crash recovery), network partition + rotation (rotation-loss path), filesystem fault (registry corruption). |
| **Why It Matters** | At-least-once is the fundamental logs contract; the rotation-under-backpressure exception is the #1 production complaint with no existing test. |

**Open Questions:**
- **Confirmed:** `BytesMissed` is an in-memory `expvar.Int` (resets on restart — read it before the next restart); `close_timeout` defaults to 60s.
- What `close_timeout` will the topology set (shorter → faster reachability)? `(needs human input)`
- Is the non-atomic registry path reachable in a plain Linux container topology? `(needs human input)`
- Pin `logs_config.registry_ttl` (~23h default) above the max simulated fault window so a blocked source's entry isn't TTL-evicted (which would restart its tailer from EOF and look correct while violating at-least-once). `(needs human input)`

### no-loss-and-duplicate-same-line — No Session Both Loses and Duplicates

| | |
|---|---|
| **Type** | Safety |
| **Property** | A single fault+crash+restart sequence never produces one byte range lost (absent from intake) while another range from the same session is duplicated outside an expected replay; in particular a 4xx-rejected line must not reappear after restart. |
| **Invariant** | Workload-side `Always` over a numbered sequence: no line both-absent-and-another-duplicated in a non-replay context; any 4xx-rejected line must not be delivered after restart. SUT-side `AlwaysOrUnreachable`: the auditor must not persist an advanced offset for a 4xx payload before the registry is flushed. Missing. |
| **Antithesis Angle** | Compound fault: network 4xx + CPU throttle (delays auditor flush) + SIGKILL — opens the window where the in-memory offset advanced for a 4xx-dropped payload, the registry wasn't flushed, and replay re-delivers the "permanently dropped" line. |
| **Why It Matters** | Violates multiple orthogonal guarantees at once; the loss-adjacent-to-duplicate anomaly is the hardest log issue to diagnose. |

**Open Questions:**
- **Confirmed:** there is no flush-before-drop path — the ≤1s crash window applies to every `output <- payload`, including 4xx drops, so a 4xx-rejected line can be replayed after a crash in that window.
- Does fakeintake record the HTTP response code per payload (needed to detect a 4xx→replay)? `(needs human input)`
- Is "4xx advances offset" intended (feature) or oversight (bug)? `(needs human input)` (shared with `auditor-offset-safety`)

---

## Category C — Crash Recovery & Registry Durability

The on-disk registry must survive ungraceful death and reload correctly, including
across format-version upgrades. Requires the node-termination fault (A7).

### registry-survives-crash — Registry File Never Corrupt After Ungraceful Shutdown

| | |
|---|---|
| **Type** | Safety |
| **Property** | After `kill -9`, `registry.json` is the state from the last (or prior) successful flush — never zero-length or invalid JSON — under both the atomic and non-atomic (Fargate) writers. |
| **Invariant** | `Always`: post-restart, the recovered registry is valid JSON with ≥ the previous flush's entries. `Unreachable("registry-file-zero-length-after-restart")`; `Reachable("non-atomic-writer-path-taken")`. Missing. |
| **Antithesis Angle** | Node termination timed between `os.Create` (truncate) and `f.Write` in the non-atomic writer (`registry_writer.go:62-69`), and across the atomic temp+rename; EXDEV makes the atomic rename silently stale. `DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false` forces the risky path. |
| **Why It Matters** | Registry corruption → all sources re-tail from default position (usually EOF) → mass loss. On Fargate, every task replacement carries this risk. |

**Open Questions:**
- **Confirmed:** `atomic_registry_write` defaults to `true` (computed as `!IsECSFargate()`); `DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false` forces the non-atomic path on any platform (so the Fargate corruption path is testable without Fargate).
- Does the Antithesis container volume give durable rename semantics, or an overlay where atomic rename isn't guaranteed? `(needs human input)`
- Will the topology simulate Fargate (force non-atomic)? `(needs human input)`

### registry-recovers-after-crash — Registry Reloads with Correct Offsets

| | |
|---|---|
| **Type** | Liveness / Reachability |
| **Property** | After ungraceful termination, the auditor loads a non-empty registry and tailers resume from committed offsets; the missing/corrupt fallback is reached only when the file is genuinely absent/corrupt. |
| **Invariant** | `Sometimes("registry-recovered-with-non-empty-offsets")` in `recoverRegistry()`; `Reachable("recovery-from-missing-corrupt-registry")` at the error returns; workload `Always(post_restart_count >= pre_restart - replay_window)`. Missing. |
| **Antithesis Angle** | Node termination timed to maximize the ≤1s un-flushed window; explores both data-loss (EOF default) and mass-replay (BOF) outcomes of an empty recovered map. |
| **Why It Matters** | Silent empty-map recovery is the root cause of mass replay or loss with no operator alert. |

**Open Questions:**
- None. (Investigated: `recoverRegistry()` is the only startup read path, and start-ordering guarantees it completes before any tailer calls `GetOffset()`.)

### registry-format-migration-safe — Version Migration Preserves Offsets

| | |
|---|---|
| **Type** | Safety / Reachability |
| **Property** | Migration from v0/v1 registry format to v2 preserves all valid offset entries; the unknown-version → empty-map `default` branch is never hit for versions 0–2. |
| **Invariant** | `Reachable("v1-migration-path-taken")`, `Reachable("v0-migration-path-taken")`; `Unreachable("unknown-version-empty-registry")` at the `default` branch; workload `Always(migrated_count >= source_valid_count)`. Missing. |
| **Antithesis Angle** | Pre-seed a v1 registry, start a v2 agent; node termination during the first post-migration flush (esp. Fargate non-atomic) may corrupt the new v2 file. |
| **Why It Matters** | Silent entry loss during upgrade migration causes replay or loss invisible until backend gaps appear. |

**Open Questions:**
- **Investigated:** migration is in-memory only, idempotent on a pre-flush crash, and the v0/v1/v2 paths have no logic bugs.
- Can the topology pre-seed a v1 registry / run two agent versions? `(needs human input)`
- Is rollback (v2→v1) in scope? `(needs human input)`

---

## Category D — Adaptive Sampling Correctness

Requires `AdaptiveSampler` enabled (A2). The credit/token model and its interval
clock are correctness- and timing-sensitive — Antithesis's strength.

### sampling-exact-count — At Most N Low-Value Logs Delivered Per Interval

| | |
|---|---|
| **Type** | Safety |
| **Property** | For any matched low-value pattern, lines delivered in any interval T do not exceed `ceil(RateLimit·T + BurstSize)`. |
| **Invariant** | `Always`: fakeintake count of pattern-matching lines in any window T ≤ the bound. SUT-side `Always` at the `allow` branch (`sampler.go:235`) confirming credits ≥ 1.0 before decrement. Missing. |
| **Antithesis Angle** | Clock jump forward grants a credit windfall (`elapsed·RateLimit`) → burst exceeds limit; pattern-table eviction under CPU faults re-allocates burst credits to hot patterns. |
| **Why It Matters** | The per-pattern rate limit is the sampler's core correctness invariant (owner's design doc); violations increase customer volume and cost. |

**Open Questions:**
- **Confirmed:** `Process()` is single-goroutine per source (no `entries` data race).
- Is there a warmup before sampling engages (assert on T0 or later)? `(needs human input)`
- Once low-value, always low-value, or re-evaluated per interval? `(needs human input)`
- Topology `RateLimit`/`BurstSize`/`MaxPatterns` + is `AdaptiveSampler` enabled? `(needs human input)`

### high-value-never-sampled — High-Value Logs Never Dropped by the Sampler

| | |
|---|---|
| **Type** | Safety |
| **Property** | With `ProtectImportantLogs=true`, every line whose tokenized content contains a severity keyword (FATAL/ERROR/PANIC…) is delivered regardless of credit balance. |
| **Invariant** | `Always`: every workload high-value line reaches fakeintake at least once. SUT-side `Always` at the `isImportant` early-return (`sampler.go:194-198`). Missing. |
| **Antithesis Angle** | Tokenizer misclassification under CPU fault makes `isImportant()` return false. Clock faults do NOT break this (protection precedes the credit check). |
| **Why It Matters** | Dropping FATAL/ERROR defeats incident detection — the sampler's primary safety contract. |

**Open Questions:**
- **Confirmed:** the tokenizer is case-**insensitive** (`fatal`/`FATAL`/`Fatal` all map to the `Fatal` token); the zero-credit important-bypass is unit-tested.
- **Resolved (eval):** `Exclude` ⇒ `shouldSample=false` (never rate-limit), so it does not conflict with `isImportant` — an excluded important log is double-protected, not dropped.

### sampling-reachable-under-load — Sampler Drop/Eviction Paths Are Exercised

| | |
|---|---|
| **Type** | Reachability |
| **Property** | During a run with a high-rate low-value pattern, the sampler's drop path and pattern-eviction path are each reached at least once. |
| **Invariant** | `Reachable` at `tlmAdaptiveSamplerDropped.Inc()` (`sampler.go:238`) and at the eviction path (`sampler.go:247`). Missing. |
| **Antithesis Angle** | If workload rate is too low or `BurstSize` too large, the drop path never executes — this sentinel surfaces that misconfiguration so the other sampling properties aren't vacuous. |
| **Why It Matters** | Without confirming the drop path runs, all sampling properties pass vacuously. |

**Open Questions:**
- **Confirmed:** `NoopSampler` is the default; `AdaptiveSampler` must be explicitly constructed at decoder creation — no config key auto-enables it, so the whole sampling cluster is vacuous unless the topology enables it.
- What `MaxPatterns`/`RateLimit`/`BurstSize` will the topology use (run duration)? `(needs human input)`

### clock-jump-no-extra-sampling — Backward Clock Jump Doesn't Drive Credits Negative

> **DROPPED — user decision B-CLOCK (2026-05-29).** Clock-dependent properties are
> out of scope: they hinge on whether the Antithesis clock fault moves Go's
> *monotonic* clock (the sampler uses `time.Now().Sub`, monotonic), which is
> unconfirmed and likely wall-clock-only. Evidence file retained for reference; not
> counted in the active catalog. (The non-clock sampling properties are unaffected.)

| | |
|---|---|
| **Type** | Safety |
| **Property** | A backward clock fault never causes a credit deficit large enough to drop more than the configured rate limit. |
| **Invariant** | `Always`: after each refill in `Process()`, `e.credits >= -1.0` (currently unbounded below: backward jump makes `elapsed` negative → `credits -= |elapsed|·RateLimit`). SUT-side `Always` after the refill at `sampler.go:208`. Missing. Requires clock fault (A7). |
| **Antithesis Angle** | Backward jump while a high-rate pattern is active → large deficit → seconds-long blackout of that pattern. |
| **Why It Matters** | Silent, transient, total drop of a pattern from a monitoring agent — a contract violation with no error surfaced. |

**Open Questions:**
- **Confirmed:** the `now` injection point is test-only (production uses `time.Now()`); `protect_important_logs` is clock-invariant (runs before the credit refill).
- Does the Antithesis clock fault move Go's monotonic clock or only wall-clock? `time.Now().Sub` uses the monotonic component — if monotonic is immune this path may be unreachable. `(needs human input)`

### adaptive-sampler-no-aliasing — Pattern-Table Resort Doesn't Corrupt Credit Accounting

| | |
|---|---|
| **Type** | Safety |
| **Property** | The `sampled_count` tag on an emitted line reflects the drops for *that* pattern, not another pattern that swapped slots during the in-place bubble sort. |
| **Invariant** | `Always("sampler-sampled-count-tag-matches-dropped-count")`: workload compares per-pattern drops to the `adaptive_sampler_sampled_count` tag on the next emission. SUT-side `Always` that the matched entry's `matchCount` incremented (not a neighbor's). Missing. |
| **Antithesis Angle** | Single-goroutine, but diverse input sequences drive states with ≥3 bubble swaps per pass — the aliasing class of `7687b846b2a`. |
| **Why It Matters** | Corrupt `sampled_count` corrupts the user's view of how much was suppressed; confirmed past bug. **Note (eval):** this is a *sequential-logic* bug — Antithesis thread/timing faults are a weak trigger; a Go fuzz test over `Process()` input sequences is the better tool. Lower Antithesis priority; keep as a workload canary. |

**Open Questions:**
- **Confirmed:** `Process()` is single-goroutine (no concurrency hazard); `sampler_test.go` covers `sampled` aliasing specifically.
- Are `credits`/`matchCount` independently verified non-aliased after a bubble swap? `(partial: only `sampled` covered)`
- Topology `BurstSize`? `(partial: caller-defined, no repo default)`

---

## Category E — Concurrency & Shutdown Safety

The agent's most common bug classes (per its own review guidance and bug history):
deadlocks, send-on-closed-channel, double-close panics, dropped-on-stop, goroutine
leaks. All are timing-driven — Antithesis's CPU-pause/thread-scheduling sweet spot.

### no-services-store-deadlock — Services Store Never Deadlocks (and Doesn't Starve Tailing)

| | |
|---|---|
| **Type** | Safety (+ liveness facet) |
| **Property** | `Services.AddService`/`RemoveService` never hold `s.mu` while blocking indefinitely on a subscriber channel send; consequently container logs aren't permanently skipped because the store blocked. |
| **Invariant** | `Unreachable("services-addservice-blocked-indefinitely")` (watchdog: timeout on the send loop at `service/services.go:42-44,62-64`). Workload `Always("addservice-completes-in-time")`; liveness facet: lines written during churn are eventually delivered or counted. Missing. |
| **Antithesis Angle** | CPU-pause a subscriber (container/journald launcher) while `AddService` is mid-send; the mutex hold stalls all callers. Contrast `LogSources` (releases lock before send) as the correct model. |
| **Why It Matters** | A deadlock here silently stops container log pickup with no failing health check or metric — total log loss for containers started afterward. **Note:** dormant today (no production subscribers); value is as a regression guard against reintroduction and as a model-vs-`LogSources` contrast. |

**Open Questions:**
- **Confirmed (scope-narrowing):** there are currently **zero production subscribers** to `Services` (all four subscriber methods are called only from tests), so the deadlock loop iterates over empty slices today — a **latent/dormant API hazard** and regression guard, not a live bug. `LogSources` (releases the mutex before sending) is the correct model.
- Is there a deployment with container churn fast enough to exercise this if subscribers return? `(needs human input)`
- Can Antithesis fault the Docker Unix socket to extend the window? `(needs human input)`

### no-send-on-closed-on-shutdown — No Send-on-Closed-Channel Panic During Shutdown

| | |
|---|---|
| **Type** | Safety |
| **Property** | No goroutine sends to a closed channel during provider/sender/auditor shutdown. |
| **Invariant** | `Unreachable("send-on-closed-inputchan-from-forwarder")` (recover wrapper around `provider.go:361` `InputChan <- msg`) and `Unreachable("sender-queue-double-close")`. Missing. |
| **Antithesis Angle** | CPU-pause the `forwardWithFailover` goroutine blocked on `InputChan`; `Stop()` closes `routerChannels` (doesn't wake it) then closes `InputChan` → panic. |
| **Why It Matters** | Shutdown panics lose in-flight data, skip the auditor flush (→ duplicates), and corrupt process state. Top-named bug class. |

**Open Questions:**
- None. (Investigated: `provider.Stop()` has no internal timeout but is externally bounded by the ~35s `stopComponents` grace period; no production path double-stops the sender. The Path-1 panic remains reachable within that window.)

### idempotent-stop — Stop() Is Safe to Call More Than Once

| | |
|---|---|
| **Type** | Safety |
| **Property** | A second `Stop()` on `Sender`, `provider`, or `DestinationSender` does not panic. |
| **Invariant** | `Unreachable("sender-stop-double-close-panic")` / `Unreachable("destination-sender-stop-double-close-panic")` (recover around the `close()` calls). Missing. No `sync.Once` guards today. |
| **Antithesis Angle** | Two goroutines call `provider.Stop()` at the same scheduler point (signal handler + API + error recovery). The file launcher's `sync.Once` shows the intended pattern. |
| **Why It Matters** | Double-close panic during shutdown loses in-flight data and skips the auditor flush. |

**Open Questions:**
- **Investigated:** no normal single-caller double-stops; `ParallelStopper` offers no re-invocation guard, so concurrent `agent.Stop()` (OS signal + API) could double-stop. `(partial: no normal double-call; concurrent-shutdown path plausible)`
- Is there a project-wide `sync.Once`-on-Stop policy? `(needs human input)`

### clean-shutdown-completes — Shutdown Always Completes in Bounded Time

| | |
|---|---|
| **Type** | Liveness |
| **Property** | After a shutdown signal, all logs components complete `Stop()` within a bounded time without an external kill. |
| **Invariant** | `Reachable("tailer-stop-completed")`; workload `Sometimes("shutdown-completed-within-30s")` after SIGTERM with no SIGKILL. Missing. |
| **Antithesis Angle** | Network partition fills `outputChan` (100); shutdown arrives; `forwardMessages` is blocked on `outputChan <- msg` while the pipeline stops draining — a circular wait Antithesis finds by pausing the network fault then triggering shutdown. |
| **Why It Matters** | A hung shutdown blocks upgrades and skips the auditor flush, forcing a kill and increasing duplicates. History: `94d7ccbfc35`, `7041f901670`. |

**Open Questions:**
- **Confirmed:** the agent imposes a ~35s shutdown bound (30s `stop_grace_period` + 5s hard cutoff); `Stop()` does NOT call `stopForward()`, so a saturated-`outputChan` tailer can hang until that fallback. Success should mean completing *without* relying on the 35s timeout.
- Is the tailer-`Stop()`→`forwardMessages` hang reachable under the current parallel-stopper ordering? `(partial: pipeline alive while tailers stop; hang requires outputChan saturation)`

### no-goroutine-leak-after-stop — All Goroutines Terminate on Stop

| | |
|---|---|
| **Type** | Safety |
| **Property** | After `Stop()` returns, all goroutines started by the logs pipeline have exited (count returns to pre-start baseline). |
| **Invariant** | SUT-side `Always("goroutine-count-returned-to-baseline")` via `runtime.NumGoroutine()` in an SDK assertion after `Stop()` (the expvar/pprof server binds loopback, unreachable cross-container, so prefer a SUT-side check); plus `Reachable` at the clean-exit of the noop-sink drain (`worker.go:214-219`) and retryReader (`destination_sender.go:57-68`). Missing. (Per eval R-GOROUTINE.) |
| **Antithesis Angle** | CPU-pause a `DestinationSender` consumer during shutdown leaving the goroutine blocked; container churn drives many fire-and-forget `WrappedSource.Start()` goroutines. |
| **Why It Matters** | Leaks → memory/FD growth in a long-running agent; prevent clean restart/upgrade. History: `86882e6e718`. |

**Open Questions:**
- **Investigated:** HTTP destination goroutines honor `destinationsContext` cancellation (lower leak risk than first thought); `WrappedSource` Add/Remove goroutines have **no join** (the residual leak risk). Shutdown is bounded ~35s (leaks past that are masked by process exit).
- Post-stop baseline goroutine count? `(needs human input)`
- Can the workload query the SUT's goroutine count externally, or is SUT-side pprof/instrumentation required? `(needs human input)`
- Is `pipeline_failover.enabled` in the topology (H4 leak is failover-specific)? `(needs human input)`

### auditor-drains-on-stop — Auditor Drains In-Flight Payloads Before Shutdown

| | |
|---|---|
| **Type** | Safety |
| **Property** | On `Stop()`, payloads buffered in the auditor `inputChan` (and payloads sent-but-not-yet-audited) are processed and their offsets persisted before shutdown — at-most duplicate delivery on restart, not loss. |
| **Invariant** | `Reachable("auditor-drained-all-buffered-payloads-at-stop")` (run loop exits with `len(inputChan)==0`); `Sometimes("auditor-run-loop-exited-with-buffered-payloads-remaining")` as a bug trap; `Unreachable("send-on-closed-channel in auditor")`. Missing. |
| **Antithesis Angle** | Go `select` non-determinism: with `inputChan` closed and buffered items, the `flushTicker`/`cleanUpTicker` arm may fire instead of the input arm → early exit with items remaining (H5). Clock fast-tick + CPU-pause widen it. |
| **Why It Matters** | Undrained payloads → stale registry offset → duplicate delivery on restart (the #2 user complaint). Root of `62bf5e55c25`, `a5141ba432c`. `Stop()` does not call `Flush()` first. |

**Open Questions:**
- **REFUTED — not a bug (2026-05-29):** `comp/logs/auditor/impl/antithesis_drains_on_stop_demo_test.go`. The earlier "drain gap" reasoning was wrong about Go semantics: `case v, ok := <-ch` on a *closed buffered* channel delivers all N buffered items (`ok=true`) before the zero-value (`ok=false`); the `select` can interleave ticker arms but cannot skip buffered items. `Stop()` → `close(inputChan)` → run loop drains all buffered payloads → `<-done` → `flushRegistry()`. 100/100 payloads persisted, verified (incl. `-race`). The genuine residual race (H2) is in `Flush()` (transport-restart `len()` snapshot), NOT `Stop()` — tracked under `transport-switch-no-loss`.

### container-addremovesource-ordering — No Add-After-Remove Source Holes During Churn

| | |
|---|---|
| **Type** | Safety |
| **Property** | When a container starts then immediately stops, `LogSources` does not retain that container's source after all stop operations complete. |
| **Invariant** | Workload `Always("no-orphaned-sources-after-container-stop")`; SUT-side `Unreachable("source-added-after-all-subscribers-stopped")`. Missing. |
| **Antithesis Angle** | `WrappedSource.Start()` and `.Stop()` each spawn fire-and-forget goroutines (`source.go:34,42`); Antithesis pauses the Add goroutine, lets Remove run (no-op), then resumes Add → permanent orphan. |
| **Why It Matters** | An orphaned source tails a dead container's file (resource leak, stale data); a missed source is silent log loss during rapid churn. |

**Open Questions:**
- **Confirmed:** no join mechanism for the fire-and-forget Add/Remove goroutines; a post-stop `AddSource` silently drops via the `<-stream.done` arm (data loss / stale source, not a panic).
- Is the long-term fix (launchers shouldn't add sources) prioritized? `(needs human input)`

---

## Category F — Backpressure, Liveness & Resource Boundaries

The headline production failure (rotation-under-backpressure loss) and its inverse
guarantee (no loss without rotation), plus recovery progress and bounded memory.
Network-to-intake faults are the trigger.

### backpressure-no-rotation-loss — Rotation Under Backpressure: Loss Is Bounded & Observable

| | |
|---|---|
| **Type** | Liveness / Reachability |
| **Property** | When a file rotates while the pipeline is backpressured, lines written to the old file before rotation are eventually delivered; if loss occurs, it occurs *only* via `closeTimeout` expiry and is reflected in `BytesMissed`. |
| **Invariant** | `Reachable("bytes-missed-on-rotation")` (`tailer.go:325`) + `Reachable("closeTimeout-goroutine-fired")` (so the dangerous branch is confirmed entered) + `Sometimes(BytesMissed > 0)` to confirm real loss; ideally the workload reconciles which sequence numbers were lost. Compound-fault recovery window required. Missing. |
| **Antithesis Angle** | Network partition (> `close_timeout`) + file rotation during the partition: `InputChan` fills → old tailer `forwardMessages` blocks → 60s `closeTimeout` fires → `stopForward()` cancels context → buffered decoded messages silently discarded. |
| **Why It Matters** | The #1 user-visible failure (sut-analysis §12.1), with zero existing end-to-end coverage. `BytesMissed` counts unread file bytes (`fileSize-lastOffset`), possibly *understating* forwarding loss. |

**Open Questions:**
- **Confirmed:** `BytesMissed = fileSize - lastReadOffset` (an upper-bound counter incremented once per rotation, not per dropped message), an in-memory `expvar` that resets on restart. After `stopForward()`, `forwardMessages` discards each remaining decoded message with **no per-message metric**.
- What `close_timeout` will the topology use (shorter → faster reachability)? `(needs human input)`
- Is there a knob to pause the rotation countdown while backpressured? `(partial: none found — likely a design gap)`

### backpressure-before-drop — No Loss Without Rotation Under Sustained Backpressure

| | |
|---|---|
| **Type** | Safety |
| **Property** | Under sustained partition *without* file rotation, no data is lost: the tailer blocks on a full `outputChan` and `BytesMissed` stays zero; all lines arrive after recovery. |
| **Invariant** | `Always(BytesMissed == 0)` during a no-rotation fault window; `Sometimes(tailer fully backpressured)` to confirm the workload reaches saturation; `Sometimes("tailer-lastReadOffset-advanced-after-backpressure-clear")` (tailer-progress sentinel for L6); workload: lines-before == lines-after-recovery. Missing. |
| **Antithesis Angle** | Partition without rotation exercises steady-state backpressure; the worker 100ms busy-sleep (`worker.go:146`) + CPU throttle shape the window. |
| **Why It Matters** | Verifies the RFC claim that loss happens *only* at rotation/deletion — the inverse of the headline property. |

**Open Questions:**
- **Resolved:** `logs_component_utilization.ratio ≈ 1.0` is the workload-observable proxy for a fully-backpressured component (no SUT instrumentation strictly required, though a SUT `Sometimes` is sharper). After `stopForward()`, dropped messages get no per-message metric — loss is visible only via `BytesMissed`.

### queued-payloads-eventually-sent — Queued Payloads Drain After Destination Recovery

| | |
|---|---|
| **Type** | Liveness |
| **Property** | After a network partition clears (no concurrent rotation), all queued payloads are delivered within ~`2×max_backoff` (≈240s), and the pipeline resumes forward progress (`LogsSent` advances). |
| **Invariant** | `Sometimes("destination-transitions-retrying-to-not-retrying")` + `Reachable("http-retry-backoff-sleep-entered")`; workload `Always(fakeintake_count >= injected - permanent_drops)` after the quiet window. Recovery window (≈240s) required. Missing. |
| **Antithesis Angle** | Partition + recovery; Antithesis varies partition duration vs. backoff steps (base 1s, max 120s). The `cancelSendChan` recovery signal and the 100ms busy-sleep affect resume latency. The `Services.AddService` deadlock (H1) would permanently block new-source progress. |
| **Why It Matters** | Retry is the first defense against network blips; "logs silently stop flowing" (§12.4) is the hardest production failure to diagnose. |

**Open Questions:**
- **Confirmed:** the 100ms busy-sleep adds ≤100ms on recovery (benign); HTTP backoff defaults base=1s, max=120s, `RecoveryInterval=2`, `RecoveryReset=false` (≈4 successes to fully recover) — the ~240s quiet window is adequate.
- Does the real Datadog intake ever return 429 (worth modeling)? `(needs human input)`
- Are replayed-from-registry payloads re-deliverable after recovery? `(partial)`

### bounded-memory-under-backpressure — RSS Stays Bounded; No zstd Context Leak

| | |
|---|---|
| **Type** | Safety |
| **Property** | Agent RSS does not grow unboundedly under prolonged outage; every `StreamCompressor` (`ZSTD_CCtx`) is freed exactly once (via `sendMessages` success or `resetBatch` error), none orphaned or double-freed. |
| **Invariant** | Workload `Always(rss < k × baseline)` sampled periodically; SUT-side `Reachable("zstd-cctx-close-on-reset")` (`batch.go:76`); structural `Always(b.compressor == nil after sendMessages)`. Missing. |
| **Antithesis Angle** | Soak with repeated encode errors / sustained backpressure → repeated `resetBatch()` cycles; long-run mode detects slow C-heap RSS growth invisible to Go GC. Regression target `0d9dfc76f46`. |
| **Why It Matters** | Multi-GiB C-heap leak caused agent OOM; the fix needs exercise under load. |

**Open Questions:**
- Does the topology build with CGO zstd or nocgo? The leak was CGO-specific; is the nocgo `klauspost` `Close()` idempotent?

---

## Category G — Protocol Contracts

Intake HTTP error handling: permanent vs. retryable classification, rate-limiting,
and silent batch-encode loss.

### permanent-error-no-retry — Permanent (4xx) Errors Dropped Once, Pipeline Continues

| | |
|---|---|
| **Type** | Safety |
| **Property** | On 400/401/403/413 the agent does not retry, increments `DestinationLogsDropped`, sends the payload to the auditor once, and continues delivering subsequent lines without stalling. |
| **Invariant** | `AlwaysOrUnreachable("permanent-error-classify")`: the error is NOT a `*client.RetryableError`; `Unreachable` for any loop-continuation on `errClient` in `sendAndRetry()`. Workload: fakeintake sees the payload exactly once per 4xx. Missing. (403+secrets refresh is a legitimate single retry that must not become infinite.) |
| **Antithesis Angle** | Chaos proxy returns 4xx; a misclassification (wrapping 4xx as retryable) creates an infinite loop that jams the pipeline — found by code-path exploration. |
| **Why It Matters** | Infinite retry on a permanent error stalls the whole pipeline (tailer eventually blocks). |

**Open Questions:**
- **Confirmed:** the permanent-drop `output <- payload` is a blocking send but runs in a `workerPool` goroutine, so it doesn't block new-payload intake — it provides bounded secondary backpressure (and blocks graceful shutdown), not deadlock.
- Is 413 "advance offset, drop the whole batch" right when individual messages were valid? `(needs human input)`

### retryable-no-retry-after — 429 Retried via Backoff, Never Regressed to Permanent Drop

| | |
|---|---|
| **Type** | Safety |
| **Property** | HTTP 429 is treated as retryable (agent's own exponential backoff, no `Retry-After` parsing) and is never reclassified as a permanent drop. |
| **Invariant** | `AlwaysOrUnreachable("429-treated-as-retryable")` in `updateRetryState()`; workload: fakeintake returns 429×N then 200, agent eventually delivers; `Always(DestinationLogsDropped unchanged during 429 phase)`. Missing. |
| **Antithesis Angle** | Fakeintake always-429 then 200; clock faults expose backoff-timing issues a `Retry-After` would otherwise govern. |
| **Why It Matters** | A regression moving 429 → permanent-drop would silently discard all logs during rate-limiting. |

**Open Questions:**
- **Confirmed:** 429 is classified retryable (`429 > 400` → `RetryableError`); there is **no** `Retry-After` parsing anywhere.
- Is `Retry-After` support planned (would change correct behavior)? `(needs human input)`

### batch-encode-failure-no-silent-batch-loss — Batch Encode Failures Don't Silently Lose Data

| | |
|---|---|
| **Type** | Safety / Observability |
| **Property** | When batch encoding fails, the batch is reset, the failure is recorded, and the affected messages are not permanently lost — their offsets are not advanced, so they are re-read on restart. |
| **Invariant** | `AlwaysOrUnreachable("batch-encode-error-classify")` at each `log.Warn("Encoding failed - dropping payload")` site (`batch.go`, 3 sites): the encode-error path is **unreachable under normal Antithesis faults** (it needs OOM-level memory pressure), so `AlwaysOrUnreachable` fits — a `Reachable`/`Sometimes` would mislead by appearing unmet. Structural `Always`: after an encode error, `outputChan <- payload` is NOT called for that batch (so offsets aren't advanced → replay on restart). Missing. |
| **Antithesis Angle** | Memory pressure can fail compressor allocation / `Serialize()`; standard network/process-pause faults do not reach this path. The value is the structural guarantee that a failed batch doesn't advance offsets, not path coverage. |
| **Why It Matters** | Encode failures are metric-invisible (warn-log only); under OOM pressure they could lose data silently. |

**Open Questions:**
- **Confirmed:** `Serialize()`/`compressor.Close()` errors are unreachable under standard faults (network/process-pause); they need OOM-level memory pressure — hence the `AlwaysOrUnreachable` choice above.
- Should the encode-error path get a dedicated drop metric (today warn-log only)? `(needs human input)`

---

## Category H — Lifecycle Transitions

Startup degradation, live transport switches, and the container_collect_all startup
race — operations spanning multiple subsystems and concurrent traffic.

### graceful-degradation-on-startup — Agent Starts Degraded When Dependencies Unavailable

| | |
|---|---|
| **Type** | Safety / Reachability |
| **Property** | When a startup dependency (journald, Docker socket, HTTP intake) is unavailable, the agent starts with reduced capability rather than panicking; `DestinationsContext.Start()` completes before any `Send()`, preventing a nil-context dereference. |
| **Invariant** | `Unreachable("nil-destinations-context-dereference")` at the nil branch of `DestinationsContext.Context()` (else panic at `http/destination.go:328`); `Reachable("TCP-fallback-taken-at-startup")`; `Sometimes("journald-source-skipped")`. Missing. |
| **Antithesis Angle** | Network partition at startup (TCP fallback path); CPU throttle during `startstop.Starter.Start()` (ordering race); removing the journald socket (graceful skip). |
| **Why It Matters** | An agent that panics on startup collects nothing; the nil-context panic is "safe" only by the sequential `Starter` ordering assumption (unproven). |

**Open Questions:**
- **Confirmed:** `startstop.NewStarter().Start()` is a plain sequential for-loop (no goroutines), so `DestinationsContext.Start()` completes before `pipelineProvider.Start()` — the nil-context `Unreachable` is justified by ordering, not just inspection.
- `smartHTTPRestart` starts a background goroutine at startup that can leak if the agent stops before the HTTP upgrade. `(partial)`

### transport-switch-no-loss — Live TCP↔HTTP Switch Doesn't Silently Lose Payloads

| | |
|---|---|
| **Type** | Safety / Liveness |
| **Property** | A transport switch (`smartHTTPRestart`/`restart()`) does not silently lose lines acknowledged before the switch; in-flight payloads may replay (at-least-once) but not vanish. |
| **Invariant** | `Reachable("transport-switch-TCP-to-HTTP")`, `Reachable("transport-rollback-initiated")`; workload `Always(fakeintake cumulative count non-decreasing through the switch)`; `Sometimes("auditor-flush-race-window")`. Missing. |
| **Antithesis Angle** | Partition → TCP fallback; recovery + `smartHTTPRestart` → HTTP upgrade; repeated cycles; CPU throttle during `partialStop` widens the H2 flush race. |
| **Why It Matters** | Every switch creates a data-loss window proportional to channel occupancy; rollback failure leaves the agent non-functional. |

**Open Questions:**
- **Confirmed:** `partialStop`'s parallel pipeline stop drops in-flight payloads (the data-loss window is real); a forwarder stuck on `InputChan` stays stuck even after context cancellation (which only cancels HTTP sends, not the plain channel send).
- After a critical rollback failure, does the agent surface a health-check failure? `(needs human input)`

### container-collect-all-startup-race — Containers Not Mis-Tagged or Gapped at Startup

| | |
|---|---|
| **Type** | Safety |
| **Property** | With `container_collect_all` enabled, containers started during the AD initial-scan window are not simultaneously attributed to both the generic and the annotated source, and have no un-tailed gap between unschedule and reschedule. |
| **Invariant** | `Sometimes("container-transitioned-generic-to-annotated")`; workload `Always("no generic-source lines after annotated source active")`; `Reachable("log-gap-window")` on Remove with no pending Add. Missing. Requires `container_collect_all=true`. |
| **Antithesis Angle** | CPU throttle on the AD goroutine delays annotation delivery, widening the wrong-metadata window; the AD ordering mitigation (`container.go:241-248`) covers only the first scan, not later container restarts. The `Services.AddService` mutex-held-during-send (H1) extends the generic-tailer window. |
| **Why It Matters** | Startup lines get wrong `service`/`env`/`version` tags — breaks trace-log correlation and tag-based RBAC, invisibly. |

**Open Questions:**
- **Confirmed:** `container_collect_all` defaults to **false** (must be enabled in the topology); the AD ordering mitigation (`providers/container.go:241-248`, `listeners/service.go:246-253`) holds only within one `Collect()` call and degrades under CPU throttle.
- Do generic and annotated tailers for the same file share an `Identifier()` (offset-collision risk)? `(needs human input)`
- Does the file-based / Kubernetes-pod-log path exhibit the same race? `(needs human input)`

---

## Category I — Clock-Sensitive Behavior  *(DROPPED — user decision B-CLOCK)*

This category is **dropped** per the user's B-CLOCK decision: clock-fault-dependent
properties are out of scope until the Antithesis clock fault is confirmed to move
Go's monotonic clock (Go timing uses the monotonic component, so a wall-clock-only
fault leaves these vacuous). The entry below is retained, marked dropped.

### clock-jump-no-backoff-underflow — Clock Faults Don't Bypass/Freeze Backoff or the Worker Pool

> **DROPPED — user decision B-CLOCK (2026-05-29).** See category note above. Evidence
> file retained; not counted in the active catalog.

| | |
|---|---|
| **Type** | Safety + Liveness |
| **Property** | A clock fault (forward or backward) does not let the HTTP destination skip its backoff sleep, nor freeze the EWMA worker-pool latency signal. |
| **Invariant** | Safety `AlwaysOrUnreachable` at `destination.go:270`: `blockedUntil.After(time.Now())` holds whenever `backoffDuration > 0` (a forward jump between the two `time.Now()` calls collapses the guard). Liveness `Sometimes(l.inUseWorkers != l.minWorkers)`: the pool scales beyond min at least once, proving EWMA isn't frozen. Missing. Requires clock fault (A7). |
| **Antithesis Angle** | Forward jump between `blockedUntil = time.Now().Add(...)` and the `.After(time.Now())` check skips backoff → retry storm; `time.Since(virtualLatencyLastSample)` overshoot with zero in-window samples → erratic worker scaling. |
| **Why It Matters** | Backoff bypass → retry storm against a faulted intake; worker miscalibration silently degrades throughput. |

**Open Questions:**
- **Confirmed:** min≠max workers is the default (`min=numPipelines`, `max=10×`), so the EWMA path is live (not dead code); the backoff guard at `destination.go:270` uses wall-clock `time.Now().After` (vulnerable to a forward jump).
- Does the Antithesis clock fault affect `context.WithDeadline`'s internal (monotonic) timer? `(needs human input)`

---

## Category J — Evaluation Gap-Fill

Properties added from the property-evaluation pass (`evaluation/synthesis.md`).

### processor-render-error-no-silent-loss — Processor Render/Encode Error Doesn't Silently Lose Data

| | |
|---|---|
| **Type** | Safety / Observability |
| **Property** | When the processor's `Render()`/`Encode()` fails, the message is dropped without advancing the auditor offset (so it is re-read on restart, not silently lost), and the drop is observable. |
| **Invariant** | `AlwaysOrUnreachable("processor-render-error-dropped")` / `AlwaysOrUnreachable("processor-encode-error-dropped")` at `processor.go:198-215`; structural `Always`: no `outputChan <- msg` for the failed message. Missing. |
| **Antithesis Angle** | OOM-level memory pressure; path unreachable under standard network/pause faults — value is the structural guarantee, not coverage. Distinct from the batch-layer `batch-encode-failure-no-silent-batch-loss`. |
| **Why It Matters** | Today these drops emit only `log.Error` — no metric. Invisible loss under memory pressure. |

**Open Questions:**
- See evidence file. (Gap-fill from coverage GAP-3 / wildcard F3.)

### wildcard-file-ordering-stable — Wildcard File→Pipeline Priority Is Stable

| | |
|---|---|
| **Type** | Safety |
| **Property** | When many files match a wildcard near the `filesLimit` cap, file selection/priority is deterministic and correct regardless of glob return order. |
| **Invariant** | Workload `Always`: the expected highest-priority files are tailed; `Reachable("filesLimit-cap-reached")` sentinel. Missing. |
| **Antithesis Angle** | `applyReverseLexicographicalOrdering` (`file_provider.go:362`) FIXME assumes sorted glob output; a skipped test ("Multiple Directories - Out of order input") confirms the gap. |
| **Why It Matters** | Wrong file priority silently drops the wrong files when over the cap. |

**Open Questions:**
- See evidence file. (Gap-fill from coverage GAP-4.)

### rotation-pipeline-reassignment-no-interleaving — Rotation Doesn't Interleave Across Pipelines

| | |
|---|---|
| **Type** | Safety |
| **Property** | Across a file rotation, all pre-rotation lines for a logical file arrive before any post-rotation lines (or, weaker: no offset regression across the boundary). |
| **Invariant** | Workload `Always` on per-file sequence numbers across the rotation boundary; `Sometimes("rotation-pipeline-reassignment-taken")`. Missing. |
| **Antithesis Angle** | New tailer round-robins to a *different* pipeline (`launcher.go:663`) than the draining old tailer; their auditor acks interleave under scheduling skew — the compound no single existing property covers. |
| **Why It Matters** | Cross-pipeline interleaving reorders/duplicates around every rotation; ties together `per-source-ordering-preserved` + `container-identifier-no-collision`. |

**Open Questions:**
- See evidence file. (Gap-fill from coverage GAP-6.)

### log-metadata-not-corrupted — Log Envelope Metadata Is Correct

| | |
|---|---|
| **Type** | Safety |
| **Property** | Every delivered line carries the correct `ddsource`/`service`/`hostname`/status and no stale `adaptive_sampler_sampled_count` tag from a prior processing state. |
| **Invariant** | Workload `Always`: received metadata matches the source's expected tags; no stale sampler tags. Missing. |
| **Antithesis Angle** | MRF tagging uses `msg.Origin.Service()` (wrong under container churn); the Kubernetes parser can mis-set `status` on header-parse failure. |
| **Why It Matters** | Addresses the catalog's loss-bias: metadata corruption (wrong `service`/`env`, breaking trace-log correlation and tag-RBAC) is higher-probability than byte corruption. |

**Open Questions:**
- See evidence file. (Gap-fill from wildcard F6.)

### journald-cursor-recovery-no-gap — Journald Cursor Recovery Has No Gap

| | |
|---|---|
| **Type** | Liveness / Reachability |
| **Property** | After a crash/restart (or mid-session journald disconnect), journald collection resumes from the saved cursor with no gap and bounded duplicates. |
| **Invariant** | Workload `Always(no gap)`; `Reachable("journald-cursor-recovered")`. **Requires a journald source container** (now in-scope per B-CONTAINER). Missing. |
| **Antithesis Angle** | journald uses cursor (not byte-offset) tracking; mid-session disconnect exits the tailer with no auto-restart until the next scan. Regression target `55c63957d9f` (drops first entry on restart). |
| **Why It Matters** | journald is a primary systemd-host source; cursor recovery is structurally different from file offsets and untested for ungraceful restart. |

**Open Questions:**
- See evidence file. (Gap-fill from coverage GAP-1.)

### mrf-unreliable-destination-drop-bounded — MRF Drop Bounded, Doesn't Affect Primary

| | |
|---|---|
| **Type** | Safety |
| **Property** | When the MRF (unreliable) destination's buffer is full, `NonBlockingSend` drops are bounded/observable and never compromise the primary (reliable) destination's at-least-once guarantee. |
| **Invariant** | Workload `Always`: primary intake receives all lines (at-least-once); MRF drops counted, not silent; SUT-side `Reachable("mrf-nonblocking-drop")`. **Requires a second intake + MRF enabled** (now in-scope per B-CONTAINER/dual-ship). Missing. |
| **Antithesis Angle** | `worker.go:169-175` `NonBlockingSend` silently drops MRF payloads when `DestinationSender.input` (cap 10) is full — the unreliable-destination drop path (sut-analysis S5) that had no property. |
| **Why It Matters** | MRF/dual-shipping is a production feature; a misrouted reliable/unreliable classification could silently drop primary logs. |

**Open Questions:**
- See evidence file. (Gap-fill from wildcard F7.)

## Category K — TCP Transport (B-TRANSPORT)

Added per the user's B-TRANSPORT decision (relaxing A1). The TCP intake path diverges
from HTTP and is otherwise uncovered.

### tcp-permanent-error-no-offset-advance — TCP Permanent Drop Does Not Advance the Offset

| | |
|---|---|
| **Type** | Safety |
| **Property** | On a TCP-path permanent send failure, the auditor offset is **not** advanced (unlike HTTP, which advances on permanent 4xx) — so the data is retried/re-read, not silently skipped. |
| **Invariant** | `Always`: on the TCP permanent-failure path, `output <- payload` is NOT called (confirmed asymmetry vs HTTP). Workload reconciles delivery vs offset. Missing. |
| **Antithesis Angle** | Network partition + TCP intake; the divergence from HTTP (which advances) is the key risk — a copy/refactor that aligned TCP to HTTP would introduce silent loss. |
| **Why It Matters** | The HTTP/TCP offset-advance asymmetry is real (eval-confirmed); a TCP user gets different durability semantics, undocumented. |

**Open Questions:**
- Is the HTTP-vs-TCP offset-advance asymmetry intentional? `(needs human input)` (shared with `auditor-offset-safety`)
- Does the topology run a TCP variant (required to exercise this)? `(needs human input)`

### tcp-connection-goroutine-no-leak — TCP Connection Management Doesn't Leak Goroutines/Contexts

| | |
|---|---|
| **Type** | Safety |
| **Property** | TCP connection churn under repeated failures does not leak `handleServerClose` goroutines or accumulate cancel-contexts. |
| **Invariant** | SUT-side `Reachable("tcp-handleServerClose-goroutine-exited")`; workload/SUT `Always(goroutine + context count bounded)` under sustained TCP failure. Missing. |
| **Antithesis Angle** | Network partition cycles against the TCP intake exercise `handleServerClose` (`connection_manager.go:125`) and the in-loop `defer cancel()` accumulation (`connection_manager.go:102-103`) — a slow leak during prolonged outages. |
| **Why It Matters** | A leak in a long-running agent → memory/FD growth; the TCP path has no leak property today. |

**Open Questions:**
- Does the topology run a TCP variant? `(needs human input)`

## Catalog-Wide Open Questions

These concern the analysis/topology, not a single property (per
`scratchbook-setup.md` they live here, not under a property):

- **Fault availability.** Node-termination and clock faults are commonly disabled by
  default. Category C, and the clock properties in D/I, are vacuous without them.
  Confirm with the tenant (A7).
- **Default config values.** `logs_config.pipelines`, `close_timeout`,
  `stop_grace_period`, sampler `RateLimit`/`BurstSize`/`MaxPatterns`, and the
  failover/sampling/atomic-write/collect-all toggles must be pinned in the topology;
  several properties are vacuous or config-gated under defaults (A2–A5).
- **Fakeintake fidelity.** Several properties need fakeintake to record per-payload
  HTTP status codes and (ideally) byte offsets, not just line counts (A6).
- **Intake ordering semantics.** Whether the real intake preserves submission order
  bounds how strong `per-source-ordering-preserved` can be at the workload layer.
- **Adaptive sampling semantics** (owner's design doc): per-interval re-evaluation of
  value status; config-order pattern matching; warmup period.

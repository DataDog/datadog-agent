---
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-29
lens: Implementability
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

# Implementability Evaluation — Logs Pipeline Property Catalog

## Scope and Method

This evaluation applies the **Implementability** lens to all 36 properties in the
catalog. For each property the question is: can the invariant actually be checked
given the three-container topology (logs-agent SUT, fakeintake, workload; shared
logs volume; persistent registry volume), the workload constraints, and the
codebase state at commit `8ff8f30e10b`? Every SUT-side assertion is net-new;
there is no existing Antithesis SDK in the repo.

The evaluation is organized by finding severity:

- **BLOCKER** — property cannot be checked at all as specified; requires explicit
  remediation before including it in a run.
- **SIGNIFICANT CONCERN** — implementable with non-trivial prerequisite work or
  a scope change; the as-written invariant will not fire or will fire vacuously.
- **MINOR CONCERN** — implementable with small clarifications; the approach is
  mostly sound.
- **PASS** — implementable as described; no structural obstacle identified.
- **UNCERTAINTY** — depends on an unresolved external question (usually the
  Antithesis clock-fault monotonic-vs-wall question).

---

## Findings

### F1 — Clock-Fault Properties: Monotonic vs. Wall Clock Is an Unresolved Gate

**Slugs:** `clock-jump-no-extra-sampling`, `clock-jump-no-backoff-underflow`,
`multiline-not-split-across-pipelines` (timer-flush variant)

**Severity:** BLOCKER (for the clock-specific failure paths)

**Concern:** All three properties rely on the Antithesis clock fault moving Go's
monotonic clock. Go's `time.Now()` on Linux reads both wall time and a monotonic
counter from the VDSO. Three mechanisms use `time.Now()` or `time.Timer`:

1. `AdaptiveSampler.Process()` credit refill uses `time.Now()` (production path,
   no test injection): backward jump → negative credits; forward jump → credit
   windfall.
2. `http.Destination` backoff guard at `destination.go:270` uses two sequential
   `time.Now()` calls; forward jump between them skips the guard.
3. `MultiLineHandler.flushTimer` and the preprocessor flush timers use
   `time.Timer`, which internally uses the monotonic clock.

If Antithesis's clock fault only adjusts `CLOCK_REALTIME` (wall clock) and not
the monotonic counter (HPET/TSC or vDSO monotonic offset), then:
- `time.Now().Sub()` in the sampler is unaffected (Go's `Sub` uses the monotonic
  component when both times were obtained from `time.Now()`).
- `time.Timer` fires are unaffected (they are monotonic-only).
- The backoff guard uses `time.Now().After(blockedUntil)` — if `blockedUntil`
  was set with wall time, a wall-only jump would affect it; but `After` also uses
  monotonic when available.

The catalog correctly flags this as `(needs human input)` but does not block
the affected properties from being listed without the caveat that the **entire
clock cluster is vacuous** if the monotonic clock is immune.

**Evidence:** `sampler.go:125` shows `now: time.Now` as the production injection
point; `clock-jump-no-extra-sampling.md` investigation log confirms this.
`multiline_handler.go:85-158` uses `time.Timer`. The open question in both files
is explicitly `(needs human input)`.

**Suggested action:** Before launching any clock-property run, confirm with the
Antithesis tenant whether their clock fault adjusts the kernel monotonic clock
(via `clock_adjtime(CLOCK_MONOTONIC)` or a hypervisor monotonic-counter override)
or only `CLOCK_REALTIME`. If wall-only, all three properties should be scoped to
the wall-clock-observable paths only (the backoff `.After()` guard remains
vulnerable to wall-only jumps; the sampler and timer paths are not). Mark the
timer-flush variant of `multiline-not-split-across-pipelines` as unreachable in
that case.

---

### F2 — `bounded-memory-under-backpressure`: CGO Dependency and Soak Duration

**Slug:** `bounded-memory-under-backpressure`

**Severity:** SIGNIFICANT CONCERN (two independent sub-concerns)

**Concern A — CGO build required for the OOM scenario:**
The zstd C-heap leak (`0d9dfc76f46`) only applies to the CGO zstd backend
(`pkg/util/compression/impl-zstd/`), which allocates `ZSTD_CCtx` on the C heap,
invisible to the Go GC. The nocgo backend (`impl-zstd-nocgo/`, using
`klauspost/compress/zstd`) is GC-visible; a leak there will not cause unbounded
RSS growth. If the Antithesis topology builds the agent with the nocgo tag (which
is common in CI to avoid the CGO toolchain), the entire OOM scenario is
unreachable. RSS will remain bounded by Go GC pressure alone, and the `Always(rss
< 2 × baseline)` assertion will trivially pass without exercising the fix.

The `Reachable("zstd-cctx-close-on-reset")` SUT-side assertion at `batch.go:76`
is valuable regardless of CGO vs. nocgo — it confirms the error path runs. But
the workload RSS assertion is only meaningful against the CGO build.

**Evidence:** `bounded-memory-under-backpressure.md` investigation log confirms:
"The OOM scenario (unbounded C-heap growth) is CGO-specific. Test topology must
use CGO zstd to exercise the original bug scenario." The open question `(needs
human input)` in the catalog is unanswered.

**Concern B — Soak duration for the C-heap leak:**
The original bug required prolonged outage + many encode-error cycles to manifest
multi-GiB growth. Under standard Antithesis run durations (minutes to a few
hours), the leak rate may not grow RSS enough to cross the `2 × baseline`
threshold, especially since `resetBatch()` on normal send success also calls
`Close()`. The regression target is "does any new code path bypass
`resetBatch()`" — that is structural, not soak-dependent. But the workload RSS
assertion needs enough cycles to be sensitive.

**Concern C — Encode errors are not reachable under standard faults:**
As documented in the catalog, `Serialize()`/`compressor.Close()` errors require
OOM-level memory pressure. Standard network/process-pause faults will not trigger
the encode-error path, so the retry of `resetBatch()` call sites 1 and 2 (in
`processMessage()`) will not fire. The `Reachable` and `Sometimes` assertions for
those sites will remain unmet. The `defer resetBatch()` in `sendMessages()` (site
3) fires on every successful send, so that `Reachable` is always met.

**Suggested action:**
1. Confirm build uses CGO zstd before claiming OOM regression coverage.
2. Split the property into two invariants: (a) structural correctness of
   `resetBatch()` paths (achievable without soak or CGO), and (b) RSS growth
   assertion (requires CGO build + soak). Mark (b) as a separate, lower-priority
   property.
3. Accept that the encode-error `Reachable` assertions at sites 1 and 2 will be
   `AlwaysOrUnreachable` under standard faults (the catalog already acknowledges
   this for the encode-failure property but not for the memory property).

---

### F3 — `no-goroutine-leak-after-stop`: External Goroutine Count Is Accessible But Requires Topology Work

**Slug:** `no-goroutine-leak-after-stop`

**Severity:** MINOR CONCERN

**Concern:** The catalog's open question asks whether the workload driver can
query the SUT's goroutine count externally. The answer is: **yes, but it requires
intentional configuration.**

The agent's main binary (`cmd/agent/subcommands/run/command.go:14`) imports
`_ "net/http/pprof"`, which registers pprof handlers on `http.DefaultServeMux`.
The profiler component (`comp/core/profiler/impl/profiler.go:225`) confirms the
goroutine endpoint is at `http://127.0.0.1:{expvar_port}/debug/pprof/goroutine`,
where `expvar_port` defaults to 5000 (confirmed in
`pkg/config/setup/common_settings.go:547`).

The workload can poll `http://logs-agent:5000/debug/pprof/goroutine?debug=1`
across the container network. This is external to the SUT process — no SUT-side
pprof SDK call is needed.

**Two caveats:**

1. The expvar server binds to `127.0.0.1` (loopback), not `0.0.0.0`. The
   workload cannot reach it across the container network boundary. The topology
   must either: (a) configure `DD_EXPVAR_PORT` to bind on `0.0.0.0` or a
   specific interface, or (b) add a SUT-side goroutine count export via the
   Antithesis SDK's random-data interface (simpler). The profiler.go code
   constructs the URL as `127.0.0.1:{expvar_port}` — confirming loopback-only.

2. Goroutine count baseline is noisy: the full agent binary has many non-log
   goroutines (telemetry, health, expvar server, etc.) that don't participate in
   the logs lifecycle. The baseline must be established while the logs pipeline is
   live but before Stop(), not before agent startup. The evidence file flags this
   as `(needs human input)`.

**The WrappedSource fire-and-forget goroutine leak** (Leak 3 in the evidence
file) requires container source churn to trigger. The base topology uses file
tailing, not container sources, so this leak path won't fire unless the topology
adds container churn.

**Suggested action:**
- Expose goroutine count via a SUT-side `assert.Sometimes("goroutine-count-at-baseline-after-stop", ...)` assertion that calls `runtime.NumGoroutine()` internally. This avoids the network-binding problem entirely.
- Document that the WrappedSource goroutine leak (Leak 3) requires container
  source topology; scope the base property to Leaks 1, 2, and 4.
- Pin `close_timeout` to a short value (e.g., 10s) so Leak 4 (`forwardMessages`
  hang) resolves within the test window rather than the 35s grace period.

---

### F4 — `secrets-redacted-before-send`: `Unreachable` on "Any Bypass Path" Cannot Prove a Negative

**Slug:** `secrets-redacted-before-send`

**Severity:** SIGNIFICANT CONCERN

**Concern:** The catalog specifies a SUT-side `Unreachable` assertion at "any
path where a `message.Message` leaves the agent process without first passing
through `applyRedactingRules`." This is not feasible as a single assertion point
for several reasons:

1. **There is no single exit gate.** The agent sends data via `destination.go`'s
   `unconditionalSend()`, the TCP `sendAndRetry`, and `NonBlockingSend`. Inserting
   `Unreachable` at every outbound byte-send point would require touching every
   network write in the codebase — not scoped to the logs pipeline.

2. **The workload-side check proves the positive case, not the negative.** The
   workload approach (embed `secret12345`, assert `[REDACTED]` arrives) correctly
   tests the normal path. What it cannot test is whether a novel code path
   introduced in a future commit bypasses the processor — that requires code
   coverage on a static structure, which the `Unreachable` was intended to capture.

3. **The practical risk vector is new code, not existing code.** The evidence
   file concludes that all current exit paths (channel launcher, serverless Flush,
   encode-error drop, failover routing, NonBlockingSend) are safe. The security
   value of Antithesis here is finding *new* bypass paths introduced by code
   changes during a multi-hour run — not confirming the static structure today.

4. **A better instrument:** Place the `Unreachable` at the processor's output
   channel: `Unreachable("message-left-processor-without-redaction")` inside
   `processMessage()` before `outputChan <- msg` if `msg.Content` still contains
   the known sentinel pattern. This is a positive-content assertion (can be
   measured), not a "no bypass path exists" structural claim (which cannot be
   measured at runtime).

**Suggested action:**
- Replace the SUT-side `Unreachable("any bypass path")` with a workload-side
  content assertion (already described: no received body contains `secret12345`).
- Add a SUT-side `Always` at the processor output that the content has been
  transformed if the message's source has `MaskSequences` configured — this is
  a positive check, not a negative structural claim.
- Scope the property's invariant to "the workload-configured redaction rule fires
  on the workload-generated secret pattern in all received payloads," which is
  fully implementable.
- Document that proving *no possible bypass path* exists is not a runtime
  property; it is a static analysis or code-review concern.

---

### F5 — `per-source-ordering-preserved`: Break #1 Requires `pipeline_failover.enabled=true`; Break #2 Requires Precise Rotation Ordering

**Slug:** `per-source-ordering-preserved`

**Severity:** SIGNIFICANT CONCERN

**Concern — Break #1 (failover-induced reorder):**
`pipeline_failover.enabled` defaults to `false`
(`config.BindEnvAndSetDefault("logs_config.pipeline_failover.enabled", false)`
confirmed in the evidence file). The topology document states it will be "toggled
per scenario via custom faults," but this means Break #1 is only exercised when
the topology explicitly enables it. With the topology pinned to
`logs_config.pipelines: 2`, there *are* two pipelines — but without failover
enabled, messages always go to the same pipeline (no cross-pipeline reorder).
The `Sometimes("failover-routing-triggered")` assertion will not fire under
default config.

**Concern — Break #2 (rotation-boundary reorder):**
Break #2 requires: (a) an old tailer on pipeline 0 draining, and (b) a new
tailer on pipeline 1 sending simultaneously. This is structurally achievable via
a workload-driven file rotation. However, detecting the out-of-order delivery at
the fakeintake requires that the two pipelines deliver their payloads
concurrently and in an interleaved way before the intake processes them in a
single request sequence. The fakeintake records arrival time at millisecond
granularity — if the pipeline 0 drain completes before pipeline 1 starts (which
can happen if rotation drains quickly), the reorder window closes and the property
passes vacuously.

**Concern — A6 fakeintake requirement (per-payload sequence numbers):**
Sequence-number checking at fakeintake requires the workload to embed per-source
monotonic sequence numbers in each log line (planned) and fakeintake to decode
them from the log body. The current `LogAggregator` stores `Message` field text
— the sequence number would need to be in the message text and parsed by the
workload's reconciliation logic, not the fakeintake itself. This is workload
complexity, not a fakeintake gap, and is implementable.

**Suggested action:**
- For Break #1: explicitly enable `pipeline_failover.enabled=true` in at least
  one topology variant; or accept that Break #1 is config-gated and scope the
  property to Break #2 only for the default run.
- For Break #2: shorten `close_timeout` (e.g., to 10s instead of 60s) to widen
  the window where old and new tailers coexist, making the concurrent-delivery
  race more likely.
- The `Sometimes("failover-routing-triggered")` reachability assertion should be
  marked conditional on `pipeline_failover.enabled=true`.

---

### F6 — `offset-no-regression-on-seek-error`: Seek-Error Injection Is Non-Trivial

**Slug:** `offset-no-regression-on-seek-error`

**Severity:** SIGNIFICANT CONCERN

**Concern:** The property requires injecting a `Seek()` failure at the OS level.
The production `fileOpenerImpl` wraps `internalOpener.OpenLogFile()` which calls
through to the real OS `open()`/`lseek()` system calls via afero. An Antithesis
filesystem fault (if supported in the tenant configuration) could inject `EIO`
into system calls on the shared logs volume.

However, two complications:

1. **Fault granularity:** Antithesis must inject `EIO` specifically on `lseek()`
   for the specific file the tailer is seeking, not on all filesystem operations
   (which would break the entire test). This requires either a targeted fault
   (file-path-based I/O error injection) or a custom fault hook. Standard
   Antithesis filesystem faults may not offer this granularity.

2. **The tailer uses the OS's real filesystem via afero:** The `opener` wraps
   `os.File`, so afero's `MemMapFs` is not used in production paths. Seek errors
   from a real volume fault are plausible — but the fault must be carefully
   timed: it must fire during the `setup()` call's `f.Seek(offset, io.SeekStart)`
   (`tailer_nix.go:36`), not during `read()` calls (which have different error
   handling). There is no hook in the codebase to inject seek errors without
   modifying the tailer.

3. **Alternative approach:** A simpler instrumentation is to add an error-injection
   shim in the `fileOpenerImpl` that can be toggled by an environment variable.
   This is a small code change (the opener interface makes it clean) and avoids
   depending on OS-level fault granularity.

**Suggested action:**
- Confirm whether the Antithesis tenant's filesystem fault can inject `EIO` on
  specific `lseek()` calls on the shared volume.
- If not: add an error-injection shim to the `FileOpener` interface that returns
  a configurable error on `Seek()`. This is a 20-line change to the opener and
  does not modify production logic.
- Lower the priority of this property if fault injection is infeasible; the bug
  (`_, _` discard of seek error) is static and can be found by code review. The
  Antithesis value is in verifying the duplicate-count bound holds, not in finding
  the static bug itself.

---

### F7 — `registry-format-migration-safe`: Two-Agent-Version Topology Not Designed

**Slug:** `registry-format-migration-safe`

**Severity:** SIGNIFICANT CONCERN

**Concern:** The property's main test scenario is: pre-seed a v1 registry, start
a v2 agent, verify all offsets are preserved. The catalog's open question asks
"Can the topology pre-seed a v1 registry / run two agent versions?" The current
topology design (single SUT container, single agent binary) has no mechanism for
this.

**Implementable workarounds:**
1. **Pre-seed approach (feasible):** The workload can write a v1-format
   `registry.json` to the persistent volume before the agent starts. The agent
   at startup reads it, migrates in memory, and flushes v2. The workload then
   verifies the offsets in the flushed v2 file match expectations. This requires
   no second agent version — just a pre-written JSON fixture.

2. **Reachability assertions (feasible):** `Reachable("v1-migration-path-taken")`
   and `Reachable("v0-migration-path-taken")` in `unmarshalRegistry()` are net-new
   SUT instrumentation that fire deterministically when the pre-seeded registry is
   read. These are implementable.

3. **`Unreachable("unknown-version-empty-registry")`:** This is a structural guard
   on the `default` branch. Implementable as a SUT-side assertion; it fires if a
   registry with an unrecognized version number is loaded. The workload can test
   this by pre-seeding a registry with `"Version": 99`.

**The real gap is node-termination during first post-migration flush.** This
requires the node-termination fault to be enabled (A7). Without it, the
migration + crash scenario in the property's "Why It Matters" is unreachable.

**Suggested action:**
- Implement via pre-seeded v1 registry fixture (does not require two agent
  versions). Document this approach in the property evidence file.
- Mark the "crash during first post-migration flush" variant as dependent on
  node-termination fault being enabled (same A7 dependency as Category C).

---

### F8 — `container-identifier-no-collision` and `container-addremovesource-ordering`: Container Source Not in Default Topology

**Slugs:** `container-identifier-no-collision`, `container-addremovesource-ordering`,
`container-collect-all-startup-race`, `no-services-store-deadlock`

**Severity:** SIGNIFICANT CONCERN (for `container-identifier-no-collision` and
`container-addremovesource-ordering`); MINOR CONCERN (for the others)

**Concern:** The base topology uses file tailing from a shared volume — no
Docker socket, no container runtime, no container log directory. The catalog's
cross-cutting assumption A5 says "add a journald/docker source only if expanding
into Category H container-source races later." This means:

- `container-identifier-no-collision`: requires two tailers on the same file
  identifier (container rotation). Without a container source, the only way to
  trigger this is with a file source that rotates to the same path, which can
  use the `"file:"+path` identifier collision. **This is actually achievable with
  the file tailer alone** — rotate `app.log` to `app.log.1` and start a new
  `app.log`. The new tailer uses `"file:"+"/var/log/app.log"` (same as the old
  one). So the property is exercisable without container sources.

- `container-addremovesource-ordering`: the `WrappedSource.Start()`/`Stop()`
  fire-and-forget goroutines (`source.go:34,42`) are only in the **container
  launcher** (`pkg/logs/launchers/container/tailerfactory/tailers/source.go`).
  The file launcher does not use `WrappedSource`. Without a container source in
  the topology, the `go t.Sources.AddSource(t.Source)` goroutine is never
  spawned, and this property is **vacuous under the base file-tailing topology**.

- `container-collect-all-startup-race`: requires `container_collect_all=true` and
  a container runtime. Vacuous in the file-tailing topology.

- `no-services-store-deadlock`: the deadlock requires at least one `Services`
  subscriber — which the evidence file confirms is **zero in production today**
  (only test code subscribes). The property is a regression guard. Vacuous under
  any topology where no subscriber is registered.

**Suggested action:**
- `container-identifier-no-collision`: rewrite the precondition to use the
  file-rotation mechanism (shared path → same identifier). The property is then
  exercisable in the base topology.
- `container-addremovesource-ordering`, `container-collect-all-startup-race`:
  require a Docker-socket or container-log-directory extension to the topology.
  Mark as out of scope for the initial run; plan as Phase 2 topology addition.
- `no-services-store-deadlock`: mark as a regression guard with `Reachable`
  coverage conditional on adding a Services subscriber (can be done by a test-only
  subscriber registered at SUT startup for Antithesis runs).

---

### F9 — Fakeintake Gaps: Per-Payload HTTP Status Code and Response Override

**Scope:** Catalog-wide (affects `auditor-offset-safety`, `no-loss-and-duplicate-same-line`,
`permanent-error-no-retry`, `retryable-no-retry-after`, `batch-encode-failure-no-silent-batch-loss`)

**Severity:** MINOR CONCERN (partially already resolved by existing fakeintake capability)

**Concern:** The catalog (A6) assumes fakeintake records per-payload HTTP status
codes and can be driven to return chosen status codes. Investigation of the
fakeintake codebase confirms:

**Already implemented:**
- `api.ResponseOverride` struct and `POST /fakeintake/configure/override` endpoint
  (`server/server.go:631-666`) allow the workload to set the response status code
  for any endpoint and method combination.
- `client.ConfigureOverride(override)` is the client-side method
  (`client/client.go:485-504`).
- This covers the "drive fakeintake to return chosen status codes" requirement.

**Not implemented (extension needed):**
- The `api.Payload` struct stores `Timestamp`, `APIKey`, `Data`, `Encoding`, and
  `ContentType` — but **not the HTTP status code that was returned** for that
  payload. The `LogAggregator` stores `Log` structs with `Message`, `Status`,
  `Timestamp`, `HostName`, `Service`, `Source`, `Tags` — no HTTP status code.
- Per-payload correlation between the workload-injected status code (via
  `ResponseOverride`) and specific received payloads requires adding a
  `ResponseStatusCode int` field to `api.Payload` and recording it at the server's
  request handler. This is a targeted fakeintake extension.

**What this blocks:**
- `no-loss-and-duplicate-same-line`: needs to detect "4xx-rejected line reappears
  after restart" — requires knowing which specific payloads were 4xx'd. Without
  per-payload status codes, only approximate reasoning (sequence number timing vs.
  override activation window) is possible.
- `auditor-offset-safety`: same gap for correlating 4xx drops to offset advances.

**What this does NOT block:**
- `permanent-error-no-retry` and `retryable-no-retry-after`: the workload can use
  `ConfigureOverride` to set a status code, observe the agent metric
  `DestinationLogsDropped`, and verify a single delivery at fakeintake. No per-payload
  status code is needed.

**Suggested action:**
- Add `ResponseStatusCode int` to `api.Payload` (small server-side change);
  record it in the catch-all handler alongside the existing `Timestamp` field.
- Add the field to `ParsedPayload` for the JSON query API.
- This unblocks the two affected properties and is a self-contained 30-line
  fakeintake extension.
- Tag this as a prerequisite for the `no-loss-and-duplicate-same-line` and
  `auditor-offset-safety` properties specifically.

---

### F10 — `bounded-memory-under-backpressure` and `queued-payloads-eventually-sent`: ≥240s Quiet Window Required

**Slugs:** `queued-payloads-eventually-sent`, `bounded-memory-under-backpressure`

**Severity:** MINOR CONCERN

**Concern:** `queued-payloads-eventually-sent` needs a ≥240s fault-quiet window
after partition recovery (confirmed in evidence: max backoff 120s, 4 consecutive
successes to fully recover, plus 100ms poll latency). Under Antithesis's default
behavior, fault injection continues throughout the run. If Antithesis does not
support `ANTITHESIS_STOP_FAULTS` or an equivalent "quiet period" trigger, a
≥240s quiescence cannot be guaranteed, and the `Eventually(all-payloads-received)`
check will be non-deterministically satisfied only by lucky scheduling.

`bounded-memory-under-backpressure` needs a soak window (hours, not minutes) for
the CGO leak to manifest measurably. Under standard Antithesis run durations
(minutes to ~1 hour), the leak may not exceed the `2 × baseline` threshold.

**These are not blockers** because:
1. Antithesis supports `eventually_` command templates which run after
   `ANTITHESIS_STOP_FAULTS` — this is a standard Antithesis pattern, not missing
   infrastructure.
2. The `queued-payloads-eventually-sent` confirmation (`Sometimes("destination
   transitions from retrying to not-retrying")`) works during the normal run,
   without a quiet window; only the final workload reconciliation (`Always(count
   >= injected - permanent_drops)`) needs the quiet window.
3. The bounded-memory property can split into (a) structural compressor-path
   coverage (run-time achievable) and (b) RSS growth assertion (long-soak only).

**Suggested action:**
- Use the `eventually_`/`finally_` command template for the post-recovery
  reconciliation assertions in `queued-payloads-eventually-sent` and
  `at-least-once-no-loss`.
- For `bounded-memory-under-backpressure`, split into a structural property (add
  `Reachable` at `resetBatch()` — achievable in short runs) and a soak property
  (RSS assertion — schedule as a separate long-run Antithesis run).

---

### F11 — `at-least-once-no-loss`: 60s `close_timeout` Requires Long Fault Windows

**Slug:** `at-least-once-no-loss`

**Severity:** MINOR CONCERN

**Concern:** The rotation-under-backpressure loss path (`tailer.go:306-338`)
requires the partition to last >60s (default `close_timeout`) before the old
tailer's `stopForward()` fires. Standard Antithesis network partition durations
may be shorter. If the partition clears before 60s, the old tailer drains
successfully and no loss occurs — the property passes vacuously (no loss
observed, but the loss path was not exercised).

The `Reachable("bytes-missed-on-rotation")` assertion in
`backpressure-no-rotation-loss` would confirm the loss path was hit, but only if
`close_timeout` expires during the test run.

**Suggested action:**
- Set `close_timeout` to a shorter value (e.g., 5–10s) in the topology
  configuration. This makes the loss path reachable within a typical fault window
  and reduces the required partition duration from >60s to >10s.
- Alternatively, use a custom workload action that directly calls
  `StopAfterFileRotation()` via a config toggle — but this is harder to wire.
- Document the `close_timeout` sensitivity in the property's preconditions.

---

### F12 — `registry-survives-crash`: Volume Rename Semantics Must Be Confirmed

**Slug:** `registry-survives-crash`

**Severity:** MINOR CONCERN

**Concern:** The atomic writer's crash safety relies on `os.Rename()` being
atomic on the volume hosting the registry. If the persistent volume in the
Antithesis topology is:
- An overlay filesystem or a bind-mount over NFS: `os.Rename()` may not be
  atomic across a crash, invalidating the property's atomic-writer assertion.
- A local filesystem (ext4/xfs within the container): `os.Rename()` is
  crash-atomic on a single filesystem.

The topology document notes this as an open question: "confirm the Antithesis
volume gives durable rename semantics."

**Suggested action:**
- Use a local volume (e.g., Antithesis's persistent disk, not a network-mounted
  volume) for the registry path. Confirm with the Antithesis tenant that the
  volume type supports atomic rename.
- For the non-atomic writer test: force `DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false`
  (confirmed feasible — this env var overrides the default on any platform).

---

### F13 — Category C (Crash Recovery) Properties: Node-Termination Fault Must Be Explicitly Enabled

**Slugs:** `registry-survives-crash`, `registry-recovers-after-crash`,
`registry-format-migration-safe`, `auditor-offset-safety`, `at-least-once-no-loss`,
`no-loss-and-duplicate-same-line`

**Severity:** MINOR CONCERN (flag, not blocker — this is already in the catalog
as A7, but the implementation consequence deserves emphasis)

**Concern:** All six properties require the agent process to be killed
ungracefully (`kill -9` / node termination). The catalog (A7) flags that node
termination faults are commonly default-OFF in Antithesis tenants. If the run
launches without node-termination enabled:
- No crash-recovery properties can fire.
- `registry-survives-crash` and `registry-recovers-after-crash` vacuously pass
  (registry is always valid if never crashed).
- `auditor-offset-safety` and `at-least-once-no-loss` never see a restart,
  so the offset regression and duplicate-delivery checks are untestable.

**Suggested action:**
- Explicitly request node-termination fault enablement from the Antithesis tenant
  before launch (this is a configuration setting, not a code change).
- Add a `Reachable("agent-started-with-non-empty-registry")` assertion in
  `recoverRegistry()` as a sentinel — if this never fires during the run, the
  node-termination fault is not engaged.

---

### F14 — `adaptive-sampler-no-aliasing`: Single-Goroutine — CPU-Pause Value Is Weak

**Slug:** `adaptive-sampler-no-aliasing`

**Severity:** MINOR CONCERN

**Concern:** `AdaptiveSampler.Process()` is confirmed single-goroutine (one per
source). CPU-pause faults cannot introduce data races in this code. The aliasing
bug (`7687b846b2a`) was a *logical* bug (wrong entry updated after bubble swap)
not a concurrency bug. Antithesis's strength here is *state-space exploration
over diverse input sequences*, not thread-interleaving.

The property is still implementable and valuable — Antithesis's diverse input
generation (many log patterns, varied arrival rates) can reach the ≥3-bubble-swap
state that triggered the original bug. But the property catalog overstates CPU
throttle as the trigger; the real trigger is *sequence diversity*.

**Suggested action:**
- Clarify in the property evidence file that the CPU-throttle angle is weak for
  this property; the Antithesis value is in diverse pattern sequences.
- Add a `Sometimes("pattern-table-resorted-with-3-or-more-swaps")` sentinel to
  confirm the target state is reached. The existing unit test in `sampler_test.go`
  covers aliasing for the `sampled` field; the `credits` and `matchCount`
  independence under swap needs a separate check (flagged as `partial` in the
  catalog).

---

## Passes

The following properties are **implementable as described**, with no structural
obstacle identified. Minor "needs human input" open questions exist (config pins,
topology values) but these don't affect implementability.

| Slug | Rationale |
|------|-----------|
| `logs-not-modified-in-transit` | Workload CRC in log body; fakeintake verifies. SUT-side assertion at `outputChan <- msg` is a clean injection point. All bypass paths investigated and resolved. |
| `oversized-line-truncation-safe` | `TRUNCATED` string in received body is observable at fakeintake. `shouldTruncate` branch in `single_line_handler.go` is a clean `AlwaysOrUnreachable` point. The truncate-then-batch-drop loop (an extended concern) is observable via sequence gaps. |
| `multiline-not-split-across-pipelines` (rotation path) | Confirmed structural bug: `stopForward()` before `decoder.Stop()`. BEGIN/END markers in workload log lines make split events detectable at fakeintake. The SUT-side `Sometimes(flush-during-rotation)` is a clean injection point. |
| `auditor-offset-safety` | Sequence-number tracking at fakeintake covers the core invariant. 4xx/2xx behavior observable via `ConfigureOverride` + `DestinationLogsDropped` metric. Per-payload status code gap is a MINOR prerequisite (see F9). |
| `container-identifier-no-collision` | Reframeable as file-rotation identifier collision (same `"file:"+path`). File-source topology sufficient (see F8). |
| `at-least-once-no-loss` | Implementable with short `close_timeout` + network partition. Sequence-number reconciliation at fakeintake after quiet window. `BytesMissed` in-memory caveat documented. |
| `no-loss-and-duplicate-same-line` | Implementable with `ConfigureOverride` for 4xx. Per-payload status code extension is a prerequisite for the 4xx-replay scenario (see F9). |
| `registry-survives-crash` | Atomic writer: pre-seed registry, kill agent, verify JSON validity. Non-atomic path: `DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false`. Volume rename semantics need confirmation (F12). |
| `registry-recovers-after-crash` | `recoverRegistry()` path is the only startup read path (confirmed). `Sometimes("registry-recovered-with-non-empty-offsets")` is a clean `Reachable` point. |
| `sampling-exact-count` | Fakeintake count of pattern-matching lines per time window. `AdaptiveSampler` must be enabled (A2) — topology already plans this. |
| `high-value-never-sampled` | Workload writes FATAL/ERROR lines; fakeintake verifies 100% delivery. SUT-side `Always` at `isImportant()` early-return is a clean point. |
| `sampling-reachable-under-load` | `Reachable` at `tlmAdaptiveSamplerDropped.Inc()` fires when workload rate exceeds `RateLimit`. Vacuous sentinel — if it doesn't fire, the other sampling properties are not exercised. |
| `no-services-store-deadlock` | Requires registering a test subscriber (currently zero in production). With a test subscriber added to the Antithesis SUT build, the deadlock loop is exercisable. The watchdog timeout assertion is clean. |
| `no-send-on-closed-on-shutdown` | CPU-pause during `forwardWithFailover` blocked on `InputChan`. `Unreachable` with a recover() wrapper is a clean injection point. No preconditions blocked. |
| `idempotent-stop` | Concurrent shutdown scenarios (signal + API). `Unreachable` with recover() around close(). No blockers. |
| `clean-shutdown-completes` | SIGTERM workload action; `Sometimes("shutdown-completed-within-30s")`. Network partition → saturated `outputChan` → tailer hang is the exact scenario. |
| `auditor-drains-on-stop` | The H5 drain gap (select non-determinism) is confirmed. `Sometimes(exited-with-buffered-items)` as a bug trap fires under CPU-pause. |
| `backpressure-no-rotation-loss` | Network partition + rotation. `Reachable("bytes-missed-on-rotation")` at `tailer.go:325`. Requires short `close_timeout` (see F11). |
| `backpressure-before-drop` | Network partition without rotation. `Always(BytesMissed==0)`. `logs_component_utilization.ratio≈1.0` as saturation proxy. |
| `queued-payloads-eventually-sent` | `eventually_` command template for post-recovery reconciliation. Recovery window (≥240s) addressed via `eventually_` pattern (see F10). |
| `permanent-error-no-retry` | `ConfigureOverride` for 4xx. `DestinationLogsDropped` metric. Single-delivery at fakeintake verifiable. |
| `retryable-no-retry-after` | `ConfigureOverride` for 429, then 200. `Always(DestinationLogsDropped unchanged during 429 phase)`. |
| `batch-encode-failure-no-silent-batch-loss` | `AlwaysOrUnreachable` is the correct framing (encode errors unreachable under standard faults). Structural assertion at `outputChan` after encode-error path is implementable. |
| `graceful-degradation-on-startup` | `startstop` sequential ordering confirmed. `Unreachable("nil-destinations-context-dereference")` is justified by static ordering. Network partition at startup exercises TCP fallback. |
| `transport-switch-no-loss` | `Reachable("transport-switch-TCP-to-HTTP")` via `smartHTTPRestart`. Workload: cumulative count non-decreasing. CPU throttle during `partialStop` is the Antithesis angle. |

---

## Uncertainties

These items cannot be resolved without external input and could change the
implementability assessment of the affected properties.

| Uncertainty | Affected slugs | Resolution path |
|-------------|---------------|-----------------|
| Does Antithesis's clock fault move Go's monotonic clock or only `CLOCK_REALTIME`? | `clock-jump-no-extra-sampling`, `clock-jump-no-backoff-underflow`, `multiline-not-split-across-pipelines` (timer variant) | Ask Antithesis tenant: does their clock fault use `clock_adjtime(CLOCK_MONOTONIC)` or hypervisor-level monotonic counter manipulation? |
| Does the Antithesis tenant's filesystem fault support targeted `lseek()` error injection on a specific file? | `offset-no-regression-on-seek-error` | Ask tenant about filesystem fault granularity; fallback: add error-injection shim to `FileOpener`. |
| Does the Antithesis tenant have node-termination fault enabled by default? | Category C + `auditor-offset-safety`, `at-least-once-no-loss`, `no-loss-and-duplicate-same-line` | Confirm with tenant; request enablement. |
| Does the topology use CGO zstd or nocgo zstd? | `bounded-memory-under-backpressure` (OOM sub-concern) | Confirm build flags; use CGO build if OOM regression is a target. |
| What is the Antithesis volume type for the persistent registry? Does `os.Rename()` have atomic crash semantics on it? | `registry-survives-crash` | Confirm with Antithesis tenant (should be a local volume, not NFS/overlay). |

---

## Summary Table

| Slug | Status | Finding |
|------|--------|---------|
| `clock-jump-no-extra-sampling` | BLOCKER (clock) | F1 — monotonic-vs-wall gate |
| `clock-jump-no-backoff-underflow` | BLOCKER (clock) | F1 — monotonic-vs-wall gate |
| `multiline-not-split-across-pipelines` (timer path) | BLOCKER (clock) | F1 — monotonic-vs-wall gate |
| `bounded-memory-under-backpressure` | SIGNIFICANT CONCERN | F2 — CGO build + soak duration + encode-error unreachability |
| `secrets-redacted-before-send` | SIGNIFICANT CONCERN | F4 — `Unreachable` on "any bypass path" not implementable; reframe to positive-content check |
| `per-source-ordering-preserved` | SIGNIFICANT CONCERN | F5 — Break #1 requires failover enabled; Break #2 needs short `close_timeout` |
| `offset-no-regression-on-seek-error` | SIGNIFICANT CONCERN | F6 — seek-error injection requires either targeted FS fault or opener shim |
| `registry-format-migration-safe` | SIGNIFICANT CONCERN | F7 — two-version topology not designed; workable via pre-seeded fixture |
| `container-addremovesource-ordering` | SIGNIFICANT CONCERN | F8 — requires container source, not present in base topology |
| `container-collect-all-startup-race` | SIGNIFICANT CONCERN | F8 — requires container source + runtime |
| `no-services-store-deadlock` | SIGNIFICANT CONCERN | F8 — zero production subscribers; needs test subscriber in Antithesis SUT build |
| `no-goroutine-leak-after-stop` | MINOR CONCERN | F3 — pprof on loopback only; use SUT-side `runtime.NumGoroutine()` assertion instead |
| `fakeintake per-payload status code` | MINOR CONCERN (prerequisite) | F9 — extension needed for `no-loss-and-duplicate-same-line` and `auditor-offset-safety` |
| `at-least-once-no-loss` | MINOR CONCERN | F11 — 60s `close_timeout` requires long partition; set to 5–10s |
| `registry-survives-crash` | MINOR CONCERN | F12 — volume rename semantics need confirmation |
| `adaptive-sampler-no-aliasing` | MINOR CONCERN | F14 — CPU-pause is weak trigger; Antithesis value is input diversity |
| Category C (node-termination) | MINOR CONCERN | F13 — node-termination fault must be explicitly requested from tenant |
| All others (22 slugs) | PASS | See Passes table |

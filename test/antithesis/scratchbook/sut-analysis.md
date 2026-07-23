---
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
external_references:
  - path: https://datadoghq.atlassian.net/wiki/spaces/~602449d8f3d296006864db68/pages/6495210537/Property+testing+Logs+Agent+Adaptive+Sampling
    why: Repo owner's proposal to property-test adaptive sampling; defines sampling correctness + read-liveness properties.
  - path: https://datadoghq.atlassian.net/wiki/spaces/~712020006700eab4c247639d448c47103cd8b7/pages/6273073381/Logs+to+Disk+-+Payload+Journaling+Design
    why: Documents backpressure drop points, auditor offset tracking, and duplicate-send-on-restart behavior; the "catch-up problem".
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/4437541188/RFC+Logs+Agent+Distributed+Senders
    why: Per-pipeline concurrency model and shared-sender proposal.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/6782419701/RFC-+Logs+Agent+Backpressure+Status
    why: Pipeline stages, backpressure propagation, rotation-related log loss, component utilization telemetry.
  - path: https://datadoghq.atlassian.net/wiki/spaces/AL/pages/6505529378/Adaptive+Sampling+Architecture+and+Overview
    why: Adaptive sampling credit/token design underlying the sampling properties.
---

# SUT Analysis — Datadog Agent Logs Pipeline

## Scope

System under test: the **logs agent** (`pkg/logs/` and `comp/logs/`). The logs
agent collects log lines from many sources (files, container runtimes, journald,
network sockets, integrations) and delivers them to the Datadog intake over HTTP
or TCP. It runs as a set of components inside the larger agent process — it is a
**single-process, multi-goroutine** system, not a distributed one. That shapes
which Antithesis faults matter: node-internal concurrency, restart/recovery,
clock, CPU throttling, and network faults to the *intake* and to *container/socket
dependencies* are the levers; cross-replica consensus faults are not applicable.

Provenance tags below: `[arch]` architecture/deps, `[state]` state/concurrency,
`[guar]` guarantees, `[fail]` failure-modes/assumptions, `[hist]` bug-history/tests/product,
`[wild]` wildcard. Full per-focus detail lives in `sut-discovery/*.md`.

---

## 1. Architecture and Data Flow

The pipeline has two halves joined by Go channels.

**Ingress half (what to log):** Autodiscovery → schedulers (`pkg/logs/schedulers`,
chiefly `ad`) → two stores (`sources.LogSources`, `service.Services`) → launchers
(`pkg/logs/launchers/*`: file, container/docker, kubernetes, listener, journald,
channel, integration, windowsevent) → **tailers** (`pkg/logs/tailers/*`). Each
launcher filters sources (usually by `source.Config.Type`) and produces tailers.

**Egress half (the pipeline):** `Tailer → Decoder → Processor → Strategy → Sender
→ Worker → DestinationSender → Destination(s) (HTTP|TCP) → Auditor`. There are a
small fixed number of pipelines (`logs_config.pipelines`, default sized for CPU
parallelism). Each tailer/input is **pinned round-robin to one pipeline** at
creation; each pipeline preserves message order. `[arch]`

**Per-message path:** a file tailer runs two goroutines — `readForever`
(reads bytes, feeds decoder) and `forwardMessages` (drains decoder output into the
pipeline). `Tailer.outputChan` *is* `Pipeline.InputChan`. The processor applies
redaction/filtering/metadata then renders+encodes; the strategy batches and
compresses; the sender fans payloads to destination workers; destinations POST to
intake; on 2xx the payload is handed to the auditor for offset tracking. `[arch][guar]`

**Channels and buffer sizes (all configurable):** `[arch][state]`
- `Pipeline.InputChan` / processor input: **100** (`logs_config.message_channel_size`)
- strategy input: **100**
- sender per-worker queue: **1** (HTTP mode)
- `DestinationSender.input`: **10** (`logs_config.payload_channel_size`)
- auditor input: **100**
- pipeline-failover router channel: **5** per pipeline (failover mode only)

**Protocols / serialization:** HTTP destination (default) and TCP destination.
Payloads are serialized and compressed (gzip/zstd) in the strategy before send.
The compression boundary is a known resource hazard (see §7, zstd C-heap leak). `[arch][hist]`

---

## 2. State Management and Persistence

**Durable state (survives crash):** `[state]`
- **Auditor registry** — `registry.json` under `logs_config.run_path`. JSON v2:
  `{Version, Registry: identifier → {Offset, TailingMode, Fingerprint, IngestionTimestamp}}`.
  Flushed by a **1-second ticker**, plus on `Stop()` and explicit `Flush()`.
  Expired-entry cleanup (TTL ≈ 23h) every 300s.
  - **Atomic writer** (default): `CreateTemp` + write + `Chmod` + `Close` + `os.Rename`
    (crash-safe on a single filesystem).
  - **Non-atomic writer** (ECS Fargate): `os.Create` (truncates) + write — a crash
    between truncate and write leaves a zero-length/partial registry. `[state][fail]`

**Ephemeral state (lost on crash):** all channel-buffered messages/payloads across
the pipeline (≈100+100+1+10+100 slots per pipeline plus batch buffers); the
`LogSources` and `Services` stores (re-populated from config/AD on restart);
pipeline objects. `[state]`

**Recovery:** on restart `recoverRegistry()` reads the file; if missing/corrupt →
empty map → every tailer seeks to its configured `TailingMode` (default:
end-of-file). The un-flushed offset window (≈1 second of just-sent traffic) is
replayed → **at-least-once delivery with duplicates after restart.** `[state][guar]`

---

## 3. Concurrency Model

The logs agent is heavily goroutine-based with explicit `Start()`/`Stop()`
lifecycles per component. Steady-state goroutine inventory for a 4-pipeline HTTP
agent: 1 auditor + 4 processor + 4 batch-strategy + sender worker(s) + drain sink
+ retryReader(s) + http.Destination.run + up to ~40 concurrent HTTP send
goroutines (dynamic worker pool) + 2 per file tailer + launcher/scan goroutines. `[state]`

**Top concurrency hazards (each a candidate property):** `[state][wild][hist]`
- **H1 — `Services.AddService`/`RemoveService` deadlock** (`pkg/logs/service/services.go:43,63`):
  both hold `s.mu` while doing blocking sends on unbuffered subscriber channels.
  If a subscriber stops consuming, the whole system can deadlock. `LogSources`
  avoids this (releases lock before sending).
- **H2 — Auditor `Flush()` race** (`auditor.go:~314`): `len(inputChan)` is snapshotted
  then that many items drained; payloads arriving after the snapshot are not
  flushed → extra replays on restart. Backed by real fix history (`62bf5e55c25`,
  `a5141ba432c` ARM64 flake).
- **H3 — Non-atomic registry corruption on Fargate** (`registry_writer.go:56-73`):
  truncate-then-write under crash → empty registry → silent full re-tail.
- **H4 — `forwardWithFailover` hang / send-on-closed-channel** (`provider.go:~361`,
  failover mode): forwarder blocked on `pipeline.InputChan <- msg` doesn't observe
  `routerChannels` close; `forwarderWaitGroup.Wait()` hangs, and a later
  `processor.Stop()` close of `InputChan` risks a send-on-closed panic.
- **H5 — Auditor `Stop()` drops in-flight payloads** (`auditor.go:171-183`): run
  loop exits on channel-close without draining buffered payloads → offset not
  advanced → re-read (duplicates) on restart.
- **H6 — `sender.Stop()` ordering / no `sync.Once`** (`fail`): `close(q)` correctness
  depends on an implicit stop-ordering invariant; a double `Stop()` would panic on
  close-of-closed-channel.

The project's own review guidance names **send-on-closed-channel during shutdown**
and **goroutine leaks** as the most common bug classes — and the bug history (§6)
confirms this is where fixes cluster.

---

## 4. Safety Guarantees (candidate invariants)

From docs, comments, and code `[guar][wild]`:

- **S1 — Per-source ordering within a pipeline.** README:156. Enforced by FIFO
  channels + single-goroutine stages. **BROKEN** when `pipeline_failover.enabled=true`
  (`provider.go:372` `trySendToPipeline` spreads consecutive messages across
  pipelines) and **across a rotation boundary** (new tailer round-robins to a
  *different* pipeline than the draining old tailer, `launcher.go:663`). Neither
  caveat is documented.
- **S2 — Auditor offset only advances past successfully-sent data.** Enforced by
  `output <- payload` only on HTTP 2xx. Caveats: permanent 4xx **also advances**
  the offset (silent drop); 1-second persistence window means a crash replays.
- **S3 — Logs are not modified after processing/redaction.** Redaction → render →
  encode complete synchronously before `outputChan <- msg`. Matches design-doc
  property "logs transmitted as-written."
- **S4 — High-value logs are never dropped by adaptive sampling.** `isImportant`
  bypasses the credit bucket before decrement. **Conditional** on
  `protect_important_logs=true` (off by default).
- **S5 — Unreliable-destination failures don't block the pipeline or advance the
  auditor.** Enforced via `noopSink` + `NonBlockingSend` (silent drop on full buffer).
- **S6 — Adaptive sampling transmits exactly N of a low-value pattern per interval T.**
  Credit-bucket *approximates* this; exact count not guaranteed for new/bursty
  patterns (initial `BurstSize` credits). (See design-doc properties (a)/(b).)
- **S7 — Registry file is durably written.** Holds only with the atomic writer;
  non-atomic (Fargate) path is tear-risky.

## 5. Liveness Guarantees (candidate progress properties)

`[guar]` — most need a fault-quiet recovery window to verify (`eventually_` or
`ANTITHESIS_STOP_FAULTS`).

- **L1 — Every written log line is eventually read.** Tailer polls indefinitely.
  **VIOLATED** under sustained backpressure during file rotation: `closeTimeout`
  (60s) expiry → `stopForward()` discards decoded messages, increments
  `BytesMissed`. This is the headline failure mode. (Design-doc liveness property (c).)
- **L2 — Queued payloads eventually sent once destination recovers.** Indefinite
  retry for retryable errors (5xx/network). **VIOLATED** on permanent 4xx and on
  context cancellation during shutdown.
- **L3 — On restart, unsent on-disk/registry data is replayed (at-least-once).**
  **VIOLATED** if registry missing/corrupt (falls back to default tailing mode).
- **L4 — Backpressure eventually clears.** Sender polls ~100ms; `cancelSendChan`
  signals recovery. **VIOLATED** if shutdown races recovery.
- **L5 — All log lines eventually reach intake.** Holds only with `NoopSampler`
  (default). **VIOLATED by design** when `AdaptiveSampler` active (rate-limited
  drops are permanent — expected, not a bug; matters for property phrasing).
- **L6 — Tailer eventually advances.** No self-stall; downstream stall propagation
  is unbounded in duration but clears when downstream unstalls.

---

## 6. Bug History and Density `[hist]`

Highest-churn / highest-risk areas (regression targets):

- **Auditor / offset registry** — correctness-critical. `62bf5e55c25` (Flush race on
  transport restart), `a5141ba432c` (ARM64 flaky test, same race), `1d1b05d054b`
  (drain queues on shutdown). Root pattern: `LogsSent` is incremented by the sender
  *before* the auditor acks the offset — a concurrent stop opens a stale-offset window.
- **File rotation / file tailer** — highest commit density. `86882e6e718` (leak +
  fingerprinting), `7964b32e5da` (panic on rotated tailer stop), `12295d572f4`
  (integration launcher rotation Linux), `55c63957d9f` (journald drops first entry),
  `60c521b9e7d` (truncation at size boundary).
- **Sender / batch strategy** — shutdown races + leaks. `0d9dfc76f46` (zstd C-heap
  leak — `resetBatch()` orphaning `ZSTD_CCtx`, multi-GiB RSS), `90560d965b0` (batch
  shutdown race), `f7cf97529ac` (destination sender deadlock), `8f16efe6289` (stop
  with pending payloads race).
- **Launcher lifecycle** — shutdown deadlocks. `94d7ccbfc35` (partial-restart
  deadlock on unbuffered LogSources channels), `7041f901670` (TCP Accept block macOS),
  `dae81c1a82e` (nil-guard stream-logs on shutdown).
- **Decoder / multiline** — `046241bfc73` (timestamp lost through auto-multiline),
  `15b1c1c8ae2` (DetectingAggregator TrimSpace breaks anchored rules), `7687b846b2a`
  (sampled_count aliasing on pattern-table resort).

`flakes.yaml` has **zero** current log-pipeline entries; several historical flakes
were real races now fixed. One test is skipped with FIXME: "Multiple Directories -
Out of order input" (`file_provider_test.go:645`, known ordering bug in
`applyReverseLexicographicalOrdering`).

## 7. Failure & Degradation Modes `[fail]`

Retry/backoff: HTTP exponential base=1s, max=120s, factor=2, recover after 2
errors-free; capped ~8 errors. TCP backoff `[2^(n-1), 2^n)`s, hard-cap n=7
(64–128s). HTTP connect timeout 10s; TCP dial 20s (hardcoded). **No `Retry-After`
support** — 429 treated like 5xx.

Drop/degradation paths (several with **no metric** — invisible loss):
1. `NonBlockingSend` drop for secondary reliable destinations on full buffer
   (`sender/worker.go:160-164`) — counter only.
2. Worker 100ms busy-sleep poll when all reliable destinations retrying
   (`worker.go:146`) — blocks backpressure propagation.
3. **Rotation `closeTimeout` loss** (`tailer.go:306-339`) — the headline loss path.
4. **Seek error ignored** (`tailer_nix.go:36` `ret, _ := f.Seek(...)`) — on error,
   re-reads from offset 0 → duplicate storm.
5. Batch encode failure drops the whole batch (`batch.go:94-119`) — `log.Warn` only,
   no metric.
6. Processor render/encode error drops the message (`processor.go:198-214`) —
   `log.Error` only, no metric.
7. Non-atomic registry write on Fargate (above).
8. Atomic `os.Rename` cross-filesystem (EXDEV) failure → registry silently not
   updated (`log.Warn`).
9. TCP `defer cancel()` inside the retry loop (`connection_manager.go:102-103`) —
   context leak during prolonged outage.

## 8. External Dependencies & Integration Points `[arch]`

- **Filesystem / file rotation** — the dominant production input. Rotation,
  deletion, truncation, wildcard globbing. Loss path #3/#4 above.
- **Container runtime (Docker API socket)** — graceful fallback to file tailing;
  mid-session socket loss exits the container tailer goroutine with no auto-restart
  until the next launcher scan.
- **journald** — `setup()` failure flags the source and skips the tailer; no
  reconnect on mid-session failure.
- **Network sockets (TCP/UDP listener)** — per-port/per-connection tailers.
- **Datadog intake (HTTP/TCP)** — retry policy above; 400/401/403/413 permanent
  drop; 403 + secrets backend triggers API-key refresh then retry.
- **Autodiscovery** — feeds the AD scheduler; delays widen the `container_collect_all`
  startup race window.

## 9. Unproven Assumptions (most dangerous under fault injection) `[fail][wild]`

1. **Clock is monotonic non-decreasing** — sampler credit refill, worker-pool EWMA
   latency, and backoff all use `time.Now()`. Clock-jitter faults can drive extra
   sampling drops, freeze EWMA, or distort backoff. **Antithesis clock faults
   directly target this.**
2. **`Seek` always succeeds** (`tailer_nix.go:36`) → duplicate storm on disk error.
3. **Atomic registry rename stays on one filesystem** → EXDEV silently skips update.
4. **`DestinationsContext.Start()` runs before any `Send`** → nil context → panic.
5. **`Sender.Stop()` called exactly once** → close-of-closed panic (no `sync.Once`).
6. **Container rotation yields unique tailer identifiers** (FIXME at `tailer.go:260`)
   → two tailers share registry key `"file:"+path` → offset revert via timestamp race.
7. **60s rotation timeout is enough to drain downstream** → false under backpressure.
8. **`filepath.Glob`/doublestar returns sorted results** (`file_provider.go:362`
   FIXME) → wildcard ordering depends on undocumented behavior.

## 10. Wildcard cross-cuts `[wild]`

- **Pipeline reassignment on rotation** + **failover mode** both silently break the
  per-source ordering guarantee (S1) — and nothing warns the user.
- **Container identifier collision** (FIXME) makes auditor offset revert a *hot*
  path under container churn.
- **Async `AddSource`/`RemoveSource`** in the container launcher (goroutines to
  avoid self-deadlock) creates ordering holes: remove-before-add, or in-flight
  source goroutine after the launcher believes it stopped.
- **`container_collect_all` startup race** (`container.go:241`): containers briefly
  logged under the generic source, then rescheduled under the annotated source —
  fault-injected AD delay widens the wrong-metadata / drop window.
- **Multi-line aggregation × truncation**: stacktraces split or cut at the size
  boundary; auto-multiline can lose timestamps.

---

## 11. Existing Test Coverage vs. Antithesis Value `[hist]`

**Well covered (unit/integration):** parser correctness (+fuzz), adaptive sampler
units, auditor persistence/format-migration/Flush correctness, restart lifecycle
(TCP↔HTTP, concurrent, rollback), HTTP/TCP retry/timeout, batch shutdown, processor
rules. Integration tests use **in-process/mock** senders and localhost intake.

**NOT covered — where Antithesis adds value:**
1. Backpressure → rotation loss end-to-end (no test rotates a file while the tailer
   is blocked on a full channel).
2. Network partition / packet loss / connection reset *mid-retry* (all current
   tests use localhost/in-process servers).
3. `kill -9` / ungraceful shutdown recovery of the auditor registry.
4. journald cursor recovery after ungraceful kill (no-gap/no-duplicate).
5. Concurrent pipeline saturation under real backpressure.
6. Adaptive sampler concurrent stress (the pattern-table resort aliasing bug had no
   concurrent test).

## 12. Product Context — user-visible failures `[hist]`

1. **Silent log loss during file rotation under backpressure** — dominant use case,
   no fault coverage. Highest impact.
2. **Duplicate logs after restart/upgrade** — auditor flush/stale-offset window; no
   intake-side dedup.
3. **Agent OOM from zstd C-heap leak** — invisible to Go GC; needs soak.
4. **Logs silently stop flowing** — sender stuck retrying, tailer blocked, only the
   `logs.dropped` metric hints at it.
5. **Truncated/split multiline events** — corrupts observability data.

---

## Antithesis fault relevance (summary)

| Fault type | Where it bites in the logs agent |
|---|---|
| **Network (partition/latency/drop) to intake** | Sender/destination retry, backpressure propagation, rotation loss, catch-up problem, NonBlockingSend drops |
| **Node termination (kill/restart)** | Auditor registry recovery, at-least-once/duplicate semantics, in-flight loss, Fargate non-atomic write — **requires node-termination fault enabled** |
| **Clock jitter** | Adaptive sampling interval/credit logic, EWMA worker pool, backoff timing — **requires clock fault enabled** |
| **CPU throttling / thread pausing** | Shutdown races (H1–H6), backpressure timing, container-launcher reconciliation races |
| **Custom faults (config toggle)** | Flip `pipeline_failover.enabled`, sampling on/off, atomic-write on/off mid-run |

---

## Assumptions

- The SUT is the logs agent in isolation; the broader agent process (metrics,
  traces, APM) is out of scope except as a host for shared lifecycle.
- "Intake" can be represented by a fakeintake-style mock that speaks the logs HTTP
  (and optionally TCP) protocol and can be faulted as a separate container.
- File tailing is the primary scenario; container/journald/socket are secondary.
- Findings cite file:line as reported by discovery agents at commit 8ff8f30e10b;
  exact line numbers may drift but the structures are stable. Detailed evidence is
  in `sut-discovery/*.md`.

## Open Questions

- Does the Fargate non-atomic registry write path execute in any Linux container
  deployment we'd actually test, or only on real Fargate? Affects whether H3 is
  reachable in the planned topology.
- What is the default value and realistic range of `logs_config.pipelines` in the
  configs we'll test? Determines how much cross-pipeline ordering/duplication
  surface exists.
- Is `pipeline_failover.enabled` ever default-on in any supported config? If never,
  the S1-failover violation is a config-gated property, not a default one.
- For at-least-once: does the intake dedup, and on what key? Determines whether
  "duplicates after restart" is observable as a *correctness* issue or only a
  *cost/volume* issue at the workload/fakeintake layer.
- Adaptive sampling open questions inherited from the owner's design doc: is
  low/high-value status re-evaluated per interval; are patterns matched in config
  order; is there a warmup period before sampling engages?

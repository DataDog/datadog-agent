# Focus 2 & 3: State Management, Persistence, and Concurrency
<!-- Commit 8ff8f30e10b — analyzed 2026-05-28 -->

---

## Focus 2 — State Management and Persistence

### 2.1 Auditor Registry (on-disk offset tracking)

**Location:** `comp/logs/auditor/impl/auditor.go`, `comp/logs/auditor/impl/registry_writer.go`

**Registry format:**
- JSON file `registry.json` at `logs_config.run_path` (default: platform run dir)
- Versioned: current version is **v2**; backward-compatible reading of v0/v1/v2
- Key: string identifier (e.g. `file:<path>`, container identifier, etc.)
- Value: `RegistryEntry{LastUpdated time.Time, Offset string, TailingMode string, IngestionTimestamp int64, Fingerprint types.Fingerprint}`
- On disk struct: `JSONRegistry{Version int, Registry map[string]RegistryEntry}`

**Recovery on startup:**
- `Start()` calls `recoverRegistry()` which reads and unmarshals the JSON file
- If file is missing → empty map, tailers start from the configured TailingMode default (typically end-of-file)
- If file is corrupt → empty map (all progress lost); error logged but not fatal
- Registry re-populated on next flush cycle

**Flush cadence:**
- In-memory `registry map[string]*RegistryEntry` updated synchronously in the auditor's run goroutine whenever a payload arrives on `inputChan`
- Ticker-based flush to disk every **1 second** (`defaultFlushPeriod`, `auditor.go:26`)
- On-demand flush via `Flush()` → `flushRequestChan` (used during transport restart `partialStop`)
- On `Stop()`: `closeChannels()` drains the run goroutine, then `flushRegistry()` is called once more

**Writer modes (config `logs_config.atomic_registry_write`):**
- **Atomic** (default on most platforms): write to temp file, `os.Rename` → crash-safe
- **Non-atomic** (ECS Fargate): write directly to `registry.json` → window of corrupt/truncated file if process dies during write

**Registry TTL:**
- Entries expire after `logs_config.auditor_ttl` (default **23 hours**)
- Cleanup runs every **300 seconds** (`defaultCleanupPeriod`, `auditor.go:27`)
- Active tailers are kept alive even past TTL via `KeepAlive()`/`SetTailed()`

**Persistence boundary:**
- Only the auditor writes to disk; tailers only hold in-memory `decodedOffset` (atomic int64)
- A crash between payload acknowledgment (destination sends on `outputChan → auditor.inputChan`) and the next flush (up to 1 second later) = **lost registry update = replay on restart**
- At-least-once delivery by design; duplicates are expected across restarts

**File offset held in tailer:**
- `Tailer.lastReadOffset` — raw bytes read from file (atomic int64, `tailer.go:50`)
- `Tailer.decodedOffset` — byte offset of last fully decoded message (atomic int64, `tailer.go:55`)
- The auditor receives `msg.Origin.Offset` (= `decodedOffset` as string) via the pipeline payload

---

### 2.2 In-Memory Pipeline Buffers / Queues

**Channel sizes (all configured, defaults from `pkg/config/setup/common_settings.go:1870-1871`):**
- `logs_config.message_channel_size` = **100** — used for:
  - `pipeline.InputChan` (pipeline.go:62, tailer → processor)
  - `processor.inputChan` (same channel as above)
  - `strategyInput` chan (pipeline.go:45, processor → batch strategy)
  - `auditor.inputChan` (auditor.go:166, sender output → auditor)
- `logs_config.payload_channel_size` = **10** — used for:
  - sender worker queue channels (`queues[i]`, sender.go:136): buffered at `workersPerQueue` (1 per queue by default)
  - HTTP destination `inputChan` (`DestinationSender`, destination_sender.go:34): `bufferSize` = payload_channel_size

**Batch strategy in-flight buffer:**
- Each pipeline's `batchStrategy` holds a `batches map[string]*batch` (keyed "main" / "mrf")
- Each `batch` contains a `MessageBuffer` (slice of `*message.MessageMetadata`, capacity = `logs_config.batch_max_size`)
- Encoded payload accumulated in `batch.encodedPayload bytes.Buffer` (in-memory)
- Flushed on: size limit reached, timer expiry (`batchWait`), on-demand `flushChan`
- **All in-flight buffered messages are lost on crash** — registry persists the last ACKed offsets from already-flushed payloads only

**Sources store (`pkg/logs/sources/sources.go`):**
- `LogSources.sources []*LogSource` — in-memory slice, guarded by `mu sync.Mutex`
- Ephemeral: populated by schedulers at startup, lost on restart
- Subscriptions via unbuffered channels with `done` channels for cancellation

**Services store (`pkg/logs/service/services.go`):**
- `Services.services []*Service` — in-memory slice, guarded by `mu sync.Mutex`
- Ephemeral: populated by container discovery at runtime

---

### 2.3 Durable vs. Ephemeral State Summary

| State | Durable | Notes |
|---|---|---|
| File read offsets (registry) | Yes (JSON, 1s flush) | Non-atomic mode: crash window for corruption |
| File fingerprints (registry) | Yes | Used for rotation detection |
| Batch strategy in-flight messages | No | Lost on crash; replayed from last registry offset |
| processor.inputChan messages | No | Lost on crash |
| strategyInput channel messages | No | Lost on crash |
| auditor.inputChan messages | No | If not flushed within 1s window, offset not persisted |
| Sources store | No | Re-populated from config/autodiscovery |
| Services store | No | Re-populated from container runtime |
| Pipeline provider (pipelines) | No | Rebuilt on start/restart |

---

## Focus 3 — Concurrency Model

### 3.1 Goroutine Map

Per-component goroutines spawned during operation:

| Component | Goroutine(s) | File:Line | Entry/Exit |
|---|---|---|---|
| `registryAuditor` | `run()` — event loop | `auditor.go:128` | Started in `Start()`; exits when `inputChan` closed |
| `Processor` | `run()` — message processing | `processor.go:126` | Started in `Start()`; exits when `inputChan` closed |
| `batchStrategy` | anonymous — batch accumulation | `batch_strategy.go:103` | Started in `Start()`; exits when `inputChan` closed |
| `streamStrategy` | anonymous — 1:1 forwarding | `stream_strategy.go:36` | Started in `Start()`; exits when `inputChan` closed |
| `worker` | `run()` — payload dispatch | `worker.go:91` | Started in `start()`; exits when `done` received |
| `noopDestinationsSink` | anonymous — draining goroutine | `worker.go:215` | Started per-worker; exits when noop channel closed |
| `DestinationSender.startRetryReader` | anonymous — retry state reader | `destination_sender.go:56` | Started in `NewDestinationSender`; exits when `retryReader` closed |
| `http.Destination` | `run()` — payload loop | `http/destination.go:224` | Started in `Start()`; exits when input channel closed |
| `http.workerPool.performWork` | N concurrent goroutines per destination | `http/worker_pool.go:122` | Spawned per payload send; exits after send completes |
| `file.Tailer.forwardMessages` | 1 per tailer | `tailer.go:280` | Started in `Start()`; exits when decoder.OutputChan closed |
| `file.Tailer.readForever` | 1 per tailer | `tailer.go:282` | Started in `Start()`; exits on stop signal |
| `file.Tailer.StopAfterFileRotation` | 1 anonymous timer goroutine | `tailer.go:311` | Per log rotation; exits after `closeTimeout` |
| `file.Launcher.run()` | 1 | `launcher.go:136` | Started in `Start()`; exits on stop signal |
| `file.Launcher.run() (FilesToTail)` | 1 per scan tick | `launcher.go:176` | Sends to `filesChan` (buffered 1) then exits |
| `provider.forwardWithFailover` | 1 per pipeline (failover mode) | `provider.go:274` | Exits when routerChannels closed |
| `LogSources` subscription goroutines | 1 per subscribe call for existing sources | `sources.go:131,167,199` | Exits after draining existing sources |
| `Services` subscription goroutines | 1 per subscribe call for existing services | `services.go:78,111` | Exits after draining existing services |
| `logAgent.httpRetryLoop` | 1 (when TCP fallback) | `agent_restart.go:206` | Exits on context cancel or success |

**Total concurrency at steady state (default config, 4 pipelines, HTTP):**
- 1 auditor run goroutine
- 4 processor goroutines
- 4 batchStrategy goroutines
- 1 sender worker (DefaultWorkersPerQueue=1, DefaultQueuesCount=1) → 1 worker.run goroutine
- 1 noopSink goroutine (per worker)
- 1 DestinationSender retry goroutine per destination
- 1 http.Destination.run goroutine per destination
- 1..N http.workerPool.performWork goroutines (dynamic, min=4 pipelines, max=40 by default)
- 2 goroutines per file tailer (readForever + forwardMessages)
- 1 file launcher goroutine + 1 per scan
- Several sources/services subscription goroutines for replay of existing sources

---

### 3.2 Channel Flow (HTTP mode, default config)

```
Tailer.outputChan (buffered 100) → pipeline.InputChan (same chan)
  → processor.inputChan (same chan)
  → processor.outputChan = strategyInput (buffered 100)
  → batchStrategy.inputChan (same chan)
  → batchStrategy.outputChan = sender.In() = queue[0] (buffered 1)
  → worker.inputChan (same chan)
  → DestinationSender.input (buffered 10)
  → http.Destination.inputChan (same chan)
  → [HTTP POST to intake]
  → http.Destination.output (= auditor.inputChan, buffered 100)
  → auditor.inputChan (same chan)
  → [registry updated, flushed to disk]
```

---

### 3.3 Stop/Shutdown Sequence

**Full stop** (`agent.go:stop()`):
1. `schedulers.Stop()` — stops AD subscription feed
2. `launchers.Stop()` — stops all launchers, which stops all tailers (parallel)
3. `pipelineProvider.Stop()` — stops all pipelines in parallel, then sender
   - Each pipeline: `processor.Stop()` (close inputChan, wait for run), then `strategy.Stop()` (close inputChan, wait for goroutine)
   - Then `sender.Stop()`: `worker.stop()` for each (send `done`, wait `finished`), then `close(queue)`
4. `auditor.Stop()` — `closeChannels()` (close inputChan, wait for run goroutine), then `flushRegistry()`
5. `destinationsCtx.Stop()` — cancels context for in-flight HTTP requests
6. `diagnosticMessageReceiver.Stop()`

Grace period: `logs_config.stop_grace_period` (default **30 seconds**). After timeout, `destinationsCtx.Stop()` is called forcibly, then waits 5 more seconds before dumping goroutines.

**Partial stop** (transport restart, `agent_restart.go:partialStop()`):
1. Stop launchers, pipelineProvider, destinationsCtx
2. Auditor **not stopped** — `Flush()` called instead

---

### 3.4 Shared Mutable State and Locking

| Shared State | Guard | Location |
|---|---|---|
| `registryAuditor.registry map[string]*RegistryEntry` | `registryMutex sync.Mutex` | auditor.go:63 |
| `registryAuditor.tailedSources map[string]bool` | `registryMutex` (same) | auditor.go:59 |
| `registryAuditor.inputChan`, `flushRequestChan`, `done` | `chansMutex sync.Mutex` | auditor.go:55 |
| `LogSources.sources`, subscription slices | `mu sync.Mutex` | sources.go:33 |
| `Services.services`, subscription slices | `mu sync.Mutex` | services.go:17 |
| `provider.currentPipelineIndex`, `currentRouterIndex` | `atomic.Uint32` (lock-free) | provider.go:67-68 |
| `Sender.idx` | `atomic.Uint32` (lock-free) | sender.go:62 |
| `Tailer.lastReadOffset`, `decodedOffset`, `isFinished`, etc. | `atomic.Int64/Bool` | tailer.go:50-99 |
| `http.Destination.nbErrors`, `shouldRetry`, `lastRetryError` | `retryLock sync.Mutex` | http/destination.go:93 |
| `http.workerPool` fields | `sync.Mutex` embedded | http/worker_pool.go:57 |
| `logAgent.restartMutex` guards full restart sequence | `sync.Mutex` | agent.go:136 |
| `logAgent.httpRetryMutex` guards retry loop cancel/create | `sync.Mutex` | agent.go:141 |

---

### 3.5 Concurrency Hazards

#### H1: Services blocking send while holding lock — **Deadlock Risk**

**File:** `pkg/logs/service/services.go:34-44` and `49-64`

`AddService()` and `RemoveService()` hold `s.mu` while doing **blocking sends** on unbuffered channels to all subscribers:
```go
s.mu.Lock()
defer s.mu.Unlock()
// ...
for _, ch := range append(added, s.allAdded...) {
    ch <- service  // blocks if consumer is slow or stopped
}
```

If a launcher's goroutine is stopped (e.g., during shutdown or restart) while another goroutine tries to call `AddService()`, the sender blocks indefinitely while holding the mutex. Any further caller that needs the mutex (e.g., `RemoveService`, `GetAddedServicesForType`) deadlocks.

Note: `LogSources` does **not** have this problem — it releases the lock before sending (sources.go:58-74).

**Severity:** High — can deadlock the entire logs pipeline during concurrent start/stop with AD discovery.

---

#### H2: Auditor Flush Race — Incomplete Flush on Transport Restart

**File:** `comp/logs/auditor/impl/auditor.go:313-331`

`Flush()` sends a request to the run goroutine which then does:
```go
n := len(a.inputChan)
for i := 0; i < n; i++ {
    select {
    case payload := <-a.inputChan:
        // process
    default:
    }
}
if err := a.flushRegistry(); err != nil { ... }
```

The `len(a.inputChan)` is a snapshot. New payloads arriving between the snapshot and the drain are **not flushed**. During transport restart (`partialStop()`), the pipeline is stopped before `Flush()` is called, so no new payloads should arrive — but the ordering relies on correct sequential stopping. If the pipeline stop races with auditor flush, offsets from the last second of operation may not be persisted.

**Severity:** Medium — causes extra duplicates on restart (within the at-least-once contract, but worth noting).

---

#### H3: Non-Atomic Registry Write on ECS Fargate — Crash Corruption Window

**File:** `comp/logs/auditor/impl/registry_writer.go:56-73`

On ECS Fargate (`logs_config.atomic_registry_write=false`), the registry is written directly without `fsync` or rename:
```go
f, err := os.Create(registryPath)  // truncates existing file
// ...
f.Write(data)   // crash here = zero-length or partial file
```

A crash between `os.Create` (which truncates) and the write completing leaves `registry.json` empty or corrupt. On restart, `unmarshalRegistry` fails and returns an empty map, causing all tailers to start from their configured mode (usually end-of-file), **silently losing offsets**.

**Severity:** High on ECS Fargate — complete registry loss on agent crash.

---

#### H4: Send-on-Closed-Channel in worker.Stop() / sender.Stop() ordering

**File:** `comp/logs-library/sender/sender.go:180-187`

```go
func (s *Sender) Stop() {
    for _, s := range s.workers {
        s.stop()   // signals worker to stop, waits for it to exit
    }
    for _, q := range s.queues {
        close(q)   // closes the queue channel AFTER workers have stopped
    }
}
```

The `close(q)` fires after `worker.stop()` has confirmed the goroutine exited, so workers are no longer reading from `q`. However, the **strategy goroutine** (`batchStrategy` or `streamStrategy`) may still be sending to `q` = `sender.In()` concurrently if `strategy.Stop()` hasn't returned yet.

The pipeline's `Stop()` is `processor.Stop()` then `strategy.Stop()` (pipeline.go:83-85), and strategy.Stop() happens **before** `sender.Stop()` (provider.go:295-298). Strategy.Stop() blocks until its goroutine exits (`<-s.stopChan`). So by the time `sender.Stop()` runs, the strategy goroutine has already exited and cannot send to `q`. The sequence is safe **as currently implemented**, but the ordering dependency is subtle and fragile.

**Severity:** Low (currently safe) — but a future reordering of Stop() calls could introduce a send-on-closed panic.

---

#### H5: forwardWithFailover Goroutine Leak (failover mode)

**File:** `comp/logs-library/pipeline/provider.go:353-366`

When failover is enabled, `forwardWithFailover` goroutines are started (one per pipeline). `Stop()` closes `routerChannels` which causes the `for msg := range p.routerChannels[routerIndex]` loop to exit normally. The `forwarderWaitGroup.Wait()` in `Stop()` blocks until all these goroutines exit. This looks correct.

However, if `forwardWithFailover` is **blocked** on `p.pipelines[primaryPipelineIndex].InputChan <- msg` (the backpressure path at provider.go:361) when `Stop()` tries to close `routerChannels` and then close `pipeline.InputChan` (via `processor.Stop()`), the goroutine is blocked on a channel that is about to be closed. Closing `pipeline.InputChan` while the goroutine is blocked sending to it would cause a **send-on-closed-channel panic**.

The Stop() sequence in provider.go:283-299:
1. Close all routerChannels
2. `forwarderWaitGroup.Wait()` — but if a forwarder is blocked at line 361 on `pipeline.InputChan <- msg`, it never reads from routerChannel to notice the close
3. This is a **potential goroutine hang/deadlock** on shutdown when a pipeline's InputChan is full

**Severity:** High — hang or potential panic on shutdown when backpressure is active.

---

#### H6: Decoder InputChan / OutputChan are Unbuffered

**File:** `pkg/logs/internal/decoder/decoder.go:125-126`

Both `inputChan` and `outputChan` in the decoder are unbuffered (`make(chan *message.Message)`). The tailer's `readForever` goroutine writes to `decoder.InputChan()`. The `forwardMessages` goroutine reads from `decoder.OutputChan()`. If the tailer's `outputChan` (= `pipeline.InputChan`, buffered 100) fills up, `forwardMessages` blocks on the send, which backs up through the decoder's unbuffered channel to `readForever`, which then blocks reading from the file. This is the intended backpressure mechanism but means that **the entire chain stalls** when the downstream pipeline is full.

**Severity:** Low (by design) — documented as a known backpressure behavior.

---

#### H7: TOCTOU in Processor.Flush()

**File:** `comp/logs-library/processor/processor.go:138-153`

`Flush()` acquires `p.mu` then drains `p.inputChan` in a loop checking `len(p.inputChan) == 0`. Meanwhile `run()` acquires `p.mu` AFTER processing each message (lines 168-170: `p.mu.Lock(); p.mu.Unlock()`). The `sync.Mutex` here is deliberately locked and immediately unlocked — this is a "blocking point" to let `Flush()` serialize with the run loop. However, `Flush()` reads from `p.inputChan` concurrently with `run()` — the `p.mu` only prevents them from both processing at the same time, but the channel read itself is not protected. Since `p.inputChan` is a Go channel, concurrent reads are safe, but it means `run()` could drain messages that `Flush()` was trying to drain.

**Severity:** Low — the semantic is "best-effort flush" for serverless.

---

### 3.6 Open Questions / Assumptions

1. **File tailer to pipeline channel assignment**: is it truly 1 tailer : 1 pipeline? The code assigns tailers to pipelines via `provider.NextPipelineChan()` (round-robin), so multiple tailers can share a pipeline. Within a pipeline, order is preserved, but interleaving between tailers on the same pipeline is possible.

2. **Services deadlock in practice**: How frequently is `AddService` called during launcher lifecycle? Are there known production incidents from this pattern?

3. **Failover mode is off by default** (`logs_config.pipeline_failover.enabled=false`). The H5 hazard only applies when enabled.

4. **Non-atomic write on Fargate**: Is there a watchdog that detects and corrects a corrupt registry on restart? The current code simply returns an empty map silently.

5. **Auditor `inputChan` buffer size = `message_channel_size` = 100**: With 4 pipelines each accumulating payloads, does this buffer ever fill during high-throughput sends? When full, the destination's `output chan *message.Payload` (also 100) backs up, blocking `sendAndRetry()`.

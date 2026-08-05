# Logs Pipeline: Failure Modes and Unproven Assumptions
## Focus 8 (Failure/Degradation) + Focus 11 (Unproven Assumptions)
## Commit: 8ff8f30e10b

---

## Areas Examined

| Area | Files |
|------|-------|
| HTTP Destination (send/retry/backoff) | `comp/logs-library/client/http/destination.go` |
| TCP Destination | `comp/logs-library/client/tcp/destination.go`, `connection_manager.go` |
| Worker / DestinationSender | `comp/logs-library/sender/worker.go`, `destination_sender.go` |
| Batch strategy | `comp/logs-library/sender/batch.go`, `batch_strategy.go` |
| Sender / pipeline provider | `comp/logs-library/sender/sender.go`, `comp/logs-library/pipeline/provider.go` |
| File tailer (read/rotation) | `pkg/logs/tailers/file/tailer.go`, `tailer_nix.go`, `rotate_nix.go` |
| Decoder / Framer | `pkg/logs/internal/decoder/decoder.go`, `framer/framer.go` |
| Single-line / multi-line handler | `decoder/single_line_handler.go`, `multiline_handler.go` |
| Adaptive sampler | `decoder/preprocessor/sampler.go` |
| Processor | `comp/logs-library/processor/processor.go` |
| Auditor (registry) | `comp/logs/auditor/impl/auditor.go`, `registry_writer.go` |
| Agent restart/transport switch | `comp/logs/agent/agentimpl/agent_restart.go` |
| Backoff policy | `pkg/util/backoff/backoff.go` |

---

## F8 — Failure and Degradation Paths

### F8.1 — HTTP Retry Loop and Backoff

**File:** `comp/logs-library/client/http/destination.go:263–320`

`sendAndRetry()` loops unconditionally when `shouldRetry=true` and the error is `*client.RetryableError`. Backoff is applied before each retry via `waitForBackoff()` which blocks on the DestinationsContext.Context() deadline. If the context is cancelled (agent stopping), the deadline fires and `ctx.Err()` propagates back as `context.Canceled`, which exits the loop (`line 298`).

**Backoff parameters (defaults):**
- Factor: 2.0 (`DefaultLogsSenderBackoffFactor`, `pkg/config/setup/config.go:124`)
- Base: 1.0 s (`DefaultLogsSenderBackoffBase`)
- Max: 120.0 s (`DefaultLogsSenderBackoffMax`)
- Recovery interval: 2 (`DefaultLogsSenderBackoffRecoveryInterval`)
- Formula: `Base * 2^n`, randomized `[backoff/Factor, min(Max, backoff)]`
- Max errors before cap: `floor(log2(Max/Base)) + 1 = 8` errors → max 120 s sleep
- On each success, `nbErrors` decremented by `RecoveryInterval=2` (not to zero immediately)

Retryable: network errors, 5xx (status > 400 that are not 400, 401, 403, 413).
Non-retryable (permanent drop): 400, 401, 403 (without secrets refresh), 413.

**Key:** `shouldRetry=false` in serverless mode; any error causes immediate payload drop without retry.

### F8.2 — 429 Rate-Limit Handling

**File:** `comp/logs-library/client/http/destination.go:414–420`

HTTP 429 ("Too Many Requests") falls through to the `else if resp.StatusCode > http.StatusBadRequest` branch at line 420 and is returned as `client.NewRetryableError(errServer)`. This means 429 is treated as a retryable 5xx-class error with standard exponential backoff. **There is no Retry-After header parsing.** Under a sustained 429 storm, the agent will spin with its own backoff timer (up to 120 s) without respecting any server-provided backoff hint.

### F8.3 — NonBlockingSend Drop Path

**Files:** `comp/logs-library/sender/worker.go:150–165`, `comp/logs-library/sender/destination_sender.go:134–141`

In `worker.run()`, once the primary reliable destination's blocking `Send()` succeeds, the code iterates over all reliable destinations again (line 150) and for any that did not succeed in the first iteration (`!lastSendSucceeded`), it tries `NonBlockingSend()`. If that also fails (buffer full), it increments:
- `tlmPayloadsDropped` ("true", destinationIndex)
- `tlmMessagesDropped`

This is the **silent drop point** for secondary reliable destinations when backpressure is extreme. There is no alert, no circuit breaker, no blocking—the payload is gone.

Similarly for unreliable destinations (`worker.go:168–183`): `NonBlockingSend()` only, drop metric incremented if full.

**NonBlockingSend itself** (`destination_sender.go:134–141`): pure non-blocking select with a default that returns `false`; caller decides whether to log.

### F8.4 — Worker Poll Loop Busy-Sleep

**File:** `comp/logs-library/sender/worker.go:141–148`

```go
if !sent {
    time.Sleep(100 * time.Millisecond)
}
```

When all reliable destinations are in retry mode (blocked), the worker polls at 100 ms intervals trying to send. This is a busy-sleep. Under Antithesis time compression/dilation this may behave differently than production. If time advances faster, the 100 ms loop can consume significant scheduling. This loop **also blocks the worker goroutine** from processing new payloads from `inputChan`, causing backpressure to propagate backward to the batch strategy.

### F8.5 — File Tailer: Rotation Close Timeout

**File:** `pkg/logs/tailers/file/tailer.go:306–339`

`StopAfterFileRotation()` spawns a goroutine that sleeps exactly `closeTimeout` (default: 60 s, `common_settings.go:1874`). After timeout, calls `t.stopForward()` (cancels the forwardContext) and sends to `t.stop`. If the output channel (pipeline InputChan) is blocked (full), `forwardMessages()` will discard messages on `forwardContext.Done()` at line 433.

**Loss condition:** If the downstream pipeline is full AND rotation happens AND the old file is deleted/renamed before all bytes are drained within `closeTimeout`, those bytes are lost. The code now measures and logs remaining bytes as `BytesMissed` (line 325), but does not block deletion or extend the timeout.

### F8.6 — File Tailer: Seek Error Silently Ignored

**File:** `pkg/logs/tailers/file/tailer_nix.go:36`

```go
ret, _ := f.Seek(offset, whence)
```

The Seek error is discarded with `_`. If `Seek` fails (e.g., file was replaced between open and seek), `ret` will be 0 (the return value when err != nil in Go's `io.Seeker`). The tailer then begins reading from offset 0 instead of the requested offset, causing full re-read of the file → **duplicate logs on restart after crash**. The error is never surfaced.

### F8.7 — File Tailer: Rotation Detection Race (os.SameFile)

**File:** `pkg/logs/tailers/file/rotate_nix.go:24–57`

`DidRotate()` opens the file at `fullpath`, calls `os.SameFile(fi1, fi2)` comparing inode numbers of the newly-opened file and the currently-held file descriptor. Between the open and stat, the file can be rotated again (double rotation), producing a false negative (`recreated=false`) while the inode has actually changed. Also `fi2` (`t.osFile.Stat()`) returns `(true, nil)` on success, but if the fd is already closed, it returns `(true, err)` and the code returns `true, nil` (line 38–39), treating a Stat error as "rotated"—which is correct but the error is swallowed.

### F8.8 — Tailer Identifier FIXME: Duplicate Registry Keys During Container Rotation

**File:** `pkg/logs/tailers/file/tailer.go:260–267`

```
// FIXME(remy): during container rotation, this Identifier() method could return
// the same value for different tailers.
```

If two tailers briefly share the same identifier, the auditor will update the registry offset with whichever payload arrives last (determined by `IngestionTimestamp` comparison at `auditor.go:387`). If the newer container's tailer has a lower ingestion timestamp (clock skew, time jump, etc.), the **dead container's offset could overwrite the live container's offset**, causing the live container's tailer to restart from the wrong position after the next agent restart.

### F8.9 — Decoder: Oversized Line Truncation

**File:** `pkg/logs/internal/framer/framer.go:195–203`

When no frame terminator is found and the buffer exceeds `contentLenLimit` (default `MaxMessageSizeBytes`), the framer forcibly emits a chunk of exactly `contentLenLimit` bytes with `isTruncated=true`. The `SingleLineHandler` (`single_line_handler.go:55–73`) then prepends/appends `TruncatedFlag`. **The raw bytes still advance `lastReadOffset` correctly** (via `rawDataLen`), so offset tracking is not corrupted by truncation.

**Loss condition:** If `multiline_handler` accumulates a very large multiline message that exceeds `lineLimit`, the buffer is truncated mid-flush (`multiline_handler.go:sendBuffer`). The truncated portion is dropped.

### F8.10 — Batch Encoder Failures Silently Drop Payloads

**File:** `comp/logs-library/sender/batch.go:94–119`

If `b.addMessage(m)` returns an encoding error (`serializer.Serialize` fails), the batch calls `resetBatch()` and returns, **dropping the entire batch in progress as well as the triggering message**. The only signal is `log.Warn("Encoding failed - dropping payload")`. No telemetry counter is incremented for encoding failures (unlike `tlmDroppedTooLarge` for oversized messages).

Similarly in `flushBuffer` (line 146) and `sendMessages` (line 163): compressor `Close()` failure drops the whole batch silently with only a warn log.

### F8.11 — Auditor: flushRegistry Error Suppression

**File:** `comp/logs/auditor/impl/auditor.go:300–311`

In the `run()` loop, `flushRegistry()` errors are logged via `a.log.Warn(err)` but not retried. If the disk write fails (e.g., full filesystem, permission change, SIGKILL between write and fsync), the registry is lost. On restart, the auditor calls `recoverRegistry()` which returns an empty map on error (line 344), causing all tailers to restart from configured defaults (Beginning or End), potentially causing **duplicate logs or log gaps** depending on tailing mode.

**Non-atomic write (default on ECS Fargate):** `nonAtomicRegistryWriter.WriteRegistry` (`registry_writer.go:56–73`) does `os.Create` then writes—if the agent crashes between create and write, the registry file is empty/truncated → **data loss**.

**Atomic write (default elsewhere):** `atomicRegistryWriter` writes to a temp file then `os.Rename`, which is atomic on Linux if src and dst are on the same filesystem. If the run directory and temp dir are on different filesystems, `os.Rename` fails → `log.Warn` only, registry NOT updated.

### F8.12 — TCP Connection Manager: defer cancel() Inside for Loop

**File:** `comp/logs-library/client/tcp/connection_manager.go:102–103`

```go
for {
    ...
    dctx, cancel := context.WithTimeout(ctx, connectionTimeout)
    defer cancel()   // deferred until function returns, not until next iteration
    ...
}
```

`defer` executes when the function returns, not when the loop iterates. In a multi-retry scenario, `cancel` is accumulated on the defer stack, meaning **all context cancellations are deferred until `NewConnection` returns**. The memory for intermediate contexts grows with each retry. In a prolonged connection failure (many retries), this is a slow memory leak. This is not a correctness bug but is a resource-management concern.

### F8.13 — TCP handleServerClose: Goroutine Leak on Connection Replacement

**File:** `comp/logs-library/client/tcp/connection_manager.go:125`, `165–182`

Every successful `NewConnection` spawns `go cm.handleServerClose(conn)`. This goroutine blocks reading 1 byte from the connection. When `CloseConnection(conn)` is called to replace the connection (rotation), the goroutine receives `net.ErrClosed` and exits normally. However, if `CloseConnection` is never called (e.g., the caller just reassigns `d.conn = nil` without explicitly closing the old conn), the handleServerClose goroutine will leak indefinitely.

In `tcp/destination.go:sendAndRetry`, on write error the code calls `d.connManager.CloseConnection(d.conn)` before setting `d.conn = nil`—so this path is clean. However on periodic reset (`line 141–143`), it also calls CloseConnection. Appears safe in normal flow.

### F8.14 — HTTP Destination: DestinationsContext nil Context Panic

**File:** `comp/logs-library/client/destinations_context.go:44–49`, `http/destination.go:328`

`DestinationsContext.Context()` returns `dc.context`, which is `nil` until `Start()` is called. In `unconditionalSend`, `ctx := d.destinationsContext.Context()` is called at line 328. If a destination is used before its context is started (e.g., in tests, or if the component lifecycle is misordered), `req.WithContext(nil)` at line 380 will panic with "nil context". No nil check is performed.

### F8.15 — Sender Stop: No Guard Against Double-Close of queues

**File:** `comp/logs-library/sender/sender.go:180–188`

`Sender.Stop()` closes all queue channels unconditionally. If `Stop()` is called twice (or Stop is called concurrently with a goroutine still writing to the queue), this panics with "close of closed channel". The `Sender` has no `sync.Once` guard. The file launcher uses `stopOnce` for its own Stop, but if the provider's Stop is called from multiple paths (shutdown + restart signal), a race is possible.

### F8.16 — Adaptive Sampler: Clock Monotonicity Assumption

**File:** `pkg/logs/internal/decoder/preprocessor/sampler.go:134`, `199–212`

```go
now: time.Now,
```

The adaptive sampler uses `time.Now()` (wall clock) for credit refill:
```go
elapsed := now.Sub(e.lastSeen).Seconds()
e.credits += elapsed * s.config.RateLimit
```

If the system clock jumps backward (NTP correction, VM migration, Antithesis fault injection), `elapsed` can be negative, causing `e.credits` to decrease rather than increase. If credits drop below 0.0, the next message is dropped even if the pattern had credits. No floor guard: `e.credits += elapsed * RateLimit` with no check for `elapsed < 0`. Similarly the worker pool in `http/worker_pool.go:170` uses `time.Now()` for EWMA sample windows without monotonic clock protection.

### F8.17 — Multi-line Handler: Flush Timer Race

**File:** `pkg/logs/internal/decoder/multiline_handler.go:95–98`

```go
if !h.flushTimer.Stop() {
    <-h.flushTimer.C
}
```

If `Stop()` returns false (timer already fired), the code drains `h.flushTimer.C`. But the `decoder.run()` goroutine at `decoder.go:443` also reads from `lineHandler.flushChan()` (which returns `h.flushTimer.C`). If the decoder goroutine drains the channel concurrently, the `<-h.flushTimer.C` in `process()` will block forever. This is a potential deadlock under concurrent execution. In practice, these both run in the same goroutine (`decoder.run()`), so it's not a race today—but the design is fragile.

### F8.18 — Processor: Render/Encode Error Silently Drops Message

**File:** `comp/logs-library/processor/processor.go:198–215`

```go
rendered, err := msg.Render()
if err != nil {
    log.Error("can't render the msg", err)
    return   // message dropped, no metric, no retry
}
...
if err := p.encoder.Encode(msg, ...); err != nil {
    log.Error("unable to encode msg ", err)
    return   // message dropped, no metric, no retry
}
```

Both render and encode failures silently drop the message with only a log.Error. No counter is incremented (`LogsDropped` or similar). The message disappears from the pipeline with no observable telemetry beyond the error log.

### F8.19 — Batch Strategy: Stuck on Closed Input Chan Close Race

**File:** `comp/logs-library/sender/batch_strategy.go:96–99`, `pipeline.go:82–86`

`batchStrategy.Stop()` calls `close(s.inputChan)`. The `Pipeline.Stop()` sequence is: `processor.Stop()` then `strategy.Stop()`. If the processor is still writing to its output channel (which is the strategy's inputChan) when Stop is called, the processor's `outputChan <- msg` at `processor.go:219` can panic with "send on closed channel" if the inputChan is closed before the processor drains. This depends on whether `processor.Stop()` (`close(p.inputChan)`) fully drains before `strategy.Stop()` is reached. Looking at the code: `processor.Stop()` closes the inputChan and blocks on `<-p.done`. The processor `run()` loop continues until inputChan is drained, sending to `outputChan`. **If the strategy's goroutine exits before the processor is done writing to outputChan**, the processor will block on `outputChan <- msg` at processor.go:219 indefinitely (deadlock on stop).

---

## F11 — Unproven Assumptions

### A1 — Atomic Registry Write Assumes Same Filesystem

**File:** `comp/logs/auditor/impl/registry_writer.go:45`

```go
return os.Rename(tmpName, registryPath)
```

`os.CreateTemp(registryDirPath, ...)` creates the temp file in `registryDirPath`. `os.Rename` is atomic only within a single filesystem. If `registryDirPath` resolves to a different mount point than the final `registryPath` (container volume mounts, bind mounts, tmpfs), `os.Rename` fails with EXDEV. This returns an error → `flushRegistry` logs a Warn and the registry is NOT updated. **The code assumes same-filesystem temp write, but the assumption is never verified.**

### A2 — Stop() Called Only Once (Sender/Worker)

**File:** `comp/logs-library/sender/sender.go:180–188`

`Sender.Stop()` closes queues without any once-guard. During agent restart (`agent_restart.go:partialStop`), the pipeline provider is stopped via a stopper. If the restart fails and `rollbackToPreviousTransport` is called, a new provider is created. But if there are lingering goroutines from the old provider that also receive a stop signal (via `DestinationsContext.Stop()`), they may attempt to write to already-closed channels. **No protection against double-stop**.

### A3 — DestinationsContext.Start() Called Before Any Send

**File:** `comp/logs-library/client/destinations_context.go:27–32`, `http/destination.go:328`

Code assumes `Start()` is called before `Context()` is ever used. If the lifecycle ordering is violated (e.g., during a fast restart where the old pipeline's goroutines outlive the context.Start() call), `Context()` returns nil and `req.WithContext(nil)` panics. No nil guard.

### A4 — File Rotation Is Atomic at the OS Level

**File:** `pkg/logs/tailers/file/rotate_nix.go:45`

```go
recreated := !os.SameFile(fi1, fi2)
```

`DidRotate()` assumes that between opening `fullpath` and statting `t.osFile`, the file identity is stable. On NFS or distributed filesystems, inode numbers can be reused rapidly. If a file is deleted and a new file with the same inode is created within the scan window (default 1 s), `os.SameFile` returns true and no rotation is detected → the tailer continues reading from the old (now gone) fd and makes no progress while the new file goes untailed until the next scan.

### A5 — Clock Is Monotonically Non-Decreasing

**Files:** `comp/logs-library/client/http/worker_pool.go:98,170`, `pkg/logs/internal/decoder/preprocessor/sampler.go:134`

Both the HTTP worker pool (EWMA latency calculations) and the adaptive sampler (credit refill) use `time.Now()` without monotonic clock protection. Under Antithesis fault injection (simulated clock skew, VM pause/resume), a backward time jump causes:
- EWMA window size to become negative → `time.Since() < 0` → no EWMA update, worker count frozen
- AdaptiveSampler credits to decrease on refill, causing extra drops

### A6 — Seek Returns the Requested Offset

**File:** `pkg/logs/tailers/file/tailer_nix.go:36`

```go
ret, _ := f.Seek(offset, whence)
```

Assumes `Seek` always succeeds and returns exactly `offset`. On error, Go's `os.File.Seek` returns `0, err`. The tailer proceeds with `lastReadOffset=0` and `decodedOffset=0`, causing the tailer to re-read from the beginning of the file, generating duplicate messages with no log.Warn or error propagation.

### A7 — Container Rotation Produces Unique Tailer Identifiers

**File:** `pkg/logs/tailers/file/tailer.go:260–267`

Explicitly documented as a FIXME. Two tailers for old and new containers can produce the same registry identifier during rotation overlap. The auditor's `updateRegistry` (line 374) uses `IngestionTimestamp` as a tie-breaker, but if clocks are not monotonic or if both tailers send with similar timestamps, the wrong offset can win. **This is a known bug with no fix.**

### A8 — 100 ms Busy-Sleep Is Acceptable Under High Load

**File:** `comp/logs-library/sender/worker.go:146`

```go
time.Sleep(100 * time.Millisecond)
```

This sleep is the only rate-limiting mechanism when all reliable destinations are in retry mode. The comment says "Throttle the poll loop." Under Antithesis, if time is compressed/sped up, this loop may spin much more frequently than intended, consuming CPU and potentially masking the actual backpressure mechanism.

### A9 — TCP Dial Timeout Is Fixed at 20 Seconds

**File:** `comp/logs-library/client/tcp/connection_manager.go:30`

```go
connectionTimeout = 20 * time.Second
```

Hard-coded constant with no configuration override. During a network partition, each dial attempt waits up to 20 s before retrying. With backoff capped at `[64, 128)` s after 7 retries, recovery from a network partition can take several minutes, during which the pipeline is blocked.

### A10 — HTTP Timeout Is Sufficient for Large Payloads Under Load

**File:** `comp/logs-library/client/http/destination.go:454`, `common_settings.go:1842`

Default `logs_config.http_timeout = 10 s`. If the intake is slow but connected, a large compressed payload might timeout before receiving a response. The timeout error is a `client.NewRetryableError`, so the same payload is retried with the full backoff. Under sustained high latency (latency fault injection), this creates a feedback loop: timeout → retry → backoff → queue fills → tailer blocks.

### A11 — Rotation Close Timeout (60s) Is Long Enough to Drain Backlog

**File:** `pkg/logs/tailers/file/tailer.go:312`

```go
time.Sleep(t.closeTimeout)
```

After rotation, the tailer gets 60 s (by default) to drain unread bytes. If the downstream pipeline is saturated (e.g., due to backoff), the tailer is blocked writing to the output channel and cannot read more bytes from the file. After 60 s, `stopForward()` is called, which causes `forwardMessages()` to discard in-flight messages on `forwardContext.Done()`. **Any messages in the decoder's output channel but not yet forwarded are lost**, even if the file had all been read.

### A12 — Log Encoding Always Succeeds

**File:** `comp/logs-library/processor/processor.go:197–214`

`msg.Render()` and `encoder.Encode()` errors drop the message silently. The encoder (JSON/proto) can fail for malformed UTF-8 input or internal JSON marshaling errors. These are treated as unrecoverable without any telemetry counter. **The assumption is that encoding never fails in production, so no retry or dead-letter mechanism exists.**

---

## Parameter Summary

| Parameter | Value | Configurable |
|-----------|-------|-------------|
| HTTP backoff base | 1.0 s | Yes (`sender_backoff_base`) |
| HTTP backoff max | 120.0 s | Yes (`sender_backoff_max`) |
| HTTP backoff factor | 2.0 | Yes (`sender_backoff_factor`) |
| HTTP recovery interval | 2 errors/success | Yes (`sender_backoff_recovery_interval`) |
| HTTP timeout | 10 s | Yes (`logs_config.http_timeout`) |
| TCP connect timeout | 20 s | No (hardcoded) |
| TCP backoff | `[2^(n-1), 2^n)` s, max n=7 (64-128 s) | No (hardcoded) |
| Rotation close timeout | 60 s | Yes (`logs_config.close_timeout`) |
| File scan period | 1 s | Yes (`logs_config.file_scan_period`) |
| Tailer sleep | 1 s | Code constant (`DefaultSleepDuration`) |
| Batch wait | 5 s | Yes (`logs_config.batch_wait`) |
| Batch max size | 1000 messages | Yes |
| Batch max content | 5 MB | Yes |
| Message channel size | 100 | Yes |
| Payload channel size | 10 | Yes |
| Worker pool min workers | = numberOfPipelines | Via config |
| Worker pool max workers | numberOfPipelines * 10 | Via config |
| Worker pool target latency | 150 ms (hardcoded) | No |
| Aggregation timeout (multiline) | 1000 ms | Yes |
| Registry flush period | 1 s | No (hardcoded) |
| Registry cleanup period | 300 s | No (hardcoded) |
| Busy-sleep (retry poll) | 100 ms | No (hardcoded) |

---

## Open Questions

1. **Is there a Retry-After header respect for 429?** No — 429 uses the same exponential backoff as 5xx. Under rate-limit storms, the backoff grows slower than a Retry-After hint might suggest.

2. **What happens if DestinationsContext.Stop() is called twice?** The Stop() checks `dc.cancel != nil` before calling it (line 37–38), so double-stop is safe. But Context() returns the cancelled context, not nil. Subsequent sends get `context.Canceled` immediately and exit the retry loop.

3. **Under what conditions does the processor stop goroutine block indefinitely?** If `strategy.inputChan` (= processor's outputChan) is full when the processor tries to send the last message after the strategy's goroutine has exited. Possible during a restart race.

4. **Is the adaptive sampler's credit floor-capped at zero?** No. Credits can go slightly negative if a negative elapsed time is computed. The first negative-credit case still allows the next message if `credits >= 1.0`, which won't be true, so the next message is dropped. Not a catastrophic bug, but unexpected behavior under clock jumps.

5. **Is there any protection against message ordering violations across pipelines?** No. Each input is pinned to one pipeline (round-robin assignment at launch time). The registry auditor uses ingestion timestamp to avoid older offsets overwriting newer ones, but ordering across sources is not guaranteed.

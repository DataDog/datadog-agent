# Logs Pipeline Guarantees — Focus 4 (Safety) & Focus 5 (Liveness)

Analyst: Focus 4+5  
Commit: 8ff8f30e10b  
Areas examined: `pkg/logs/`, `comp/logs/`, `comp/logs-library/`

---

## Safety Guarantees (invariants that must never be violated)

### S1 — Per-source message ordering is preserved within a pipeline

**Claim:** Messages from a single source are always delivered to the intake in the order they were read.

**Source:** `pkg/logs/README.md:156` — *"Each pipeline handles messages in-order, so assigning each input to a single pipeline ensures that input's messages will be delivered to the intake in-order."*

**How enforced:**
- Each tailer writes to exactly one pipeline `InputChan` (a buffered Go channel); channels preserve FIFO order.
- Processor, Strategy, and Sender each operate as a single sequential goroutine per pipeline, maintaining order.
- The pipeline-assignment round-robin (`provider.currentPipelineIndex`) runs at launch time, so a tailer's assignment is stable for its lifetime.

**Violation condition (pipeline failover mode):**
When `logs_config.pipeline_failover.enabled = true`, `forwardWithFailover` may redirect a message from the primary pipeline to any other pipeline via `trySendToPipeline` (`comp/logs-library/pipeline/provider.go:372`). If two consecutive messages from the same source land on different pipelines, relative ordering across those pipelines is not preserved. **This mode breaks the per-source ordering guarantee.**

**Status:** Upheld in default (non-failover) mode; BROKEN in experimental failover mode.

---

### S2 — Auditor offset only advances past messages whose payload was successfully received by at least one reliable destination

**Claim:** The auditor registry entry for a source is only updated after the destination's `output` channel is written, which only happens after a successful HTTP 2xx response (or TCP accept).

**Source:** `comp/logs-library/client/http/destination.go:318` — `output <- payload` is reached only after `unconditionalSend` returns `nil`.

**How enforced (audit pathway):**
1. `destination.sendAndRetry` loops until success or permanent error.
2. On success, `output <- payload` sends the payload to the auditor channel (which is `sink.Channel()` = `auditor.inputChan`).
3. On permanent client error (HTTP 400/401/403/413), the payload goes to `output` too (advancing the registry), but logs are counted as dropped (`metrics.DestinationLogsDropped`). **This means the auditor offset CAN advance past permanently-rejected payloads — they are silently discarded at the intake.** The offset advance still happens.
4. Auditor `updateRegistry` (`comp/logs/auditor/impl/auditor.go:374`) uses `IngestionTimestamp` to prevent a dual-shipping race from advancing an older offset past a newer one: it only updates if `ingestionTimestamp >= existing.IngestionTimestamp`.

**Violation condition:**
- Permanent client errors (HTTP 4xx) cause the offset to advance while the logs are actually dropped. This is by design (undeliverable payloads) but violates a strict "offset only advances past successfully-received data" interpretation.
- Auditor registry is written to disk on a 1-second ticker (not synchronized to send). A crash between a successful send and the next `flushRegistry` can cause the offset to not be persisted, leading to replay on restart.

**Status:** Partially upheld. Offset does not regress (monotone by ingestion-timestamp guard). Offset is not guaranteed to be disk-durable at the moment of send (up to ~1 second gap).

---

### S3 — Logs are not modified after processing/redaction completes

**Claim:** A message's content is finalized (redacted and encoded) before it is placed on the output channel to the Strategy.

**Source:** `comp/logs-library/processor/processor.go:192-218` — `applyRedactingRules` → `msg.Render()` → `msg.SetRendered(rendered)` → `encoder.Encode(msg, ...)` — all happen synchronously before `p.outputChan <- msg`.

**How enforced:** All processing (filtering, masking, encoding, tag attachment) happens in `processMessage` which is a synchronous function called in the processor's run loop. The message is not written to the output channel until all mutations are complete.

**Violation conditions:**
- `MaskSequences` mutates `msg.GetContent()` in place via `rule.Regex.ReplaceAll`. If two goroutines shared a message pointer this would be a race, but each message flows through a single pipeline goroutine — safe.
- Tag slices from `origin.GetTags()` have documented aliasing risks (`pkg/logs/message/origin.go:40`: *"The returned slice must not be modified by the caller."*). Tag content is set before sending and not mutated downstream.

**Status:** Upheld. Processing-before-send ordering is structurally enforced by channel sequencing.

---

### S4 — High-value (important) logs are never dropped by adaptive sampling

**Claim:** When `ProtectImportantLogs = true`, logs containing severity tokens (FATAL, ERROR, PANIC, ALERT, SEVERE, CRITICAL, EMERGENCY, WARN, EXCEPTION, CRASH, FAILURE, DEADLOCK, TIMEOUT) bypass credit-based rate limiting and are always forwarded.

**Source:** `pkg/logs/internal/decoder/preprocessor/sampler.go:194-197` — `if s.config.ProtectImportantLogs && isImportant(tokens) { return msg }` before credit check.

**How enforced:** The `isImportant` check runs before credit decrement. An important log returns immediately without entering the pattern table.

**Violation conditions:**
- `ProtectImportantLogs` defaults to `false` (`logs_config.experimental_adaptive_sampling.protect_important_logs`). The guarantee only holds when the flag is explicitly enabled.
- The token-based detection is structural (token type match), not regex. A log line containing "ERROR" as part of a word (e.g. "ERRORCODE") may or may not tokenize as the `Error` token depending on `Tokenizer` behavior — not verified here.
- When `ProtectImportantLogs = false`, important logs ARE rate-limited like any other (confirmed by `sampler_test.go:369`).

**Status:** Upheld only when the config flag is enabled.

---

### S5 — Unreliable destination failures do not block the pipeline or update the auditor

**Claim:** Unreliable destinations use non-blocking sends; a failed send is dropped silently; the auditor is not updated.

**Source:** `comp/logs-library/sender/worker.go:29-34` — *"Unreliable destinations will only send logs when at least one reliable destination is also sending logs. However they do not update the auditor or block the pipeline if they fail."*

**How enforced:** Unreliable `DestinationSender`s are wired to `noopSink` (not the auditor output channel). `NonBlockingSend` drops the payload if the buffer is full with a counter increment (`tlmPayloadsDropped`).

**Status:** Upheld.

---

### S6 — Adaptive sampling preserves exact count for low-value patterns (doc claim)

**Claim:** For a low-value pattern occurring >N times per interval T, exactly N are transmitted (from the design-doc property).

**How enforced:** Credit-based token bucket: each pattern gets `RateLimit` credits/second, decrementing by 1 per forwarded message. When credits < 1.0, the message is dropped.

**Violation conditions:**
- Credit refill is time-based (`elapsed * RateLimit`), not interval-based. Over an interval T, the count transmitted is approximately `RateLimit * T + BurstSize`, not exactly N.
- New patterns receive `BurstSize - 1` initial credits, so the first `BurstSize` messages of a new pattern are always forwarded regardless of rate.
- The "exactly N" claim is therefore an approximation; the actual behavior is credit-bucket rate limiting.

**Status:** The claim is approximately correct at steady state; not exactly correct for new or bursty patterns.

---

### S7 — The auditor registry file is durably written (atomic write option)

**Claim:** When `logs_config.atomic_registry_write = true`, the registry file is written atomically via a temp file + rename, preventing a torn write from corrupting the registry.

**Source:** `comp/logs/auditor/impl/registry_writer.go:23-45` — `os.CreateTemp` + `os.Rename`.

**How enforced:** The atomic writer uses POSIX rename semantics (atomic on Linux). The non-atomic writer (`NewNonAtomicRegistryWriter`) writes directly and is the default (`atomicRegistryWrite` defaults to value of config).

**Violation conditions:**
- If `atomic_registry_write = false` (non-atomic writer), a crash mid-write produces a corrupted registry file. On recovery, `recoverRegistry` returns an empty registry (`make(map[string]*RegistryEntry)`) and the agent restarts from default offsets, potentially re-reading all data.
- On crash *between* a successful send and the next 1-second flush tick, offsets that were in-memory are lost. The registry on disk lags in-memory state by up to 1 second.

**Status:** Upheld for atomic mode. Non-atomic mode has a tear-risk.

---

## Liveness Guarantees (progress that must eventually happen)

### L1 — Every written log line is eventually read by the tailer (no reader starvation)

**Claim:** The file tailer's `readForever` loop polls continuously until stopped; new data is eventually consumed.

**Source:** `pkg/logs/tailers/file/tailer.go:342-373` — `readForever` loops calling `t.read()` and sleeping `sleepDuration` when `n == 0`.

**How enforced:** The tailer sleeps `sleepDuration` (configurable, default varies) when no data is present, then retries. On file data arrival, the next poll reads it.

**Violation conditions / recovery window needed:**
- If the downstream pipeline channel (`outputChan`) is full (backpressure), `forwardMessages` blocks at `t.outputChan <- msg`. The `readForever` goroutine continues reading raw bytes but the decoder's `InputChan` can fill, eventually blocking reads at `t.decoder.InputChan() <- msg`. This creates a back-pressure chain that can halt reads.
- **File rotation risk:** If backpressure persists past `closeTimeout` (default: configurable seconds), `StopAfterFileRotation` calls `stopForward()`, which cancels `forwardContext`. Messages decoded after that point are silently dropped (`case <-t.forwardContext.Done():`). Unread bytes are logged as lost (`BytesMissed` metric). **This is the documented central failure mode.**
- **Recovery window needed:** This property cannot be verified during fault injection; a quiet window is needed after faults clear to confirm the tailer is making progress again.

**Status:** Upheld during normal operation. Violated under sustained backpressure during file rotation.

---

### L2 — Queued payloads are eventually sent once the reliable destination recovers

**Claim:** The HTTP destination retries indefinitely (with exponential backoff) on retryable errors; once the destination recovers, all buffered payloads are eventually flushed.

**Source:** `comp/logs-library/client/http/destination.go:263-321` — `sendAndRetry` loops forever on `RetryableError` (network errors, 5xx responses), sleeping `backoffDuration` between attempts.

**How enforced:** `shouldRetry = true` for the main endpoint. The loop has no finite retry limit. Backoff is bounded by `endpoint.BackoffMax`.

**Violation conditions / recovery window needed:**
- The `DestinationsContext` context cancels when `ctx.Stop()` is called. On context cancellation (`ctx.Err() == context.Canceled`), `sendAndRetry` returns without sending, and the payload goes to `output` regardless (auditor is notified, offset advances). This can cause data loss on shutdown if in-flight retries are cancelled.
- Permanent client errors (HTTP 400/401/403/413) break out of the retry loop; those payloads are permanently dropped.
- **Recovery window needed:** After a network fault is healed, the backoff may delay the first successful retry by up to `BackoffMax` seconds. Verification requires waiting for at least one `BackoffMax` interval after fault recovery.

**Status:** Upheld for recoverable errors during steady operation. Violated for permanent errors and during context cancellation.

---

### L3 — On restart, unsent on-disk offsets are replayed (at-least-once delivery)

**Claim:** After restart, the auditor reads the persisted registry and tailers resume from the last committed offset, replaying any data that was not successfully sent.

**Source:** `comp/logs/auditor/impl/auditor.go:127` — `a.registry = a.recoverRegistry()` called in `Start()`.
`pkg/logs/tailers/file/tailer.go:271` — `Start(offset int64, whence int)` uses the registry offset.

**How enforced:** The file launcher queries `auditor.GetOffset(identifier)` before starting each tailer. The tailer `setup()` calls `f.Seek(offset, whence)` to position at the recovered offset.

**Violation conditions / recovery window needed:**
- If the registry file is missing or corrupted, `recoverRegistry` returns an empty map and tailers start from the configured default (`TailingMode`: `beginning` or `end`). If `TailingMode = end`, all data written while the agent was down is lost.
- If the registry was flushed at a stale offset (crash between send and flush), the tailer re-reads and re-sends data already received by the intake → duplicate sends. This is the documented "at-least-once, not exactly-once" semantic.
- **Recovery window needed:** After restart, tailers may take up to `sleepDuration` to detect new data. No quiet window is needed for the property itself, but verification must allow sufficient time for replay to complete.

**Status:** Upheld with at-least-once semantics. Exactly-once is not guaranteed.

---

### L4 — Backpressure eventually clears (the sender unblocks)

**Claim:** Once the reliable destination accepts payloads again, the blocked sender goroutine unblocks and the pipeline resumes draining.

**Source:** `comp/logs-library/sender/worker.go:120-148` — the inner `for !sent` loop polls `destSender.Send(payload)` every 100ms when all destinations are blocked.

**How enforced:** `DestinationSender.Send` uses a cancelable select: if the destination transitions out of retry state, `cancelSendChan` is signaled, unblocking the sender. When the destination recovers, `updateRetryState(nil, ...)` sends `false` to `isRetrying`, which `startRetryReader` relays to `cancelSendChan`.

**Violation conditions / recovery window needed:**
- If the pipeline is blocked for longer than `logs_config.stop_grace_period`, the agent shutdown sequence calls `destinationsCtx.Stop()`, cancelling all contexts and forcing the pipeline to drain without sending. Payloads in-flight at that point are lost (or replayed on next start if the registry was not yet advanced).
- **Recovery window needed:** After a destination fault heals, the backoff timer (`BackoffMax`) must expire before the first retry attempt. Property verification requires a quiet window of at least `BackoffMax` after fault recovery.

**Status:** Upheld during normal fault-recovery cycles. Violated if shutdown races with recovery.

---

### L5 — Adaptive sampler eventually transmits all instances of a low-frequency pattern (liveness across intervals)

**Claim:** For all intervals, the agent reads all written log lines (the design-doc liveness claim c).

**How enforced:** The adaptive sampler's `NoopSampler` (default) passes all messages through. The `AdaptiveSampler` drops messages when credits are exhausted.

**Violation conditions:**
- When `AdaptiveSampler` is active and a pattern exceeds the configured rate limit, lines are permanently dropped (returned as `nil` by `Process`). There is no re-injection or buffering — dropped lines are gone.
- "All written log lines are read" is satisfied at the tailer level (L1), but not at the intake level when adaptive sampling is active.

**Status:** Upheld only when using `NoopSampler` (default). Violated by design when `AdaptiveSampler` is active and rate limits are exceeded.

---

### L6 — The tailer eventually advances (does not stall indefinitely)

**Claim:** A file tailer, once started, makes measurable progress reading bytes within a bounded time.

**How enforced:** `readForever` calls `t.read()` in a tight loop with `t.wait()` (sleep) on empty reads. There is no mechanism to permanently stall the read loop itself.

**Violation conditions / recovery window needed:**
- If `t.decoder.InputChan()` is full (decoder not consuming), `t.read()` blocks at `t.decoder.InputChan() <- msg`. This is bounded only by the decoder draining.
- If the decoder output channel is full (processor not consuming), the decoder blocks. The upstream chain stalls.
- **Recovery window needed:** After injecting a pipeline stall and then clearing it, the tailer's `bytesRead` counter should resume incrementing within a bounded time. Verification requires a quiet window after fault clearance.

**Status:** Upheld absent downstream stalls. Stall propagation is deterministic but unbounded in duration.

---

## Assumptions and Open Questions

1. **Fingerprinting and rotation:** `Tailer.Identifier()` has a documented FIXME (`tailer.go:263`) noting it can return the same value for different tailers during container rotation. This could cause auditor registry collisions, silently advancing one tailer's offset with another's data.

2. **Pipeline failover and per-source ordering:** The `pipeline_failover.enabled` feature (experimental) breaks the S1 ordering guarantee. The codebase does not document this explicitly as a known trade-off.

3. **Auditor offset monotonicity guard:** The `IngestionTimestamp` guard in `updateRegistry` (`auditor.go:387`) prevents a dual-shipping race from regressing offsets. However, if two payloads from the same source arrive out of ingestion-timestamp order for reasons other than dual-shipping (e.g., pipeline failover), the guard would silently discard the out-of-order update.

4. **Non-atomic registry default:** The codebase has both `AtomicRegistryWriter` and `NonAtomicRegistryWriter`. The actual default depends on `logs_config.atomic_registry_write`; documentation on which is the production default is not visible in this analysis.

5. **Windows file loss window:** `windowsOpenFileTimeout` is the Windows analog of `closeTimeout`. A stalled pipeline causes rotated file loss on Windows after this timeout — same failure mode as Linux.

6. **Serverless flush:** `Flush(ctx)` in `Pipeline` sends `flushChan <- struct{}{}` and then calls `processor.Flush(ctx)` with a context. If the context expires, the flush is incomplete — in-flight messages in the processor are not sent.

7. **Adaptive sampler is experimental:** `logs_config.experimental_adaptive_sampling` — the "experimental" prefix suggests the feature is not production-stable. The `ProtectImportantLogs` guarantee (S4) and liveness claim (L5) are contingent on this experimental feature being active.

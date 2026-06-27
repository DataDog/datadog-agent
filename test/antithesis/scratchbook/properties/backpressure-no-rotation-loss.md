---
slug: backpressure-no-rotation-loss
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# backpressure-no-rotation-loss — File Rotation Under Backpressure Does Not Cause Permanent Log Loss

## What Led to This Property

This is the #1 user-visible failure mode in the logs agent (product context item
#1 in the SUT analysis). When a log file rotates while the pipeline is
backpressured (intake unavailable, channel full), the old tailer has a 60-second
window (`close_timeout`) to drain its decoded messages. If it can't drain in
time, `BytesMissed` is incremented and those lines are permanently lost. No
existing test exercises this end-to-end.

## Code Paths Involved

**Rotation detection** — `pkg/logs/launchers/file/launcher.go:663`: when a file
scan detects rotation, the launcher calls `tailer.StopAfterFileRotation()` on
the old tailer and starts a new tailer.

**`StopAfterFileRotation()`** — `pkg/logs/tailers/file/tailer.go:306-338`:
- Stores `bytesReadAtRotationTime = t.bytesRead.Get()`.
- Starts a goroutine that sleeps for `closeTimeout` (default 60s).
- After sleep: calls `t.stopForward()` (cancels `forwardContext`).
- Sends to `t.stop` (signals `readForever()` to stop).

**`forwardMessages()`** — `pkg/logs/tailers/file/tailer.go` (lines not shown in
detail, but the structure is):
- Reads from `t.decoder.OutputChan()`.
- Forwards to `t.outputChan` using the forwardContext.
- When `forwardContext` is cancelled, the select on the context fires → the
  goroutine exits without forwarding remaining messages in the decoder output.

**The loss window:**
- Decoder has decoded messages buffered in its output channel.
- `forwardMessages` is blocked because `t.outputChan` is full (backpressure from
  downstream).
- `closeTimeout` fires → `stopForward()` → `forwardMessages` exits.
- Buffered decoder messages are never forwarded → `BytesMissed` is incremented.

**Key metric:**
- `metrics.BytesMissed.Add(remainingBytes)` at `tailer.go:325` — the amount
  of data that was in the old file but not read before `closeTimeout`.
- `metrics.TlmBytesMissed.Add(float64(remainingBytes))` — telemetry counter.

## Failure Scenario

1. Intake is unreachable (network partition injected by Antithesis).
2. Agent retries; backpressure fills all pipeline channels.
3. File rotation event: old file is renamed (logrotate), new file is created.
4. Launcher detects rotation; calls `StopAfterFileRotation()` on old tailer.
5. 60 seconds pass. Backpressure persists (partition not cleared).
6. `stopForward()` fires. 10KB of log data in the old file's decoder output is
   discarded. `BytesMissed` increments.
7. Partition clears. Agent delivers what it has. 10KB is permanently gone.

The loss is silent in the sense that there's no error returned, no retry, and
no user-visible alert beyond the `BytesMissed` metric (which users rarely
monitor).

## Why It Matters

This is the headline production complaint for the logs agent. The design docs
acknowledge it as the "catch-up problem." Any fix that reduces `BytesMissed`
under this scenario is a meaningful improvement; Antithesis can verify whether
proposed fixes actually eliminate loss.

## Workload Instrumentation

- Workload writes N log lines to a file, then rotates it (rename + create new).
- Simultaneously, fakeintake is configured to reject connections (partition).
- After the partition clears:
  - Assert: all N sequence numbers are received.
  - Assert: `BytesMissed` metric is 0 (or, if > 0, record the count and fail).
- SUT-side: a `Sometimes` assertion confirming `BytesMissed` increments during
  the test (proving the failure condition was reached) — currently **missing**.
- Also: a `Reachable` assertion at the `BytesMissed.Add()` call site to ensure
  Antithesis explores the rotation-under-backpressure code path.

## Open Questions

- Is 60s (`close_timeout`) the correct default for the test topology? A shorter
  timeout (e.g., 5s) makes the loss path reachable faster and reduces test
  duration. But it also changes what "acceptable" loss looks like. Yes:
  `DD_LOGS_CONFIG_CLOSE_TIMEOUT` (`setup/common_settings.go`) overrides the
  default; setting it to 5s in the test container is recommended.
- Is there a configuration knob to pause the `closeTimeout` countdown while the
  pipeline is backpressured? If not, this is a design gap — the user cannot
  prevent loss by increasing `close_timeout` if the backpressure duration exceeds
  any finite timeout.
- Is the "bytes remaining" calculation accurate when the rotated file has already
  been partially moved (rename then write)? Affects metric accuracy but not
  loss-path existence.
- Does `closeTimeout` apply to container tailers (Docker socket tailing) or only
  to file tailers? If only file tailers, the property scope narrows.
- Should the property test fire only at the rotation boundary, or also at
  deletion-while-backpressured? Deletion path may differ.

### Investigation Log

#### Does `BytesMissed` count unread file bytes (`fileSize - lastOffset`) or decoded-but-unforwarded messages? Is it an expvar (resets on restart)?

- Examined: `pkg/logs/tailers/file/tailer.go:310-337` (`StopAfterFileRotation`), `comp/logs-library/metrics/metrics.go:65-68`.
- Found: `BytesMissed` is declared as `expvar.Int{}` (package-level var). `expvar.Int` is an in-process, in-memory counter — it resets on process restart, not persistent across runs. The metric is calculated as `fileSize - lastOffset` at the moment the `closeTimeout` goroutine fires (`tailer.go:321-326`). This is the number of bytes in the old file that were **not yet read** by `readForever`, not the number of decoded-but-unforwarded messages. Consequence: if the decoder had already decoded N bytes of the remaining M bytes, those N bytes of decoded-but-stalled messages are included in `BytesMissed` (they were not re-read) but the distinction between "read but stalled" vs "never read" is collapsed. The metric counts **raw bytes not read from the file at the time of timeout**, which is an upper bound on actual log line loss.
- Not found: any persistent storage of `BytesMissed` across restarts; any per-message accounting of decode-but-not-forward loss.
- Conclusion: resolved. `BytesMissed` = `fileSize - lastReadOffset` (unread bytes at timeout time), stored as an in-memory `expvar.Int` that resets on restart. The workload MUST read this metric before the agent restarts to capture it. The metric is an upper bound — it may overcount loss when some bytes were read but stalled in the decoder output channel, though those bytes are also lost due to `stopForward()` cancelling `forwardMessages`. In practice, the full remaining-unread count is the actual loss amount since `stopForward()` prevents both unread-from-file and read-but-stalled-in-decoder messages from being delivered.

## Merged-in evidence (from no-loss-rotation-under-backpressure)

The secondary file provided **exact code snippets** for `StopAfterFileRotation`
and `forwardMessages` and clarified per-message vs. per-rotation accounting:

**`forwardMessages` select** (`tailer.go:430-434`):
```go
select {
case t.outputChan <- msg:
    t.CapacityMonitor.AddIngress(msg)
case <-t.forwardContext.Done():  // cancelled by stopForward()
}
```
When `stopForward()` is called, any message currently being written into a full
`outputChan` is **silently dropped**. No metric is incremented for individual
messages lost this way — only `BytesMissed` which counts remaining raw bytes, not
processed log lines.

**`BytesMissed` accounting** (`metrics.go:65-68`): the metric is incremented
exactly once per rotation event, not per dropped message. This means a single
rotation can drop hundreds of log lines but registers only one metric event.

**Additional assertions (from secondary):**
- Primary `Reachable` assertion directly at the `metrics.BytesMissed.Add(remainingBytes)`
  call site — confirms the loss path is reached during the test run.
- `Sometimes(remainingBytes > 0)` at the same site — confirms scenarios with real
  byte loss (not just a zero-byte rotation event).

## Merged-in evidence (from backpressure-eventually-clears)

The secondary file covered **three additional interactions** between backpressure
and the pipeline lifecycle:

**Shutdown racing recovery:** if the agent is stopped while destinations are
retrying, `destinationsCtx.Stop()` cancels the context, causing all retry loops
to exit immediately (`destination.go:298`). Payloads waiting for retry are
dropped without being written to the auditor — not replayed on restart because
source positions were never committed. This is distinct from the rotation-loss
path but has the same result: permanent silent loss.

**L4 violation at shutdown:** even if the network recovers before shutdown, if
shutdown is initiated during a backoff sleep (up to 120s), `cancelSendChan` does
not interrupt the backoff sleep — only context cancellation does, via
`ctx.Err()`. Payloads queued in the `DestinationSender` buffer are dropped
silently.

**Channel saturation bounds** (from secondary): under full backpressure:
- `Pipeline.InputChan` / processor input: 100
- strategy input: 100
- sender per-worker queue: 1 (HTTP mode)
- `DestinationSender.input`: 10
- auditor input: 100

**Fault-trigger timing note (from secondary):** network partition duration >
60s *with concurrent file rotation* is the compound fault. CPU throttling widens
the drain rate, making the 60s window more easily exceeded. Fault-quiet window
of at least 3 minutes (> max backoff 120s) is required after lifting the
partition before checking `BytesMissed`.

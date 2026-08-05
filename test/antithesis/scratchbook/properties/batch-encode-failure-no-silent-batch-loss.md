# Evidence: batch-encode-failure-no-silent-batch-loss

## Summary

When serialization or compression fails during batch assembly (`batch.go`), the
agent calls `resetBatch()` and drops the entire in-progress batch. This
failure is logged at `log.Warn` level only — no error metric, no
`BytesMissed` increment, no `tlmPayloadsDropped` increment. The auditor does NOT
advance offsets for the dropped messages because they never reached the output
channel. On restart, the messages will be re-read.

However, there are two sub-cases:
1. **`addMessage` encode error** (lines 94-98): `resetBatch()` drops the entire
   *current* batch that was being assembled, plus the current message. The
   messages already in the buffer are also lost (not flushed first).
2. **`Finish`/`Close` encode error** (lines 146-151 in `flushBuffer`): drops
   the batch that was being serialized for send. The messages are already
   consumed from the buffer (`GetMessages()` was called) but their contents
   are lost.

The property: every batch encode failure must be observable (detectable, not
silent), and the messages must either be delivered or their loss recorded with a
durable counter.

## Key code

**`comp/logs-library/sender/batch.go:90-122`** — `processMessage()`:
```go
func (b *batch) processMessage(m *message.Message, outputChan chan *message.Payload) {
    ...
    added, err := b.addMessage(m)
    if err != nil {
        log.Warn("Encoding failed - dropping payload", err)  // log.Warn only
        b.resetBatch()
        return  // message + entire batch dropped; no metric
    }
    ...
}
```

**`comp/logs-library/sender/batch.go:140-161`** — `flushBuffer()`:
```go
func (b *batch) flushBuffer(outputChan chan *message.Payload, reason string) {
    ...
    if err := b.serializer.Finish(b.writeCounter); err != nil {
        log.Warn("Encoding failed - dropping payload", err)  // log.Warn only
        b.resetBatch()
        b.utilization.Stop()
        return  // batch dropped; no metric
    }
    ...
}
```

**`comp/logs-library/sender/batch.go:163-192`** — `sendMessages()`:
```go
func (b *batch) sendMessages(...) {
    defer b.resetBatch()

    err := b.compressor.Close()
    b.compressor = nil
    if err != nil {
        log.Warn("Encoding failed - dropping payload", err)  // log.Warn only
        b.utilization.Stop()
        return  // batch dropped before output <- p; no metric
    }
    ...
    outputChan <- p  // only if Close succeeded
}
```

In all three cases, the dropped batch messages:
- Have their offsets NOT advanced in the auditor (messages never reached output)
- Will be re-read from disk on the next tailer pass (at-least-once recovery)
- Produce only a `log.Warn` entry, not a metric

This is a **silent loss from the metrics perspective** — only log analysis
reveals encode failures.

## Contrast with too-large drop

The `tlmDroppedTooLarge` counter IS incremented for messages too large for an
empty batch. But encode errors (`addMessage err != nil`) do NOT have a dedicated
counter.

## Why it matters

In production, encode errors could be caused by:
- OOM-induced allocation failures in the compressor (Go or C heap)
- Corrupt compressor state (e.g., after an error that left the stream in an
  inconsistent state)
- Bugs in the serializer (rare but possible)

Without a metric, operators have no signal that batches are being dropped.
Under Antithesis memory pressure or CPU throttling, encode errors might surface
that are normally invisible.

## Assertion design

**SUT-side (`Sometimes`):** At each `log.Warn("Encoding failed - dropping payload",
err)` call site (3 sites in batch.go), add `Reachable("batch-encode-error-drop")`
to confirm at least one encode error is observed during the run. (If these paths
are never reached, that is interesting too — `AlwaysOrUnreachable` might be
more appropriate if encode errors truly cannot occur under normal fault injection.)

**SUT-side (`Always`):** After an encode error drops a batch, assert that
`outputChan` was NOT written for those messages (i.e., the batch is truly
discarded). This is structurally guaranteed by the early `return`, but an
explicit assertion documents the contract.

**Workload-side:** Monitor `agent.logs.bytes_sent` for unexpected stalls. A
encode-error storm would show bytes_sent stagnating while bytes_read continues
to grow. Correlation detects silent drops.

## Open Questions

- Is there any path where a single oversized message enters `addMessage()` and
  causes a serialization error (rather than a "not added" return)? If so, the
  encode-error drop and the too-large drop paths overlap.
- Should these encode-error paths have a metric added? This is a product
  improvement question — the property currently relies on log analysis for
  observability. `(needs human input)`

### Investigation Log

#### Under what realistic Antithesis fault conditions can `Serializer.Serialize()` or `compressor.Close()` return an error? Is this path `AlwaysOrUnreachable` in practice?

- Examined: `comp/logs-library/sender/batch.go:90-122` (`processMessage`), `batch.go:124-136` (`addMessage`), `comp/logs-library/sender/serializer.go` (serializer implementation), `pkg/util/compression/impl-zstd-nocgo/zstd_nocgo_strategy.go`, `pkg/util/compression/impl-zstd/` (CGO), `pkg/util/compression/impl-gzip/`, `pkg/util/compression/impl-zlib/`.
- Found: `addMessage` calls `b.serializer.Serialize(m, b.writeCounter)`. The `ArraySerializer` writes JSON via `b.writeCounter` (which wraps the compressor's `Write` method). `Serializer.Serialize()` can only return an error if the underlying `Write` call fails. The write target is the compressor's stream (a `bytes.Buffer` wrapper). `bytes.Buffer.Write` always succeeds (it grows as needed until OOM). The compressor's `Write` (e.g., `*zstd.Encoder.Write`) can fail if the encoder is in an error state (e.g., after a previous error). `compressor.Close()` in `sendMessages()` can return an error if the underlying stream write fails — same `bytes.Buffer` target, same reasoning. Under normal operation (no OOM, no previous encoder error), these paths are unreachable. Under Antithesis memory pressure (OOM-induced allocation failure in `bytes.Buffer.Grow`), these paths could be reached. The practical conclusion: **these paths are `AlwaysOrUnreachable` under normal fault injection (network partitions, process pauses) but could become reachable under synthetic memory pressure.**
- Not found: any production incident reports or test assertions confirming these paths fire in practice; no explicit OOM injection mechanism at the Antithesis level without SUT modification.
- Conclusion: resolved. These encode-error paths are unreachable under standard Antithesis faults (network partitions, CPU throttling, process pauses). They would require OOM-level memory pressure to fire. The property's assertion type should be `AlwaysOrUnreachable` rather than `Sometimes` for the encode-error drop path — if it fires, that's a real fault worth investigating; if it never fires, the test is still valid. The existing `Reachable` and `Sometimes` assertion suggestions in the evidence file should be reconsidered: `AlwaysOrUnreachable` at the error-drop site would assert "whenever this path runs, the batch was not forwarded to output" — which is correct and testable. A workload-side `Reachable` targeting the encode-error path would require OOM injection that is not currently planned.

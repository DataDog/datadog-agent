# Evidence: backpressure-before-drop

## Summary

The claim in the Backpressure Status RFC (external ref #4) is that the agent
backpressures (blocks the tailer) before dropping logs. The actual code reveals
this is **true for data currently in flight through the pipeline**, but
**false at the rotation boundary**: once a file is rotated, the 60-second
`closeTimeout` governs, not backpressure propagation. The property must be
phrased carefully.

## Claim vs. reality

**Claim:** "Loss occurs only at rotation/deletion when source retention is
exhausted."

**Reality:**
1. Under backpressure, the tailer's `forwardMessages` goroutine blocks on
   `t.outputChan <- msg` (bounded, cap=100). This backpressure is real and
   bounded: the tailer stops reading from disk. No bytes are *lost* — they
   remain on disk. ✓
2. At file rotation, `StopAfterFileRotation()` starts a 60-second timer
   *concurrently with backpressure*. If backpressure lasts > 60 seconds after
   rotation, `stopForward()` is called and undrainable messages are abandoned.
   `BytesMissed` is incremented. ✗
3. `NonBlockingSend` drops secondary reliable destination payloads silently when
   their buffer is full (`sender/worker.go:158-164`). The primary reliable
   destination always uses blocking `Send()`. ✓ for primary, but secondary drops
   have a counter.

## Key code

**`comp/logs-library/sender/worker.go:120-148`** — Primary send loop:
```go
sent := false
for !sent {
    for _, destSender := range reliableDestinations {
        if destSender.Send(payload) {  // blocks when destination retrying
            sent = true
            ...
        }
    }
    if !sent {
        time.Sleep(100 * time.Millisecond)  // 100ms busy-sleep
    }
}
```
The 100ms sleep means backpressure propagation is delayed by up to 100ms. During
high-throughput, this creates a window where the pipeline channels fill and
additional messages from the tailer block, which is the intended behavior.

**`comp/logs-library/sender/worker.go:150-165`** — Secondary send (non-blocking):
```go
for i, destSender := range reliableDestinations {
    if !destSender.lastSendSucceeded {
        if !destSender.NonBlockingSend(payload) {  // drop if full
            tlmPayloadsDropped.Inc(...)
            tlmMessagesDropped.Add(...)
        }
    }
}
```
Secondary destinations use `NonBlockingSend` — drops counted by `tlmPayloadsDropped`.

**`comp/logs-library/sender/destination_sender.go:134-141`** — `NonBlockingSend`:
```go
func (d *DestinationSender) NonBlockingSend(payload *message.Payload) bool {
    select {
    case d.input <- payload:
        return true
    default:
    }
    return false
}
```

## Antithesis angle

A network partition lasting > 60s after a file rotation will trigger the
headline loss path. The property "backpressure propagates before loss" is
`Always` true in the steady-state (non-rotation) case, and `AlwaysOrUnreachable`
(or conditional) at the rotation boundary. Antithesis can explore:
- Partition lasting exactly at the closeTimeout boundary (60s ± epsilon)
- Multiple concurrent rotations during backpressure
- Recovery just before the timer fires

## Why it matters

Users understand "logs stop flowing during an outage, then resume" (correct).
Users do NOT expect "logs that existed on disk before outage are permanently
lost" (incorrect expectation). The closeTimeout-driven loss is the surprise.

## Assertion design

**SUT-side (`Always`, pre-rotation):** In `forwardMessages()`, before the
`select` that writes to `outputChan`, assert that `forwardContext` is not
already cancelled (i.e., `stopForward` hasn't fired while messages are still
being forwarded). This catches the case where `stopForward` races the forwarding
goroutine before all messages are sent.

**Workload-side (`Sometimes`):** Observe that `logs_component_utilization` for
the tailer stage hits 100% utilization (fully backpressured) at least once
during a partition run. Confirms the workload reaches the dangerous state.

**Workload-side (`Always`):** `BytesMissed` counter remains 0 unless a file
rotation occurred during the partition window. If no rotation occurred during
the fault injection window, no bytes should be lost.

## Open Questions

- Is there a workload-observable signal for "tailer is currently backpressured"
  without adding SUT-side instrumentation? `logs_component_utilization.ratio`
  approaching 1.0 is the closest proxy.

### Investigation Log

#### What is the actual behavior when `stopForward()` is called while `forwardMessages` is mid-decode (in the `for output := range decoder.OutputChan()` loop)?

- Examined: `pkg/logs/tailers/file/tailer.go:392-436` (`forwardMessages`), `tailer.go:306-338` (`StopAfterFileRotation`).
- Found: `forwardMessages` loops over `t.decoder.OutputChan()` with a range. For each decoded message the loop executes a `select { case t.outputChan <- msg: ... case <-t.forwardContext.Done(): }`. When `stopForward()` is called (cancelling `forwardContext`), the select resolves via the `Done()` case — the current message is silently dropped (no counter). The loop does NOT exit immediately: it `continue`s back to the outer `for output := range t.decoder.OutputChan()` loop. On the next iteration, it will again try to forward the next decoder message via the same select. Since `forwardContext` is already cancelled, the `Done()` case fires again immediately for every subsequent message. This continues until the decoder output channel is **closed** (when `readForever` stops and calls `t.decoder.Stop()`). So ALL messages remaining in the decoder output channel after `stopForward()` are silently dropped one by one — only the raw byte count (fileSize - lastReadOffset) is tracked via `BytesMissed`, not the count of individually dropped messages.
- Not found: any counter incremented per dropped message; any mechanism to exit the range loop early (the loop only exits when the channel is closed).
- Conclusion: resolved. After `stopForward()`, every pending decoded message is dropped in O(N) iterations through the range loop with zero per-message metric increments. The `BytesMissed` metric is the only observable signal and it counts bytes-not-read, not messages-dropped. This confirms the property's claim that loss is silent from a message-count perspective.

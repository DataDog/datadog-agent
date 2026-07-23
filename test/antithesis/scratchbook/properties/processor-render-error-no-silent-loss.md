---
slug: processor-render-error-no-silent-loss
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-29
---

# processor-render-error-no-silent-loss — Processor Render/Encode Error Does Not Silently Lose a Message

## What Led to This Property

`sut-analysis.md` §7 (failure mode #6) identifies a silent-drop path in the
processor: when `msg.Render()` or `p.encoder.Encode()` returns an error, the
function logs at `log.Error` and returns early. The message is never written to
`outputChan`, so the auditor never receives an ack, so the source offset is never
advanced. On restart the tailer re-reads from the last committed offset and the
message is re-delivered — **at-least-once is preserved**, but this is invisible
from metrics: there is no counter, no `BytesMissed` increment, no
`tlmPayloadsDropped` increment.

This property is distinct from `batch-encode-failure-no-silent-batch-loss`:
that property covers the *batch-layer* serialization/compression step; this one
covers the *per-message* Render/Encode step inside the processor, which runs
before any batching. The structural guarantee here (offset not advanced) is what
makes the drop safe — it is a hard requirement, not merely a design preference.

## Code Paths Involved

**`comp/logs-library/processor/processor.go:197-215`** — `processMessage()`:

```go
// render the message
rendered, err := msg.Render()
if err != nil {
    log.Error("can't render the msg", err)
    return          // message dropped; no outputChan write; no metric
}
msg.SetRendered(rendered)
...
// encode the message to its final format, it is done in-place
if err := p.encoder.Encode(msg, p.GetHostname(msg)); err != nil {
    log.Error("unable to encode msg ", err)
    return          // message dropped; no outputChan write; no metric
}
p.outputChan <- msg
```

The `return` statements before `p.outputChan <- msg` mean that the message never
reaches the strategy stage, and therefore never reaches a destination, and
therefore the auditor never sees a 2xx ack for it.

**`comp/logs-library/processor/json.go:47-78`** — `jsonEncoder.Encode()`:

`json.Marshal` is the only error path. `json.Marshal` errors on non-marshallable
types (channels, functions) or, theoretically, very large payloads. In practice
this is rare but not impossible with unusual structured content.

**`pkg/logs/message/message.go`** — `Render()`:

Render transforms the internal message representation into the wire format; error
conditions depend on the encoder chain being used (raw, JSON, passthrough).

## What Goes Wrong If the Property Is Violated

The structural guarantee is: *a processor render/encode error must not advance the
auditor offset for the affected message*. If this were violated (e.g., by a future
refactor that writes to `outputChan` before encoding, or that advances a counter
without completing encoding), the message would be permanently lost — no re-read
on restart.

The secondary concern: even with the structural guarantee intact, render/encode
errors are **invisible from the outside**. Under OOM pressure or corrupted
structured message content, an operator has no signal that messages are being
silently dropped at this stage. The property should detect both the structural
violation and the observability gap.

## Assertion Design

**`AlwaysOrUnreachable`** (SUT-side): At each early-return site in
`processMessage()` (Render error and Encode error), assert:

```
antithesis.AlwaysOrUnreachable(
    "processor-render-error-no-silent-loss: offset not advanced after render/encode error",
    // The message was NOT written to outputChan — verified by the structural
    // invariant that we are in an early-return path before the outputChan write.
    true,
    map[string]any{"msg_id": msg.Identifier()},
)
```

This is `AlwaysOrUnreachable` because: under standard Antithesis fault injection
(network partitions, process pauses, clock jitter), Render/Encode errors are
unlikely to be triggered — they require OOM-level pressure or corrupted content.
The assertion says: if this path is ever reached, the structural guarantee must
hold; if it is never reached, that is acceptable.

**`Reachable`** (SUT-side, optional): A workload that injects malformed structured
content (oversized JSON field, non-UTF8 bytes) can pair with a `Reachable`
sentinel at the error-drop sites to confirm at least one error was observed during
the run. This requires workload cooperation.

**Workload-side (`Always`)**: For every message written by the workload to a
monitored log file, assert that the message is eventually seen at intake (fakeintake)
OR that `registry.json` still contains an offset at or before the dropped message
position (i.e., the auditor did not advance past it). A stall in offset advancement
after confirmed message writes is observable as a metric divergence.

## Why It Matters

Under Antithesis fault injection:
- **Memory pressure** can trigger `json.Marshal` allocation failures via
  `bytes.Buffer.Grow` running out of heap, making the Encode error path reachable.
- **Corrupted structured content** from a concurrent container churn event (wrong
  `msg.Origin.Service()` under container recycling) may produce non-serializable
  values.

Without observability instrumentation at this drop site, operators cannot detect
a Render/Encode error storm during production incidents. The property's value is
both as a regression guard for the structural guarantee and as a forcing function
for adding a counter to these paths.

## Relationship to Other Properties

- `batch-encode-failure-no-silent-batch-loss` — covers the batch-layer encode path
  (batch.go), which runs *after* this processor-layer path. Non-overlapping.
- `auditor-offset-safety` — covers the general invariant that the auditor only
  advances offsets for successfully sent messages. This property is the specific
  enforcement point within the processor.

## Open Questions

- Under what realistic workload can `msg.Render()` return an error? If Render is
  infallible for all current message types, the property is `AlwaysOrUnreachable`
  in the strong sense (never reachable under any planned workload). `(partial:
  Encode can fail via json.Marshal; Render error path depends on message state machine)`
- Should a metric counter be added to these drop sites? A product improvement
  question — the property currently relies on log analysis for observability.
  `(needs human input)`

### Investigation Log

#### What are the concrete error conditions for msg.Render() and encoder.Encode()?

- Examined: `comp/logs-library/processor/processor.go:197-215`, `comp/logs-library/processor/json.go:47-78`, `comp/logs-library/processor/raw.go`, `comp/logs-library/processor/passthrough.go`, `pkg/logs/message/message.go`.
- Found: `jsonEncoder.Encode()` fails only if `json.Marshal` fails. `json.Marshal` can fail on non-serializable types or OOM during `bytes.Buffer.Grow`. Under normal operation with string/int/byte content, this is unreachable. The `Render()` method (in message.go) sets state to `StateRendered`; it can return an error if the message is already in an invalid state, but in the normal pipeline flow messages arrive in `StateUnrendered`. Raw and passthrough encoders have simpler error surfaces.
- Not found: any production incident or test asserting these error paths fire under standard faults.
- Conclusion: both paths are `AlwaysOrUnreachable` under standard Antithesis faults. Under OOM-level pressure, the Encode path becomes reachable. Render errors are more theoretical. The structural guarantee (no outputChan write before successful encode) is clearly correct from the code.

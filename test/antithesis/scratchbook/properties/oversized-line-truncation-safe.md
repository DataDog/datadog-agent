# Evidence: oversized-line-truncation-safe

## Summary

Log lines exceeding the configured size limit (`logs_config.max_message_size_bytes`,
default 256 KiB) are truncated by the decoder's `SingleLineHandler`. The
truncation must be safe: the agent must not panic, lose the truncated message,
drop the whole file position, or produce a message that itself exceeds limits.
The property also covers what happens when the batch-level content size limit is
hit: messages too large for even an empty batch are dropped with a warning
(`tlmDroppedTooLarge` counter) but no error metric.

## Key code

**`pkg/logs/internal/decoder/single_line_handler.go:51-81`** — truncation:
```go
func (h *SingleLineHandler) process(msg *message.Message) {
    lastWasTruncated := h.shouldTruncate
    content := msg.GetContent()
    h.shouldTruncate = len(content) > h.lineLimit || msg.ParsingExtra.IsTruncated

    content = bytes.TrimSpace(content)

    if lastWasTruncated {
        content = append(message.TruncatedFlag, content...)  // prepend TRUNCATED
    }
    if h.shouldTruncate {
        content = append(content, message.TruncatedFlag...)  // append TRUNCATED
        metrics.LogsTruncated.Add(1)
    }
    ...
    h.outputFn(msg)  // always called, even on truncated lines
}
```

The truncation flag is appended/prepended to the content. The resulting message
is always forwarded (`h.outputFn(msg)` is unconditional). The agent does NOT
drop truncated messages at this stage. `LogsTruncated` metric is incremented. ✓

**`pkg/logs/internal/decoder/line_parser.go:76-135`** — truncation tracking in
`MultiLineParser`:
```go
// buffer exceeds size cap — mark as truncated and let SingleLineHandler
// handle the truncation flag
isBufferTruncated bool
```
Multi-line aggregation respects the same size cap.

**`comp/logs-library/sender/batch.go:114-120`** — batch-level oversized drop:
```go
added, err = b.addMessage(m)
if err != nil {
    log.Warn("Encoding failed - dropping payload", err)
    b.resetBatch()
    return
}
if !added {
    log.Warnf("Dropped message in pipeline=%s reason=too-large ...")
    tlmDroppedTooLarge.Inc(b.pipelineName)
}
```
If a message (already truncated by decoder to `lineLimit`) is still too large
for an empty batch (`maxContentSize` is the batch-level limit), it is dropped
with a `log.Warn` and `tlmDroppedTooLarge` counter. **No error metric** — only
telemetry counter. No `BytesMissed` increment. No auditor offset tracking (the
message's offset is lost).

## The size boundary interaction

Two limits interact:
1. **Decoder line limit** (`logs_config.max_message_size_bytes`, default 256 KiB):
   Lines longer than this are truncated + flagged.
2. **Batch content size limit** (`logs_config.batch_max_content_size`): Maximum
   total bytes in a batch. A single truncated line message of 256 KiB +
   truncation flags may still exceed this if `batch_max_content_size < 256 KiB`.

If `lineLimit ≈ batch_max_content_size`, a message can pass the decoder but fail
the batch add — silent drop with only `tlmDroppedTooLarge` counter.

## Why it matters

Multi-line stacktraces are a common use case. A 500-line exception trace that
exceeds the line limit gets truncated, which corrupts the observability signal.
More critically, if the truncated message also exceeds the batch size limit, it
is silently dropped. This is a data integrity property.

## Assertion design

**SUT-side (`AlwaysOrUnreachable`):** At `h.outputFn(msg)` in
`SingleLineHandler.process()`, assert that the resulting `len(msg.GetContent())`
does not exceed `h.lineLimit + len(message.TruncatedFlag) * 2`. The truncation
flag is appended; the message content should be bounded.

**SUT-side (`Reachable`):** At `tlmDroppedTooLarge.Inc()` in
`processMessage()`, add `Reachable("batch-oversized-drop")` to confirm
the too-large-for-batch path is exercised.

**SUT-side (`Always`):** When `LogsTruncated` is incremented, the forwarded
message must have `IsTruncated = true` in its `ParsingExtra`. Structural check.

**Workload-side (`Always`):** For each truncated message that appears at
fakeintake, the message content must contain the `TRUNCATED` flag string (as set
by `message.TruncatedFlag`). A truncated log without the flag would mislead users.

## Open Questions

- None remaining — see Investigation Log.

### Investigation Log

#### What is the value of `message.TruncatedFlag`?

- Examined: `pkg/logs/message/message.go:21`.
- Found: `TruncatedFlag = []byte("...TRUNCATED...")`.
- Conclusion: **resolved** — confirmed `"...TRUNCATED..."`. Workload-side assertion
  must check for this exact substring in received message content.

#### When batch drops an oversized message (`tlmDroppedTooLarge`), does the auditor advance the offset?

- Examined: `comp/logs-library/sender/batch.go:90-121` (`processMessage()`),
  `comp/logs-library/sender/message_buffer.go:29-37` (`AddMessage()`),
  `comp/logs-library/sender/worker.go:101-210` (`run()`),
  `comp/logs/auditor/impl/auditor.go:285-296` (auditor `run()` loop).
- Found: When `addMessage(m)` returns `(false, nil)` (message too large, line 116-118
  of batch.go), the message metadata is **never** added to the `MessageBuffer`. The
  buffer is flushed without the oversized message's metadata. The auditor receives
  only `Payload.MessageMetas` — which does not include the dropped message. The
  auditor's `run()` loop (auditor.go:291-296) only updates the registry for messages
  present in `payload.MessageMetas`. Therefore, the auditor does NOT advance the
  offset for the oversized dropped message.
- Consequence confirmed: on agent restart, the tailer re-reads the oversized line
  from the last committed offset, the decoder truncates it again, the batch drops
  it again — infinite retry. The `BytesMissed` metric is also not incremented (that
  metric is only incremented in `StopAfterFileRotation` for unread bytes, not for
  batch-drop events).
- Conclusion: **resolved** — offset is NOT advanced. The oversized message creates
  a permanent re-read loop on restart. This is a significant data integrity issue
  beyond the original property scope; the property description should note this
  as a known gap in the current implementation.

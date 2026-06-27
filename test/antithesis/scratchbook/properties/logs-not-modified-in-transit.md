---
slug: logs-not-modified-in-transit
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# logs-not-modified-in-transit — Log Content Is Not Modified After Processing

## What Led to This Property

SUT guarantee S3 in the SUT analysis: log content is finalized (redacted →
rendered → encoded) synchronously in the processor before the message leaves
the processor stage. Post-processor stages (strategy, sender, destination)
should treat content as read-only bytes.

## Code Paths Involved

**Redaction** — `comp/logs-library/processor/processor.go:244-297`
`applyRedactingRules()` mutates `msg.GetContent()` in-place via
`msg.SetContent(content)`. This completes before `msg.Render()` is called.

**Render** — `processor.go:197` `msg.Render()` produces the rendered bytes;
the result is stored via `msg.SetRendered(rendered)`.

**Encode** — `processor.go:212` `p.encoder.Encode(msg, hostname)` encodes
in-place; on error the message is dropped (`return`).

**After the processor** — the message is pushed to `p.outputChan` (the
strategy input). From this point, the message object is shared across:
- Strategy (`comp/logs-library/sender/batch_strategy.go`) — assembles messages
  into `Payload`; reads `.GetContent()` / rendered bytes.
- `message.Payload` — wraps `[]*MessageMetadata` and encoded bytes.
- The sender worker and HTTP destination — only read `payload.Encoded`.

**Hazard: mutation after forwarding.** If any post-processor code path holds a
reference to `msg` and writes to it, content integrity breaks. The
`diagnosticMessageReceiver.HandleMessage(msg, rendered, "")` call at
`processor.go:205` shares the rendered content. In a race scenario (e.g., the
diagnostic receiver modifies the message while it is concurrently read by the
strategy), content could be observed in a corrupted intermediate state.

**Hazard: encode error silent drop.** `processor.go:212-215`: if `Encode()`
fails, the message is dropped silently (only a `log.Error`). This is not a
modification, but it means no content integrity can be checked for
encode-failure paths.

## Failure Scenario

Fault-injection angle: CPU throttle or thread-pause fault causing the
diagnostic receiver and strategy goroutines to interleave at exactly the right
time to observe a partially-written message. Under Antithesis, this timing can
be explored exhaustively.

A second angle: a compression library bug (e.g., the zstd C-heap corruption
noted in the SUT analysis bug history — `0d9dfc76f46`) corrupting `payload.Encoded`
after it has been set. The fakeintake would receive garbled bytes.

## Why It Matters

Corrupted log content silently delivers garbage to the user. Unlike a delivery
failure (which can be detected), content modification is invisible unless the
receiver checksums payloads — which the current fakeintake mock doesn't do.
This property, if violated, is extremely hard to detect in production.

## Workload Instrumentation

Each log line emitted by the workload should include a known checksum (e.g.,
CRC32 appended as a structured field). The fakeintake verifies that received
lines have the correct checksum for their content. SUT-side: a `Always`
assertion at the `outputChan <- msg` site confirming the rendered content is
byte-equal to what was produced by `Render()` — currently **missing**.

## Open Questions

- None remaining — see Investigation Log.

### Investigation Log

#### Does the `diagnosticMessageReceiver` run in a separate goroutine that could concurrently write to the shared `msg` object?

- Examined: `pkg/logs/diagnostic/message_receiver.go` — `BufferedMessageReceiver.HandleMessage()`
  (lines 102-111) and `Filter()` (lines 114-137).
- Found: `HandleMessage()` only writes the `*message.Message` pointer and `rendered []byte`
  into `b.inputChan` (a buffered channel). It does NOT modify any field of `msg`. The
  `Filter()` goroutine reads from `inputChan` and calls `b.formatter.Format(...)`, which
  only reads fields. No goroutine spawned by the diagnostic receiver ever calls
  `msg.SetContent()` or any mutating method.
- Not found: any write to `msg` fields after `HandleMessage` returns.
- Conclusion: **resolved** — no concurrent write hazard from the diagnostic receiver.
  The receiver is read-only with respect to the message object.

#### Does `msg.SetContent()` copy or alias the input bytes?

- Examined: `pkg/logs/message/message.go:256-265` (`SetContent()`) and
  `pkg/logs/message/message.go:421-432` (`BasicStructuredContent.SetContent()`).
- Found: For `StateUnstructured` (the common file-tailing case), `SetContent(content)`
  simply stores the slice reference: `m.content = content`. No copy is made. The
  underlying array is shared between the caller's slice and the stored slice.
  `applyRedactingRules` calls `rule.Regex.ReplaceAllLiteral(content, replacement)` which
  returns a **new** slice; the result is passed to `msg.SetContent()`. After this call,
  `msg.content` points to the new (redacted) slice. The pre-redaction bytes are referenced
  only by the old `content` local variable in the redaction function, which goes out of
  scope. No goroutine holds the pre-redaction bytes after `applyRedactingRules` returns.
- Conclusion: **resolved** — no alias hazard for the common case. SetContent stores a
  reference (no copy), but the caller always passes the new post-regex slice; the old
  slice is immediately unreachable.

#### Does the batch strategy retain a reference to `*message.Message` after encoding?

- Examined: `comp/logs-library/sender/message_buffer.go:29-37` (`AddMessage()`).
- Found: `AddMessage` explicitly copies `MessageMetadata`: `meta := message.MessageMetadata`
  (line 32, copy by value), then stores a pointer to that copy: `p.messageBuffer = append(p.messageBuffer, &meta)`.
  After `AddMessage` returns, the batch buffer holds only a `*MessageMetadata` (a copy of
  the metadata struct), not a `*Message`. The `*message.Message` object is not retained
  by the batch buffer. Once the processor pushes the message to the strategy's inputChan
  and the strategy calls `AddMessage`, the strategy holds only metadata — not the full
  message.
- Conclusion: **resolved** — the batch strategy does NOT retain a reference to
  `*message.Message` after encoding. The message object is not shared between goroutines
  after the strategy processes it.

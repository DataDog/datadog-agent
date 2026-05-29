---
slug: secrets-redacted-before-send
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# secrets-redacted-before-send — PII/Secret Patterns Are Redacted Before Transmission

## What Led to This Property

The `MaskSequences` processing rule exists specifically to strip PII and secret
patterns before logs leave the agent. This is a security-critical guarantee:
any code path that causes a message to bypass the processor — or that allows
un-redacted content to reach the wire — constitutes a secret-leak.

## Code Paths Involved

**Redaction enforcement point** — `comp/logs-library/processor/processor.go:192`
`applyRedactingRules(msg)` is called for every message in `processMessage()`.
For `MaskSequences` rules the function mutates `msg`'s content in-place.
Return value `false` drops the message entirely (for `ExcludeAtMatch`).

**Bypass paths to check:**
1. **Encode error drop** (`processor.go:213-215`): if encoding fails, `return`
   is called *without* sending the message anywhere — this is safe (the message
   is dropped, not sent un-redacted).
2. **Diagnostic receiver** (`processor.go:205`): `HandleMessage(msg, rendered, "")`
   is called *after* redaction but shares the rendered content with the
   `stream-logs` command. If the diagnostic receiver surfaces rendered content
   externally (e.g., to the agent CLI), this is a controlled disclosure path —
   but should still see redacted content.
3. **Failover pipeline routing** (`provider.go:372-388`): messages are routed
   after leaving the tailer but *before* the processor. The processor is per-pipeline,
   not per-tailer. Therefore messages always pass through a processor before the
   strategy — no bypass here as long as the pipeline topology is correct.
4. **Channel buffer drops** — if a message is dropped from a channel (e.g.,
   `NonBlockingSend` fails), it is not sent anywhere. This is also safe.
5. **Permanent 4xx advances auditor without send** — the content never reaches
   intake, so no bypass.
6. **Non-blocking send to unreliable destinations** — content has already been
   redacted; the payload sent to unreliable destinations is the same
   post-processor payload.

**The actual risk is in workload construction:** if a test sends logs whose
"secret" content is added after the processor (e.g., injected at the sender
layer), the test is broken by design. The relevant Antithesis risk is whether
a fault can cause a message to arrive at a destination without passing through
`applyRedactingRules()`.

## Failure Scenario

Primary risk: CPU pause or context switch at exactly the moment between
`applyRedactingRules()` returning and `msg.SetContent(content)` storing the
result (if there is a window of inconsistency). In Go, `SetContent` is a simple
store — the race window is extremely small. The realistic risk is a **code path
bug** where a new feature adds a bypass. Antithesis code-exploration finds such
paths.

Secondary risk: a double-send scenario (at-least-once replay after restart)
where the replayed payload is the already-encoded (post-redaction) bytes stored
in the registry — actually safe because the auditor stores file *offsets*, not
content. On replay, the raw file bytes are re-read and re-processed through the
full pipeline including redaction.

## Why It Matters

Sending un-redacted PII to the Datadog intake violates user data-handling
agreements and regulatory requirements (GDPR, HIPAA). Unlike most correctness
bugs, a secret-leak has legal consequences. Any code path that bypasses
`applyRedactingRules` is a P0 vulnerability.

## Workload Instrumentation

The workload writes log lines that include a sentinel secret pattern (e.g.,
`API_KEY=secret12345`) matching a configured `MaskSequences` rule that replaces
it with `[REDACTED]`. The fakeintake asserts:
1. No received log body contains `secret12345`.
2. At least some received log bodies contain `[REDACTED]` (confirming the rule fired).

SUT-side: an `Unreachable` assertion at any point where a log message exits the
agent process without first passing through `applyRedactingRules` — currently
**missing**.

## Open Questions

- Are there log sources that bypass the standard processor entirely, with
  per-source redaction rules not applied? For the channel launcher specifically,
  the pipeline it is assigned to has the global processing rules configured at
  provider construction — but per-source `MaskSequences` rules (defined in
  `conf.d/<check>.d/conf.yaml`) are applied at the processor stage using the
  message's source's `ProcessingRules`. Whether those per-source rules reach the
  channel-sourced messages depends on how the channel tailer sets `msg.Origin`.
  `(partial: channel launcher routes through processor — confirmed; whether
  per-source rules are applied to channel messages depends on Origin setup)`

### Investigation Log

#### Does the channel launcher bypass the processor?

- Examined: `pkg/logs/launchers/channel/launcher.go` (`startNewTailer` at line
  49-53) and `pkg/logs/tailers/channel/tailer.go` (`run()` at line 63-87).
- Found: `startNewTailer` calls `l.pipelineProvider.NextPipelineChan()` to obtain
  a pipeline's `InputChan`, then constructs a `channel.Tailer` that writes
  `message.Message` objects directly to that `outputChan`. The channel tailer does
  not have its own decoder or preprocessor — messages go directly from the
  `inputChan` (the `config.Channel`) to the pipeline's `InputChan`. The pipeline's
  `InputChan` feeds the processor (`processor.go:run()`), which calls
  `applyRedactingRules`. So channel-sourced messages DO pass through the processor.
- Not found: any shortcut path that avoids the processor for channel sources.
- Conclusion: channel sources route through the processor; global processing rules
  apply. Per-source rules depend on `msg.Origin` setup — tagged `(partial: ...)`.

#### Does the serverless Flush path apply the same `applyRedactingRules` as the normal `run` path?

- Examined: `comp/logs-library/processor/processor.go:136-153` (`Flush()`) and
  `processor.go:177-221` (`processMessage()`).
- Found: `Flush()` calls `p.processMessage(msg)` (line 150), which calls
  `p.applyRedactingRules(msg)` (line 192). The code path is identical to `run()`.
  There is no special-case in `processMessage` for serverless mode.
- Conclusion: **resolved** — the serverless Flush path applies exactly the same
  redaction logic as the normal run path. No bypass exists.

---
slug: at-least-once-no-loss
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# at-least-once-no-loss — Every Written Log Line Is Delivered At Least Once (No Permanent Loss)

## What Led to This Property

The logs agent's core delivery contract is at-least-once: after a transient
failure (network partition, agent restart), every log line that was written to
disk before the tailer started reading it will eventually be delivered. Duplicate
delivery is acceptable; silent permanent loss is not. This is the headline
property that all the other properties support or constrain.

## Code Paths Involved

**Happy path:**
- File tailer polls `readForever` → decoder → `forwardMessages` →
  `Pipeline.InputChan` → processor → strategy → sender worker →
  destination → auditor registry update.

**Loss path #1 — Rotation `closeTimeout` expiry** (`pkg/logs/tailers/file/tailer.go:306-338`):
- `StopAfterFileRotation()` fires after `closeTimeout` (default 60s).
- `t.stopForward()` cancels `forwardContext` → `forwardMessages` silently
  discards buffered decoded messages (sends `nil` or stops sending).
- `BytesMissed` is incremented (`metrics.BytesMissed.Add(remainingBytes)`).
- This is a **permanent loss path**: the lines in the rotation drain window that
  were not yet forwarded are gone.

**Loss path #2 — Non-blocking send drops** (`comp/logs-library/sender/worker.go:160-164`):
- `NonBlockingSend(payload)` fails silently when the destination input buffer is full.
- `tlmPayloadsDropped.Inc()` is the only signal.
- Not a retry path; those payloads are permanently dropped.

**Loss path #3 — Batch encode failure** (`comp/logs-library/sender/batch.go:94-119`):
- If batch encoding fails, the batch is discarded with only a `log.Warn`.
- No retry, no metric.

**Loss path #4 — Processor render/encode error** (`comp/logs-library/processor/processor.go:198-215`):
- `msg.Render()` or `p.encoder.Encode()` failure returns without sending.
- `log.Error` only; no metric.

**Loss path #5 — Non-atomic registry corruption** (Fargate):
- `comp/logs/auditor/impl/registry_writer.go:56-73`: `os.Create` truncates file,
  then writes. A crash between truncation and write completion leaves a zero-byte
  file.
- On restart: `recoverRegistry()` fails to parse the empty file → starts from
  default tailing mode (end-of-file) → all un-acked lines in the flush window
  are permanently lost.

**Recovery path (at-least-once guarantee):**
- `auditor.go:127`: `recoverRegistry()` reads the registry on `Start()`.
- Tailer `Start()` calls `GetOffset()` to find the resume point.
- If registry is valid: tailing resumes from last flushed offset → at-least-once
  for the 1-second flush window.

## Failure Scenario

**Network partition + file rotation under backpressure:**
1. Partition causes the destination to retry; the sender is blocked.
2. Backpressure propagates: `Pipeline.InputChan` fills; the file tailer's
   `forwardMessages` blocks.
3. A log rotation event fires; the new file gets a new tailer.
4. `StopAfterFileRotation()` is called on the old tailer.
5. The old tailer's `forwardContext` is cancelled after 60s; undrained messages
   in the decoder output are silently discarded.
6. When the partition clears, those lines are never re-read (the file is rotated
   away).

**Kill -9 during flush window:**
1. Payload delivered to intake; `output <- payload` called.
2. Agent killed before the 1-second flush tick.
3. Registry still shows old offset.
4. On restart: lines are re-read from old offset → duplicates (at-least-once
   maintained).

**Registry corruption (Fargate, non-atomic writer):**
1. Flush ticker fires; `os.Create` truncates the file.
2. Agent killed between truncate and write completion.
3. Registry is zero bytes; parse fails.
4. On restart: tailing starts from end-of-file → all un-acknowledged lines lost.

## Why It Matters

Silent log loss is the #1 user complaint about the logs agent (product context
item #1: "Silent log loss during file rotation under backpressure"). No existing
integration test exercises this scenario end-to-end. The `BytesMissed` metric
is the only production signal, and users typically don't monitor it.

## Workload Instrumentation

- Workload writes N log lines with sequence numbers 1..N, then rotates the file.
- After a quiet period (partition cleared, agent recovered), fakeintake checks
  that lines 1..N were all received.
- Acceptable: duplicate delivery of some lines (at-least-once).
- Not acceptable: any line number in 1..N absent from the fakeintake (permanent
  loss).
- SUT-side: a `Sometimes` assertion confirming `BytesMissed` is 0 during
  rotation under load would confirm the no-loss path was exercised. Currently
  **missing**.

## Open Questions

- What is `close_timeout` in the test topology? Default is 60s
  (`logs_config.close_timeout=60`). If not overridden, loss under backpressure
  requires the pipeline to be blocked for >60s. If set lower in the test
  topology, loss is more likely. The property is parametric on this configuration.
  `(needs human input)`
- Is there any mechanism to recover lost lines after `closeTimeout` expiry other
  than re-reading from file? (Answer appears to be no, but the design docs
  mention "payload journaling" — does this apply?)
- For the Fargate non-atomic writer: is this path reachable in a Linux container
  test topology, or only on real AWS Fargate? Can be forced with
  `DD_LOGS_CONFIG_ATOMIC_REGISTRY_WRITE=false`. `(needs human input)` — whether
  the test topology enables this path.

### Investigation Log

#### Is `BytesMissed` an expvar/in-memory counter (resets on restart)?

- Examined: `comp/logs-library/metrics/metrics.go:65-69`.
- Found: `BytesMissed = expvar.Int{}` — declared as a package-level `expvar.Int`. It is registered into `LogsExpvars` (line 184) so it appears in `/vars` HTTP endpoint. `TlmBytesMissed` is a Prometheus-style counter via telemetry (`logs.bytes_missed` counter). `expvar.Int` is an in-memory counter: **it resets to 0 on every agent restart**. The Prometheus/telemetry counter (`TlmBytesMissed`) may or may not persist depending on the telemetry backend.
- Conclusion: resolved. `BytesMissed` is in-memory only, resets on restart. The workload cannot use it to detect loss across restart events — it can only detect within-session loss. Sequence-gap analysis in fakeintake is the correct approach for across-restart loss detection. Removed from Open Questions.

#### Is `close_timeout` default 60s?

- Examined: `pkg/config/setup/common_settings.go:1874`.
- Found: `config.BindEnvAndSetDefault("logs_config.close_timeout", 60)` — the value is in seconds (per `tailer.go:172` which multiplies by `time.Second`). Default is 60 seconds.
- Conclusion: resolved. Tagged remaining question `(needs human input)` since the test topology config is not in code.

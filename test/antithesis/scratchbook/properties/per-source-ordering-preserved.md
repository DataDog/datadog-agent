---
slug: per-source-ordering-preserved
sut_path: /home/ssm-user/src/datadog-agent
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# per-source-ordering-preserved — Per-Source Log Order Is Preserved Within a Pipeline

## What Led to This Property

The logs-agent README states (README:156) that log lines from a single source
are delivered in order. The pipeline enforces this via FIFO Go channels plus
single-goroutine stages. However, two code paths silently break this guarantee
and neither is documented in user-facing materials.

## Code Paths Involved

**Normal path (ordering preserved):**
- Each tailer owns one `outputChan`; one goroutine (`forwardMessages`) drains it.
- `Tailer.outputChan` is wired directly to a specific `Pipeline.InputChan` via
  `NextPipelineChan()` / `NextPipelineChanWithMonitor()`.
- `Pipeline.InputChan → Processor → Strategy → Sender` are all single-goroutine
  FIFO stages — ordering is structurally guaranteed.

**Break #1 — `pipeline_failover.enabled=true`:**
- `comp/logs-library/pipeline/provider.go:353-388` — `forwardWithFailover()`.
  When the primary pipeline `InputChan` is full, `trySendToPipeline()` iterates
  through other pipelines with non-blocking sends (line 373-388).
- Result: consecutive messages from the same source can land in different
  pipelines, processed and sent concurrently → interleaved delivery.
- This is by design for throughput, but no documentation warns that ordering
  breaks.

**Break #2 — File rotation crosses pipeline assignment boundary:**
- `pkg/logs/launchers/file/launcher.go:663` — when a rotated file spawns a new
  tailer, `NextPipelineChan()` round-robins to the *next* pipeline index.
- The draining old tailer and the new tailer for the same logical source are
  now on different pipelines → reorder possible at the rotation boundary.
- Noted in `pkg/logs/tailers/file/tailer.go:260` FIXME for container rotation
  (same mechanism).

## Failure Scenario

1. Network partition causes the primary pipeline's destination to queue and back
   up.
2. `forwardWithFailover` routes message N to pipeline 1 (unblocked), then
   message N+1 to pipeline 0 (blocked, falls back to pipeline 1 via failover),
   then message N+2 also succeeds on pipeline 1.
3. When the partition clears, pipeline 0 drains its backlog; pipeline 1 sends in
   arrival order.
4. Intake receives messages from the same source in an interleaved order.

Alternatively: rotate a log file while the sender is mid-retry. Old tailer
(pipeline 0) hasn't finished its tail; new tailer (pipeline 1) starts sending
immediately. Ordering at the rotation boundary is arbitrary.

## Why It Matters

Out-of-order log lines corrupt temporal analysis in Datadog Log Management:
- Stack trace lines arrive in the wrong order → multiline assembly fails.
- Request/response pairs appear reversed → tracing broken.
- Auditors reading sequential entries are confused.

The guarantee is stated publicly; violating it silently undermines user trust.

## Workload Instrumentation

Each log line emitted by the workload should embed a strictly-increasing
sequence number per source (e.g., `SEQ=00001`). The fakeintake verifies that
received messages from each source have non-decreasing sequence numbers.
SUT-side instrumentation (`Always` assertion at the auditor or intake-proxied
verification) is currently **missing**.

## Open Questions

- Is `pipeline_failover.enabled` ever default-on in any deployed config? `(needs human input)`
- At the rotation boundary: is there a protocol-level ordering guarantee from
  the HTTP intake that two payloads submitted in sequence are stored in
  submission order? If yes, the rotation break matters only for concurrent
  submissions from different pipelines. `(needs human input)`
- What is the standard test topology pipeline count? With a single pipeline,
  `forwardWithFailover` never distributes across pipelines → Break #1 is unreachable.
  `(needs human input)`
- Does the intake guarantee ordering within a single TCP connection / HTTP/2
  stream? `(needs human input)`

### Investigation Log

#### Is `pipeline_failover.enabled` ever default-on?

- Examined: `pkg/config/setup/common_settings.go:1994`:
  `config.BindEnvAndSetDefault("logs_config.pipeline_failover.enabled", false)`.
- Found: The default is `false`. Break #1 (cross-pipeline reordering) requires
  explicit opt-in via `logs_config.pipeline_failover.enabled=true`. The break only
  manifests in deployments that enable this feature, or in a test topology that
  explicitly sets it.
- Conclusion: **partially resolved** — default is confirmed off; whether any
  deployed Datadog customer configuration enables it in practice is `(needs human input)`.

#### What is the default `router_channel_size`?

- Examined: `pkg/config/setup/common_settings.go:1995`:
  `config.BindEnvAndSetDefault("logs_config.pipeline_failover.router_channel_size", 5)`.
- Found: Default is 5. This means `forwardWithFailover` has a 5-message buffer per
  router channel. The reorder window (number of messages that can be in-flight across
  different pipelines simultaneously) is bounded by this buffer size.
- Conclusion: **resolved** — default router channel size is 5.

## Merged-in evidence (from cross-pipeline-ordering-under-failover)

The secondary file focused exclusively on the `pipeline_failover.enabled=true`
path and supplied the following additional detail not present in the canonical:

**Coordination gap** — there is **no cross-pipeline coordination** to enforce
that pipelines delivering messages from the same source do so in submission
order. Source attribution is lost once messages enter `InputChan`; the sender
cannot detect that M1 and M2 are from the same source.

**Rotation cross-cut** — `createRotatedTailer` calls
`NextPipelineChanWithMonitor()` (`launcher.go:665`), which in failover mode
round-robins via `p.currentRouterIndex.Inc()` (`provider.go:345`). The new
tailer gets the *next* router-channel index, so old and new tailers may land on
different pipelines simultaneously. Ordering loss and identifier collision can
therefore co-occur in the same rotation event — a compound failure where neither
symptom alone is obvious.

**Additional SUT-side instrumentation noted as missing:**
- `Sometimes("failover-routing-triggered")` — inside `trySendToPipeline` when
  `attempt > 0` and a non-primary pipeline receives the message.
- `Reachable("trySendToPipeline-returned-false")` — when all pipelines are full.

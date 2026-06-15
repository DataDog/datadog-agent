# Loss-gated logs performance-profile recommendations

**Date:** 2026-06-15
**Branch:** `UTXOnly/performance-profiles`
**Status:** Design — pending implementation plan

## Problem

The logs-agent recommends a performance profile (shown in `agent status`)
whenever the backpressure state is `SATURATED` or `WARNING`. That state is
derived purely from per-component utilization saturation
(`getBackpressureStatus`, `pkg/logs/status/builder.go`).

Saturation is not harm. A pipeline can run at 100% utilization indefinitely
without losing a single log. Recommending a profile change on saturation alone
produces false positives — nagging operators to retune when nothing is wrong.

Separately, when logs *are* being lost, the loss counts are not surfaced in the
top metrics block of the status output, so an operator cannot easily see that
loss is happening.

## Goal

Recommend a profile only when the agent is **actually losing logs**, use *which*
loss signal is firing to localize the bottleneck, and make loss visible in the
status output.

## Loss signals

The agent already exposes two distinct loss counters, registered in the
`logs-agent` expvar map (`comp/logs-library/metrics/metrics.go`):

| Metric (expvar key) | Incremented at | Meaning | Pipeline end |
|---|---|---|---|
| `BytesMissed` (`*expvar.Int`) | file tailer, `pkg/logs/tailers/file/tailer.go:325` | A file rotated away before the tailer drained it. Backpressure-induced read-side loss (or rotation simply outran a slow reader). | Upstream |
| `DestinationLogsDropped` (`*expvar.Map`, per host) | HTTP/TCP destinations, `comp/logs-library/client/http/destination.go:313`, `.../tcp/destination.go:151` | A payload hit a permanent send error, or a non-reliable endpoint gave up. Send-side failure, not pure backpressure. | Downstream |

Key insight: backpressure propagates **upstream**. When any stage stalls, the
loss symptom surfaces at the most-upstream point (the tailer → `BytesMissed`),
regardless of which stage is the true bottleneck. So `BytesMissed` is a *trigger*
that says "loss is happening" — the saturation table is still needed to
*localize* the bottleneck. `DestinationLogsDropped` is measured at the
destination, so it localizes itself to the send stage.

## Design

### 1. Surface loss in the top metrics block

`getMetricsStatus()` (`pkg/logs/status/builder.go`) builds the `StatusMetrics`
map rendered at the top of the logs-agent status. Add two keys:

- `BytesMissed` — the `BytesMissed` counter value.
- `LogsDropped` — the sum of the per-host `*expvar.Int` values in
  `DestinationLogsDropped`.

The status template (`comp/logs/agent/impl/status_templates/logsagent.tmpl` and
`logsagentHTML.tmpl`) ranges over `StatusMetrics` in sorted-key order, so both
keys render automatically with no template change (`BytesMissed` after
`BytesSent`, `LogsDropped` among the `Logs*` entries).

This is independent of any recommendation: loss is always visible.

### 2. New loss accessors

Add two helpers on `Builder`, mirroring the existing `senderLatencyMs()`:

```go
// bytesMissed returns total bytes lost before consumption (e.g. file rotation
// outpacing the tailer), or 0 when unavailable.
func (b *Builder) bytesMissed() int64

// logsDropped returns total logs dropped across all destinations (permanent
// send failures / non-reliable endpoints giving up), or 0 when unavailable.
func (b *Builder) logsDropped() int64
```

Both nil-guard `b.logsExpVars`. `logsDropped` sums across the per-host map.

### 3. Reworked `getProfileRecommendation`

Loss becomes the gate; saturation is demoted from trigger to localizer.

New signature (drops the now-unused `bp`; loss values passed in like
`latencyMs`, read in `BuildStatus` via the accessors so the function stays
unit-testable without touching global expvars):

```go
func (b *Builder) getProfileRecommendation(
    utils []ComponentUtilization,
    activeProfile string,
    latencyMs, dropped, missed int64,
) *ProfileRecommendation
```

Logic:

1. **Gate.** If `dropped == 0 && missed == 0` → return `nil`. Saturation alone
   never recommends.
2. **Send-side loss wins (most specific).** If `dropped > 0`:
   - Find the most-downstream saturated stage (current saturation; fall back to
     recent 1m/30m).
   - If it is the send stage (`worker`, `SenderTlmName`, or `destination_*`) →
     recommend `high-concurrency`, citing intake latency when
     `latencyMs >= senderLatencyHighThresholdMs` (250ms). Drops corroborated by
     downstream saturation are load-driven.
   - Otherwise (no downstream saturation) → the drops are permanent send errors
     (4xx/auth/payload), which no profile fixes → return `nil`.
3. **Read-side backpressure loss.** Else (`missed > 0`):
   - Localize via the saturation table: most-downstream saturated stage
     (current; fall back to recent 1m/30m) → `recommendProfileForBottleneck`.
   - If **nothing** is saturated → loss is rotation outrunning an idle reader;
     the remedy is `logs_config.close_timeout`, not a perf profile → return
     `nil`.
4. **Suppress redundant recommendation.** If the recommended profile is `""` or
   equals `activeProfile` → return `nil` (existing guard, unchanged).

`recommendProfileForBottleneck` is reused unchanged. The bottleneck-localization
helper `mostDownstreamSaturated` is reused. Reason strings are updated so they
state that loss is occurring (e.g. "Logs are being lost; the pipeline is
bottlenecked at the network send stage."), distinguishing the message from the
old saturation-only phrasing.

### 4. Unchanged

- `getBackpressureStatus` and the `SATURATED`/`WARNING`/`HEALTHY` state stay as
  informational diagnostics.
- The rendered backpressure table stays. (Loss visibility is handled by the top
  metrics block in §1, so no table change is required.)
- The profile catalog (`pkg/config/setup/logs_performance_profiles.go`) is
  unchanged.

## Behavior matrix

| Condition | Recommendation |
|---|---|
| No loss (any saturation) | none |
| `missed>0`, processor saturated | `high-throughput` |
| `missed>0`, strategy saturated | `high-throughput` |
| `missed>0`, worker/destination saturated | `high-concurrency` |
| `missed>0`, nothing saturated | none (close_timeout hint territory) |
| `dropped>0`, worker/destination saturated | `high-concurrency` |
| `dropped>0`, no downstream saturation | none (permanent send errors) |
| recommended profile already active | none |

## Testing

In `pkg/logs/status/builder.go`'s test file
(`pkg/logs/status/status_test.go`):

- Update existing `TestProfileRecommendation_*` tests to the new signature
  (pass `dropped`/`missed`, drop `bp`) and inject loss so they still exercise
  localization.
- Add cases covering each row of the behavior matrix above, in particular the
  headline change: **saturated with zero loss → `nil`**.
- Add a test that `getMetricsStatus` includes `BytesMissed` and `LogsDropped`
  keys, summing across multiple destination hosts for `LogsDropped`.

The `TestMetrics` golden-JSON assertions in `status_test.go` already include
`BytesMissed` and `DestinationLogsDropped` (they serialize the raw expvar map);
confirm those remain correct — only the human-rendered `StatusMetrics` map
gains keys, not the JSON expvar dump.

## Out of scope

- Rate/windowed loss detection (deltas over time). We use a simple non-zero
  threshold since agent start; the recommendation is advisory.
- Loss signals for non-file, non-HTTP/TCP sources (UDP socket overflow,
  journald, integration channel drops). Neither counter covers those; left for
  future work.
- Automated profile application. Recommendations remain advisory output in
  `agent status`.

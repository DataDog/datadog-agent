# Repeated log failure evidence

The opt-in `log_failure` detector recognizes repeated application errors without
statistical warmup, prior healthy observations, or a previously seen log pattern.
Failure evidence describes the signal; it does not establish urgent business impact.

## Qualification and renewal

The fixed rule requires at least three accepted ERROR-or-higher logs across
two occupied five-second intervals in the last thirty seconds. It pools patterns
and containers within each host, service, environment and Observer input.

Application levels come from the level the application wrote: a JSON level field
(any key case, including pino/bunyan numeric levels), a klog prefix such as
`E1116 10:00:00.000000`, `level=error` (logfmt), `[ERROR]`, an upper-case level
near the start of the line such as `... ERROR [main] ...`, or `ERROR:`.
Container stderr alone does not establish an application error. Agent-internal
logs can use the severity supplied by the Agent logger. Messages over 16 KiB and
messages with no recognized application level, such as bare stack-trace lines,
are not classified. Non-error logs establish zero-error history; intervals with
no accepted logs remain missing.

Analysis advances only when input arrives, so an interval can become visible to
the detector well after it completed. Each newly visible positive interval is
judged once, by the window ending at that interval; evidence keeps the
interval's time and is reported at the next analysis. Old errors, zero counts,
and missing observations cannot renew evidence. A later repeated failure can
qualify again. The rule uses Observer data time; total input silence does not
establish recovery.

The detector shares an error-count extractor whose pending state is capped at
256 scopes and two intervals per scope. Shared storage limits bound retained
series and detector state; eviction and reset clear auxiliary state. Pending
capacity drops increment `observer.log_error_count.capacity_dropped`.

Statistical detectors exclude these error-count series before creating models
when `log_failure` is enabled. Other log-derived and native metric detection
continues normally, including the kind-routed metric pipeline.
Emissions carry `EvidenceFailure` and the detector's `failure_weight`
(`anomaly_detection.detectors.log_failure.failure_weight`, default 3, maximum 6).
While a detector's failure evidence is in the scorer window, the scorer floors its
input at `weight × (1 − e^(−1/k))`; with several failure detectors, the largest
fresh weight applies. The default floor (≈0.54) exceeds the High threshold, so
sustained failure evidence can reach High on its own; 0 gives the ordinary
contribution. The floor does not grow with more failing
series. Repeated emissions refresh the same contributor rather than adding one per log.
An open scorer episode lists each failure series once, so repeats do not crowd
other detectors out of its capped member list.

## Baseline analysis

Like other detectors, `log_failure` follows `anomaly_detection.baseline_analysis`
(enabled by default, 10 minutes from Observer start). Evidence during the window
is suppressed. With `mute_noisy_metrics` (default true), each scope that qualified
during the window is muted until the Agent restarts, so errors that repeat from
startup do not keep raising evidence. A scope that logged only isolated errors
during the window, or that first appears after it, is not muted. A failure that
begins during the window is not reported, and a muted scope's later errors are
ignored, including new kinds of error. The detector has no separate baseline
setting.

## Enable for a controlled experiment

```yaml
anomaly_detection:
  anomaly_scorer:
    dry_run:
      enabled: true
  logs:
    containers:
      enabled: true
  detectors:
    log_failure:
      enabled: true
```

The detector is disabled by default. The error-count extractor runs exactly when
the detector is enabled and has no setting of its own. Enabling the detector does
not activate Observer without an existing consumer or the scorer's dry-run consumer.
Container application logs reach the detector only when `logs.containers.enabled`
is true.

Ingress severity gates, processing rules and rate limits apply before extraction.
Container stdout is transported as info. While the detector is
enabled, a log below its input's `min_severity` is still admitted when the
application declares ERROR or higher, and is then rate limited at that level.
Declared non-error logs stay subject to the minimum, so zero-error history from
INFO-only output requires that input's `min_severity` to be `info`. Replay
bypasses these gates and does not validate live admission or unsampled
application rates. Recurring errors that begin after the baseline window, such
as periodic database or instrumentation errors, may still qualify during
otherwise healthy workload periods. Review severity timelines and
healthy-workload results before enabling this path broadly.

## Replay configuration

The testbench `--config` JSON accepts the same `failure_weight` as the Agent:

```json
{"components": {"log_failure": {"enabled": true, "failure_weight": 3}}}
```

The qualification thresholds are fixed and are not configurable.

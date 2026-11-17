# Repeated log failure evidence

The opt-in `log_failure` detector recognizes repeated application errors without
statistical warmup, prior healthy observations, or a previously seen log pattern.
Failure evidence describes the signal; it does not establish urgent business impact.

## Qualification and renewal

The default rule requires at least three accepted ERROR-or-higher logs across
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
continues normally, including the kind-routed metric pipeline. Statistical
baseline qualification and noisy-series muting do not suppress direct evidence.
Emissions carry `EvidenceFailure`. While failure evidence is in the scorer
window, `anomaly_detection.anomaly_scorer.failure_weight` (default 3, maximum
10, 0 disables) floors the scorer input at `weight × (1 − e^(−1/k))`. The default
floor (≈0.54) exceeds the High threshold, so sustained failure evidence can reach High
on its own. The floor does not grow with more failing
series. Repeated emissions refresh the same contributor rather than adding one per log.
An open scorer episode lists each failure series once, so repeats do not crowd
other detectors out of its capped member list.

## Enable for a controlled experiment

```yaml
anomaly_detection:
  anomaly_scorer:
    dry_run:
      enabled: true
  detectors:
    log_failure:
      enabled: true
```

The detector is disabled by default. Enabling it also enables the required
extractor, but does not activate Observer without an existing consumer or the
scorer's dry-run consumer. Explicitly disabling the required extractor is an error.

Ingress severity gates, processing rules and rate limits apply before extraction.
Container stdout is transported as info. While error-count extraction is
enabled, a log below its input's `min_severity` is still admitted when the
application declares ERROR or higher, and is then rate limited at that level.
Declared non-error logs stay subject to the minimum, so zero-error history from
INFO-only output requires that input's `min_severity` to be `info`. Replay
bypasses these gates and does not validate live admission or unsampled
application rates. Recurring database or instrumentation errors may qualify
during otherwise healthy workload periods. Review severity timelines and
healthy-workload results before enabling this path broadly.

## Replay tuning

The testbench's existing `--config` JSON supports bounded experiments:

```json
{
  "components": {
    "log_failure": {
      "enabled": true,
      "min_errors": 3,
      "min_intervals": 2,
      "window_seconds": 30
    }
  }
}
```

Omitted fields retain the defaults. Error and interval minima must be positive;
the lookback must be a multiple of five from 5 to 300 seconds, and must fit the
required intervals. Bucket width stays at five seconds. Reset keeps
the selected rule, storage retains its required history, and anomaly descriptions
include its thresholds. These overrides are replay-only; the Agent uses 3/2/30.

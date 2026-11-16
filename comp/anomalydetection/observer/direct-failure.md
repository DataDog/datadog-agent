# Repeated log failure evidence

The opt-in `log_failure` detector recognizes repeated application errors without
statistical warmup, prior healthy observations, or a previously seen log pattern.
Failure evidence describes the signal; it does not establish urgent business impact.

## Qualification and renewal

The default rule requires at least three accepted ERROR-or-higher logs across
two occupied five-second intervals in the last thirty seconds. It pools patterns
and containers within each host, service, environment and Observer input.

Application levels come from supported JSON fields or a declared text level.
Container stderr alone does not establish an application error. Agent-internal
logs can use the severity supplied by the Agent logger. Messages over 16 KiB and
messages with no recognized application level are not classified.

Only a fresh positive interval renews evidence. Old errors, zero counts, and
missing observations cannot renew it. A later repeated failure can qualify again.
The rule uses Observer data time; total input silence does not establish recovery.

The detector shares an error-count extractor whose pending state is capped at
256 scopes and two intervals per scope. Shared storage limits bound retained
series and detector state; eviction and reset clear auxiliary state. Pending
capacity drops increment `observer.log_error_count.capacity_dropped`.

Statistical detectors exclude these error-count series before creating models
when `log_failure` is enabled. Other log-derived and native metric detection
continues normally, including the kind-routed metric pipeline. Statistical
baseline qualification and noisy-series muting do not suppress direct evidence.
Emissions carry `EvidenceFailure` and receive the ordinary scorer contribution.
Repeated emissions refresh the same contributor rather than adding one per log.

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
Application ERROR messages written to stdout require INFO transport logs to be
admitted at that input. Replay bypasses these gates and does not validate live
admission or unsampled application rates. Recurring database or instrumentation
errors may qualify during otherwise healthy workload periods. Review severity
timelines and healthy-workload results before enabling this path broadly.

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
required intervals. Bucket width and freshness stay at five seconds. Reset keeps
the selected rule, storage retains its required history, and anomaly descriptions
include its thresholds. These overrides are replay-only; the Agent uses 3/2/30.

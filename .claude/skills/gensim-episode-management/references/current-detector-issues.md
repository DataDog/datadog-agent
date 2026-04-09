# Current Detector Issues (as of 2026-03)

This file contains detector-specific known issues that change as the observer
algorithms evolve. Verify these are still accurate before relying on them.

## BOCPD Warmup

BOCPD (Bayesian Online Changepoint Detection) needs `WarmupPoints` data points
before it starts detecting. Check `DefaultBOCPDConfig()` in
`comp/observer/impl/metrics_detector_bocpd.go` for the current value (120 as of
this writing).

Impact by metric frequency:

| Metric type | Typical interval | Warmup time |
|---|---|---|
| Virtual log patterns | ~1s | ~2 min |
| DogStatsD metrics | ~10s | ~20 min |
| Check metrics (redis, pg) | ~15s | ~30 min |
| Trace stats | ~10s | ~20 min |

With a 10-minute baseline, only high-frequency metrics (virtual logs) complete
warmup before disruption starts. Expect the observer to primarily fire on
log-derived metrics for scenarios with slow-interval check/trace metrics.

## ScanWelch / Batch Detector Eviction

Batch detectors (ScanWelch, ScanMW) timestamp anomalies at the historical
changepoint, not at detection time. The TimeCluster correlator's eviction window
(`WindowSeconds`, default 120s in `DefaultTimeClusterConfig()`) can evict these
anomalies before the EventReporter sees them.

This means batch detectors work in testbench (which reads `rawAnomalies` directly)
but may not produce Datadog events in live mode. The fix is to either:
- Move `correlator.Advance()` before the detector loop in `engine.go`
- Make `WindowSeconds` configurable and set it higher

Check `comp/observer/impl/engine.go` `runDetectorsAndCorrelatorsSnapshot()` to
see if this has been fixed.

## Silent Observation Channel Drops

The observer's observation channel (`observer.go` line ~235) has a fixed buffer
with non-blocking sends. Under load, observations are silently dropped. This
affects live mode only -- testbench replays bypass the channel entirely.

Check `observer.go` for the current buffer size and whether drop metrics have
been added.

## min_cluster_size Filtering

The TimeCluster correlator's `min_cluster_size` controls how many anomalies must
cluster together (within `ProximitySeconds`) before an event is emitted. Most
single-disruption scenarios produce clusters of size 1-2. Setting this to 3+
filters out ALL events in many scenarios.

For evaluation, use `min_cluster_size: 1`. For production noise reduction, tune
based on observed cluster sizes in your specific scenarios.

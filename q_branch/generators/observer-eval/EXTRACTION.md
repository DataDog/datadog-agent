# Scrappy Training Data: Extraction Methodology

What the ScrappyCollector captures, where it comes from, and why each piece matters
for the Scrappy anomaly detection model.

## What We're Capturing

Every ~15-30 seconds, the observer's detection pipeline fires. The ScrappyCollector
runs as a detector and snapshots the **entire metric surface** visible to the observer
at that moment — every time series across all namespaces, with their current values.

The output is a JSONL file where each line is one tick:

```jsonc
{
  "data_time": 1777496208,
  "series": [
    // System metric — name preserved as-is
    {"ns": "system-checks-hf", "name": "system.cpu.user", "tags": ["core:0"], "points": [{"ts": 1777496208, "val": 23.4}]},
    // Log metric from Drain clusterer — pattern is the Drain template
    {"ns": "log_pattern_extractor", "pattern": "GET /api/v1/* HTTP/1.1 * *ms", "tags": ["service:webapp"], "points": [{"ts": 1777496208, "val": 12.0}]},
    // Log metric from structural extractor — pattern is the C/D signature
    {"ns": "log_metrics_extractor", "pattern": "CCC CCCC:DDD:DD.DDD CCCCC CC CCCC DDDD", "tags": ["level:info"], "points": [{"ts": 1777496208, "val": 1.0}]}
  ]
}
```

## Data Sources (Namespaces)

The observer receives data from multiple internal pipelines. Each produces series in
a distinct namespace:

### `system-checks-hf` (~75% of series)

**Source:** High-frequency system check runner (`comp/observer/impl/hfrunner/`).
Runs standard agent system checks at 1-second intervals instead of the default 15s.

**What it contains:**
- CPU: `system.cpu.user`, `system.cpu.system`, `system.cpu.idle`, `system.cpu.iowait`
- Memory: `system.mem.total`, `system.mem.used`, `system.mem.free`, `system.mem.slab`, `system.mem.cached`
- Disk: `system.disk.read_time`, `system.disk.write_time`, `system.io.read_bytes`, `system.io.write_bytes`
- Network: `system.net.bytes_rcvd`, `system.net.bytes_sent`, `system.net.packets_in.error`
- Load: `system.load.1`, `system.load.5`, `system.load.15`
- Filesystem: `system.fs.inodes.used`, `system.fs.file_handles.used`

**Why it matters:** These are the continuous health signals. CPU saturation, memory
pressure, disk I/O spikes, and network errors are the primary indicators of system-level
anomalies. The 1-second cadence means the model sees fine-grained dynamics during
incidents, not smoothed-out 15-second averages.

**Controlled by:** `DD_OBSERVER_HIGH_FREQUENCY_SYSTEM_CHECKS_ENABLED=true`

### `log_metrics_extractor` (~23% of series)

**Source:** `LogMetricsExtractor` (`comp/observer/impl/log_metrics_extractor.go`).
Processes every log line the agent sees and emits a count metric per unique structural
pattern.

**Pattern format:** Structural fingerprint using `logSignature()`. Letters become `C`
(run-length encoded), digits become `D`, punctuation is preserved:
```
Input:  "Starting server on port 8080"
Pattern: "CCCCCCCC CCCCCC CC CCCC DDDD"
```

**What it contains:** One `log.pattern.<fnv64a(signature)>.count` metric per unique
log structure. The value is the count of matching log lines in the observation window.

**Why it matters:** Log volume and pattern frequency shifts are strong anomaly signals.
A pattern that normally appears 5x/sec suddenly appearing 500x/sec (error flood) or
dropping to 0 (service crash) is directly observable in these counts. The structural
fingerprint groups logs by shape rather than content, which is robust to variable fields
(timestamps, IDs, values).

**Limitation:** The `logSignature` fingerprint is opaque — `CCCCCC DDDD` doesn't tell
you whether this is a health check, an error, or a startup message. For semantic
classification, use `log_pattern_extractor` patterns instead.

### `log_pattern_extractor` (<1% of series)

**Source:** `LogPatternExtractor` (`comp/observer/impl/log_pattern_extractor.go`).
Uses a Drain-style log clustering algorithm with typed token classification.

**Pattern format:** Drain template with preserved words and `*` wildcards:
```
Input:  "GET /api/v1/users HTTP/1.1 200 4ms"
Pattern: "GET /api/v1/* HTTP/1.1 * *"
```

Tokens are typed during parsing (`comp/observer/impl/patterns/token.go`):

| Token Type | Example | Preserved in pattern? |
|---|---|---|
| `Word` | `Starting`, `server` | Yes (constant words) |
| `HTTPMethod` | `GET`, `POST` | Yes |
| `HTTPStatusCode` | `200`, `503` | Wildcarded if variable across cluster |
| `Severity` | `ERROR`, `WARN` | Yes |
| `IPv4Address` | `10.0.0.1` | Wildcarded |
| `NumericValue` | `8080`, `4ms` | Wildcarded |
| `AbsolutePath` | `/api/v1/users` | Partially preserved |
| `URI` | `https://...` | Wildcarded |
| `Date`, `LocalTime` | `2026-04-29` | Wildcarded |
| `KeyValueSequence` | `key=value` | Structure preserved, values wildcarded |

**Why it matters:** This is the semantically rich representation. The tokenizer can
distinguish "GET request succeeded" from "POST request failed" from "connection timeout"
by reading the preserved words and token types. The Drain algorithm merges logs that
differ only in variable parts, so each pattern represents a **class of log events**.

**Default behavior:** Only processes warn+ severity logs. All severity levels are
processed by `log_metrics_extractor` above.

**Controlled by:** `observer.components.log_pattern_extractor.enabled` (default: true)

### `connection_error_extractor` (<1% of series)

**Source:** `ConnectionErrorExtractor`. Monitors for TCP connection errors in log content.

**What it contains:** `connection.errors` count metric.

**Why it matters:** Connection errors are a direct signal of network partition, service
unavailability, or DNS failure — common incident precursors.

### `all-metrics` (<1% of series)

**Source:** DogStatsD and APM pipelines. These are the standard agent metrics sent by
application workloads via `ddtrace` instrumentation and custom metrics.

**What it contains:** Whatever the workload emits — request counts, latency percentiles,
queue depths, custom gauges. Series names are application-defined (e.g. `flask.request`,
`redis.commands`, `memcached.connections`).

**Why it matters:** Application-level metrics are where many incidents first manifest.
A spike in `flask.request.errors` or a drop in `redis.commands` indicates workload-level
problems that system metrics may not yet reflect.

**Current limitation:** In our gs-flow collection setup, workload metrics are thin because
the agent runs as a standalone Deployment (not a DaemonSet), and workload pods must
actively send to it via DogStatsD/APM. Coverage depends on episode service instrumentation.

## What's NOT Captured

- **Raw log lines:** The JSONL contains pattern counts, not the logs themselves. The
  parquet files contain raw logs if needed for salience testing.
- **Trace spans:** APM traces are processed into metrics but individual spans are not
  recorded in the JSONL.
- **Observer telemetry:** Internal observer metrics (detection timing, storage stats)
  are excluded via `WorkloadSeriesFilter()` which filters out the `TelemetryNamespace`.
- **Container metrics:** Require the HF container check runner, which depends on kubelet
  access. Not currently available in the vcluster deployment.

## Value Aggregation

The storage holds summary statistics per series per timestamp bucket (sum, count, min,
max). The collector reads with `AggregateAverage` (sum/count), which is the natural
value for gauges. For counters (like log pattern counts where count=1 per observation),
average equals the raw value.

Other aggregations are available (`AggregateSum`, `AggregateCount`, `AggregateMin`,
`AggregateMax`) but average provides the best single representation across mixed metric
types.

## Detection Cadence

The collector runs on every engine `Advance()`, which fires when new data arrives.
With HF system checks enabled, data arrives every second, so the detection cadence
is **1 Hz** — one tick per second. Confirmed empirically: 59 ticks over 58 seconds
with exactly 1-second intervals.

| HF system checks | HF container checks | Cadence |
|---|---|---|
| Enabled | Enabled | 1 Hz |
| Enabled | Disabled | 1 Hz |
| Disabled | Disabled | ~0.03-0.07 Hz (DogStatsD-driven, 15-30s intervals) |

Each tick reads the last 300 seconds of data for each series, so there's overlap
between ticks. This is intentional — the model sees a sliding window, not disjoint
snapshots.

A full 33-minute gensim episode at 1 Hz produces ~1,980 ticks.

## File Format Reference

```
Line 1:  {"type": "header", "start_ts": "2026-...", "collector_version": "0.4"}
Line 2+: {"data_time": <unix_sec>, "series": [<series>...]}
```

Each series object:
```jsonc
{
  "ns": "<namespace>",          // always present
  "name": "<metric_name>",      // present for non-log metrics
  "pattern": "<pattern_text>",  // present for log metrics (Drain template or structural sig)
  "tags": ["key:value", ...],   // dimensional tags
  "points": [{"ts": <unix_sec>, "val": <float64>}, ...]
}
```

`name` and `pattern` are mutually exclusive — exactly one is present per series.

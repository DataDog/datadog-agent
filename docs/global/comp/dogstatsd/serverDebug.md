> **TL;DR:** A runtime debug facility for the DogStatsD server that tracks per-metric statistics, detects volume spikes via a rolling 5-second bucket, and exposes the results as JSON — all toggled at runtime without an agent restart.

# comp/dogstatsd/serverDebug

**Team:** agent-metric-pipelines

## Purpose

This component provides a runtime debug facility for the DogStatsD server. When enabled, it tracks per-metric statistics (name, tags, receive count, last-seen time) and exposes them as JSON. It also detects metric-volume spikes by comparing the current-second bucket against the sum of the preceding four seconds, logging a warning when an anomaly is found.

The debug mode is off by default and can be toggled at runtime without restarting the agent, making it useful for diagnosing misbehaving StatsD clients in production.

## Key elements

### Key interfaces

`comp/dogstatsd/serverDebug/component.go`

```go
type Component interface {
    StoreMetricStats(sample metrics.MetricSample)
    IsDebugEnabled() bool
    SetMetricStatsEnabled(bool)
    GetJSONDebugStats() ([]byte, error)
}
```

| Method | Description |
|--------|-------------|
| `StoreMetricStats` | Records a single metric sample. No-ops when debug is disabled. Called on every packet the DogStatsD server processes. |
| `IsDebugEnabled` | Returns the current enabled state. |
| `SetMetricStatsEnabled` | Toggles debug mode. Enabling starts a background goroutine that maintains rolling 1-second count buckets. Disabling stops it. |
| `GetJSONDebugStats` | Returns JSON-marshalled stats. Keys are `ckey.ContextKey` hashes; values are `metricStat` objects. |

### Key types

#### `metricStat`

```go
type metricStat struct {
    Name     string    `json:"name"`
    Count    uint64    `json:"count"`
    LastSeen time.Time `json:"last_seen"`
    Tags     string    `json:"tags"`
}
```

#### `metricsCountBuckets`

A ring buffer of five `uint64` counters, one per second, used for spike detection. A spike is flagged when the current bucket exceeds the sum of all other four buckets.

### Key functions

#### Spike detection and logging

The `enableMetricsStats` path starts a ticker at 100 ms. On each second boundary it rotates the ring buffer and calls `hasSpike()`. If a spike is detected, a `Warnf` is emitted via the component logger.

#### Optional DogStatsD file logger

If `dogstatsd_logging_enabled` is `true` in the agent config, the component writes every recorded metric to a dedicated log file (`dogstatsd_log_file`, defaulting to `defaultpaths.DogstatsDLogFile`). This is separate from the main agent log.

#### Serverless constructor

`NewServerlessServerDebug()` creates an instance without an fx dependency graph, using a temporary logger and the global config reader. This is used by the serverless agent.

#### `FormatDebugStats` (helper)

`serverdebugimpl.FormatDebugStats(stats []byte) (string, error)` parses the JSON produced by `GetJSONDebugStats` and returns a human-readable tabular string sorted by descending count. Used by the `agent dogstatsd-stats` CLI subcommand.

### Configuration and build flags

| Key | Default | Description |
|---|---|---|
| `dogstatsd_metrics_stats_enable` | `false` | Enable per-metric debug statistics at startup |
| `dogstatsd_logging_enabled` | `false` | Write every recorded metric to a dedicated log file |
| `dogstatsd_log_file` | `defaultpaths.DogstatsDLogFile` | Path to the DogStatsD-specific log file |

## Usage

### Wire-up

The component is included in the DogStatsD bundle (`comp/dogstatsd/bundle.go`) via `serverdebugimpl.Module()`. It is consumed by:

- `comp/dogstatsd/server` — calls `StoreMetricStats` on every processed sample and checks `IsDebugEnabled` before doing so.
- `cmd/agent/subcommands/dogstatsdstats` — calls `GetJSONDebugStats`, formats the result with `FormatDebugStats`, and prints it.
- `cmd/agent/subcommands/run` runtime settings — exposes `SetMetricStatsEnabled` through the agent's runtime settings API so operators can toggle debug mode without a restart.

### Enabling at runtime

```bash
# Enable via runtime setting
datadog-agent config set dogstatsd_metrics_stats_enable true

# Read current stats
datadog-agent dogstatsd-stats
```

Alternatively, set `dogstatsd_metrics_stats_enable: true` in `datadog.yaml` to enable at startup.

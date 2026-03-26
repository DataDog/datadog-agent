> **TL;DR:** `pkg/security/metrics` is the central registry of all CWS statsd metric name constants and tag constants, ensuring a single source of truth for metric naming across every `pkg/security` sub-package.

# pkg/security/metrics

## Purpose

Central registry of all CWS (Cloud Workload Security) metric name constants and shared tag constants. No logic lives here — every other package in `pkg/security/` imports these names when emitting metrics, ensuring a single source of truth for metric naming.

## Key elements

### Key types

#### `ITMetric`

`ITMetric` is a struct (`Subsystem`, `Name`) used with Prometheus-style internal telemetry (`pkg/telemetry`). Created by `newITRuntimeMetric(subsystem, name)`. Helper constructors `NewITCounter` and `NewITGauge` wrap `telemetry.NewCounter` / `telemetry.NewGauge`.

### Key functions

#### Metric prefixes

| Variable | Value |
|----------|-------|
| `MetricRuntimePrefix` | `"datadog.runtime_security"` — prefix for metrics emitted by system-probe (the kernel-side module). |
| `MetricAgentPrefix` | `"datadog.security_agent"` — prefix for metrics emitted by the security-agent process. |

Metric name variables are constructed by `newRuntimeMetric(suffix)` or `newAgentMetric(suffix)` which concatenate the appropriate prefix with the suffix.

#### Metric groupings (selected)

| Group | Example constants | Typical tags |
|-------|-------------------|--------------|
| Event server | `MetricEventServerExpired` | `rule_id` |
| Rate limiter | `MetricRateLimiterDrop`, `MetricRateLimiterAllow` | `rule_id` |
| Dentry resolver | `MetricDentryResolverHits`, `MetricDentryResolverMiss`, `MetricDentryERPC` | `cache`, `kernel_maps` |
| DNS resolver | `MetricDNSResolverIPResolverCache`, `MetricDNSResolverCnameResolverCache` | `hit`, `miss`, `insertion`, `eviction` |
| Perf buffer | `MetricPerfBufferLostWrite`, `MetricPerfBufferEventsRead`, `MetricPerfBufferBytesInUse` | `map`, `event_type` |
| Process resolver | `MetricProcessResolverCacheSize`, `MetricProcessResolverHits`, `MetricProcessResolverReparentSuccess` | `type`, `callpath` |
| Mount resolver | `MetricMountResolverCacheSize`, `MetricMountResolverHits` | `cache`, `procfs` |
| Activity dump | `MetricActivityDumpEventProcessed`, `MetricActivityDumpSizeInBytes`, `MetricActivityDumpActiveDumps` | `event_type`, `format`, `storage_type` |
| Security profile | `MetricSecurityProfileProfiles`, `MetricSecurityProfileEventFiltering` | `in_kernel`, `anomaly_detection`, `profile_state` |
| Security profile v2 | `MetricSecurityProfileV2EventsReceived`, `MetricSecurityProfileV2TagResolutionLatency` | `source` |
| Hash resolver | `MetricHashResolverHashCount`, `MetricHashResolverHashMiss` (ITMetric) | `event_type`, `hash` |
| Enforcement | `MetricEnforcementProcessKilled`, `MetricEnforcementRuleDisarmed` | `rule_id`, `disarmer_type` |
| Security agent | `MetricSecurityAgentRuntimeRunning`, `MetricSecurityAgentRuntimeContainersRunning` | — |
| Windows-specific | `MetricWindowsETWEventsLost`, `MetricWindowsFileResolverNew`, etc. | — |

### Configuration and build flags

#### Tag constants

Pre-built tag strings used when emitting the metrics above:

- `CacheTag`, `KernelMapsTag`, `ProcFSTag`, `ERPCTag` (and `AllTypesTags` slice)
- `SegmentResolutionTag`, `ParentResolutionTag`, `PathResolutionTag`
- `ProcessSourceEventTags`, `ProcessSourceKernelMapsTags`, `ProcessSourceProcTags`
- `ReparentCallpathSetProcessContext`, `ReparentCallpathDoExit`, etc.

#### Windows-specific metrics (`metrics_windows.go`)

Compiled only on Windows. Covers ETW buffer stats, file/registry resolver sizes, process start/stop notifications, and approver rejects. All use the `datadog.runtime_security.windows.*` namespace.

## Usage

Import the package and reference the constants directly:

```go
import "github.com/DataDog/datadog-agent/pkg/security/metrics"

statsdClient.Count(metrics.MetricRateLimiterDrop, 1, []string{"rule_id:" + ruleID}, 1.0)
```

For `ITMetric` counters, call the constructor helpers:

```go
counter := metrics.NewITCounter(metrics.MetricHashResolverHashCount, []string{"event_type", "hash"}, "help text")
counter.Inc(tags...)
```

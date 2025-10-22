# Service Statistics Collection in Doctor Component

This document describes how the doctor component collects and aggregates telemetry data by service across different agent pipelines.

## Overview

The doctor component aggregates metrics, logs, and traces by service name to provide a unified view of agent activity. Each pipeline (checks, DogStatsD, logs, traces) has different data sources and collection strategies.

## Data Model

```go
type ServiceStats struct {
    Name        string  `json:"name"`         // Service name
    TracesRate  float64 `json:"traces_rate"`  // Traces per second
    MetricsRate float64 `json:"metrics_rate"` // Metrics per second
    LogsRate    float64 `json:"logs_rate"`    // Logs (bytes) per second
}
```

## Collection Strategies by Pipeline

### 1. Check Metrics

**Source**: Agent checks (Python and Go corechecks)

**Location**: `comp/doctor/doctorimpl/aggregator.go:collectMetricsServiceStats()`

**Service Attribution Strategy**:

```
Priority 1: Explicit service: tag in check instance config
   └─> Parse YAML config via collector.Component
   └─> Extract "service:name" from tags array

Priority 2: Check loader type
   └─> Corechecks (loader == "core") → "datadog-agent"
   └─> Other checks without service tag → "" (empty string)
```

**Rate Calculation**: ✅ **Instantaneous**
- Uses `MetricSamples` (last run count) divided by `Interval`
- Formula: `metricsPerSecond = MetricSamples / Interval.Seconds()`
- True instantaneous rate per check run

**Example**:
```yaml
# Check instance config
instances:
  - url: http://localhost:6379
    tags:
      - service:redis  # ← Extracted here
      - env:prod
```

**Implementation Details**:
```go
// 1. Get collector component to access check instances
coll, collectorAvailable := d.collector.Get()

// 2. For each check with metrics
for checkID, stats := range checkStatsMap {
    // Get check instance
    check := findCheckByID(checkID)

    // 3. Determine service name
    if collectorAvailable {
        checkLoader = check.Loader()
        serviceName = extractServiceFromInstanceConfig(check.InstanceConfig())
    }

    // 4. Default based on loader type
    if serviceName == "" && checkLoader == "core" {
        serviceName = "datadog-agent"  // Corechecks
    }

    // 5. Calculate instantaneous rate
    metricsPerSecond = MetricSamples / Interval.Seconds()
}
```

**Dependencies**:
- `collector.Component` (optional) - for config parsing
- `expvars.GetCheckStats()` - for check statistics

---

### 2. DogStatsD Metrics

**Source**: Metrics sent via DogStatsD protocol

**Location**: `comp/doctor/doctorimpl/aggregator.go:collectDogStatsDServiceStats()`

**Service Attribution Strategy**:

```
Extraction Point: Time sampler hot path
   └─> Extract "service:" tag from metric sample tags
   └─> Increment per-service counter atomically
   └─> Aggregate across all sharded time samplers on flush
```

**Rate Calculation**: ✅ **Instantaneous** (Delta-based)
- Tracks previous sample counts per service
- Calculates delta between collections
- Formula: `metricsPerSecond = (currentCount - previousCount) / timeDelta`
- Falls back to average-since-start on first collection

**Data Flow**:
```
Metric Sample → TimeSampler.sample()
                    ↓
              serviceStats.trackSample()
                    ↓
              Extract service: tag
                    ↓
              Increment counter (atomic)
                    ↓
           [On Flush] Aggregate all shards
                    ↓
              Export to expvar
                    ↓
              Doctor reads & calculates delta
```

**Implementation Details**:
```go
// 1. Time sampler tracks on every metric sample
func (s *TimeSampler) sample(metricSample *metrics.MetricSample, timestamp float64) {
    s.serviceStats.trackSample(metricSample)  // ← Extraction happens here
    // ... rest of sampling logic
}

// 2. Service stats tracker extracts tag
func (s *serviceStatsTracker) trackSample(sample *metrics.MetricSample) {
    serviceName := extractServiceTag(sample.Tags)  // Find "service:xxx"
    s.incrementService(serviceName)                // Atomic increment
}

// 3. On flush, demultiplexer aggregates all shards
func (d *AgentDemultiplexer) updateServiceStats() {
    for _, worker := range d.statsd.workers {
        worker.sampler.serviceStats.exportToExpvar(&aggregatorDogstatsdServiceStats)
    }
}

// 4. Doctor calculates delta-based rate
now := time.Now()
timeDelta := now.Sub(d.lastDogstatsdCollectionTime).Seconds()

for serviceName, currentCount := range serviceStatsMap {
    previousCount := d.previousDogstatsdSamples[serviceName]
    deltaCount := currentCount - previousCount
    metricsRate := float64(deltaCount) / timeDelta

    d.previousDogstatsdSamples[serviceName] = currentCount
}
```

**Performance**:
- Hot path overhead: ~1-2% (one map lookup + atomic increment per metric)
- Memory: O(unique service tags) - typically 10-100 entries
- Thread-safe with RWMutex

**Dependencies**:
- `aggregatorDogstatsdServiceStats` expvar
- Delta tracking state in doctorImpl

---

### 3. Logs

**Source**: Log collection agent

**Location**: `comp/doctor/doctorimpl/aggregator.go:collectLogsServiceStats()`

**Service Attribution Strategy**:

```
Source: Log source configuration
   └─> source.Configuration["Service"] field
   └─> Explicitly configured by user in logs YAML
   └─> Sources without service tag grouped under ""
```

**Rate Calculation**: ✅ **Instantaneous** (Delta-based)
- Tracks previous bytes read per service
- Calculates delta between collections
- Formula: `bytesPerSecond = (currentBytes - previousBytes) / timeDelta`
- Falls back to average-since-start on first collection

**Example Configuration**:
```yaml
# logs config
logs:
  - type: file
    path: /var/log/nginx/*.log
    service: nginx      # ← Extracted here
    source: nginx
```

**Implementation Details**:
```go
// 1. Get logs agent status
logsAgentStatus := logsstatus.Get(true)

// 2. Lock for thread-safe delta tracking
d.logsDeltaMu.Lock()
defer d.logsDeltaMu.Unlock()

// 3. Calculate time delta
now := time.Now()
timeDelta := now.Sub(d.lastLogsCollectionTime).Seconds()

// 4. Collect current bytes read per service
currentBytesRead := make(map[string]int64)
for _, integration := range logsAgentStatus.Integrations {
    for _, source := range integration.Sources {
        // Extract service from config
        serviceName := source.Configuration["Service"].(string)

        // Parse bytes read
        if bytesInfo, ok := source.Info["Bytes Read"]; ok {
            var bytesRead int64
            fmt.Sscanf(bytesInfo[0], "%d", &bytesRead)
            currentBytesRead[serviceName] += bytesRead
        }
    }
}

// 5. Calculate instantaneous rates using deltas
for serviceName, currentBytes := range currentBytesRead {
    previousBytes := d.previousLogsBytesRead[serviceName]

    if currentBytes >= previousBytes {
        deltaBytes := currentBytes - previousBytes
        bytesPerSecond := float64(deltaBytes) / timeDelta
    } else {
        // Counter reset - use average since start
        bytesPerSecond := float64(currentBytes) / uptimeSeconds
    }

    d.previousLogsBytesRead[serviceName] = currentBytes
}
```

**Dependencies**:
- `logsstatus.Get()` - logs agent status
- Delta tracking state in doctorImpl

---

### 4. Traces

**Source**: Trace agent receiver

**Location**: `comp/doctor/doctorimpl/aggregator.go:collectTraceServiceStats()`

**Service Attribution Strategy**:

```
Source: Trace receiver expvars
   └─> receiver expvar contains per-service stats
   └─> Tags["Service"] field in receiver stats
```

**Rate Calculation**: ✅ **Instantaneous** (Delta-based)
- Tracks previous traces received per service
- Calculates delta between collections
- Formula: `tracesPerSecond = (currentTraces - previousTraces) / timeDelta`
- Falls back to average-since-start on first collection
- Aggregates across multiple TagStats per service

**Implementation Details**:
```go
// 1. Fetch from trace-agent HTTP endpoint
traceAgentPort := d.config.GetInt("apm_config.debug.port")  // Default: 5012
url := fmt.Sprintf("https://localhost:%d/debug/vars", traceAgentPort)
resp, err := d.httpclient.Get(url)

// 2. Parse expvar JSON and extract "receiver" array
var expvarData map[string]interface{}
json.Unmarshal(resp, &expvarData)
receiverInterface := expvarData["receiver"]

// 3. Lock for thread-safe delta tracking
d.tracesDeltaMu.Lock()
defer d.tracesDeltaMu.Unlock()

// 4. Calculate time delta
now := time.Now()
timeDelta := now.Sub(d.lastTracesCollectionTime).Seconds()

// 5. Aggregate traces across all entries for same service
// (Same service can have multiple entries with different Lang/TracerVersion)
currentTracesReceived := make(map[string]int64)

for _, tagStats := range receiverStats {
    // Service and TracesReceived are at top level (not nested)
    serviceName, hasService := tagStats["Service"].(string)
    if !hasService || serviceName == "" {
        continue
    }

    tracesReceived, ok := tagStats["TracesReceived"].(float64)
    if !ok {
        continue
    }

    currentTracesReceived[serviceName] += int64(tracesReceived)
}

// 4. Calculate instantaneous rates using deltas
for serviceName, currentTraces := range currentTracesReceived {
    previousTraces := d.previousTracesReceived[serviceName]

    if currentTraces >= previousTraces {
        deltaTraces := currentTraces - previousTraces
        tracesRate := float64(deltaTraces) / timeDelta
    } else {
        // Counter reset - fallback to average since start
        tracesRate := float64(currentTraces) / uptimeSeconds
    }

    d.previousTracesReceived[serviceName] = currentTraces
}
```

**Receiver JSON Structure**:
The "receiver" expvar returns an array where each entry contains flattened fields:
```json
[
    {
        "Service": "python-flask-app",         ← Service name at top level
        "Lang": "python",
        "LangVersion": "3.11.14",
        "TracerVersion": "2.15.0",
        "TracesReceived": 13,                  ← Traces count at top level
        "SpansReceived": 136,
        "TracesBytes": 17625,
        "TracesDropped": { ... },
        ...
    }
]
```

**Key Implementation Details**:
- Fields are at **top level**, not nested under "Tags" or "Stats" keys
- Aggregates multiple entries per service (different tracers/languages)
- Fetches via HTTP from separate trace-agent process
- Handles counter resets gracefully
- Thread-safe with mutex protection
- First collection uses average-since-start as fallback

**Dependencies**:
- Trace-agent HTTP endpoint: `http://localhost:{apm_config.debug.port}/debug/vars` (default port 5012)
- Fetches `receiver` expvar via HTTP from separate trace-agent process

---

## Rate Calculation Summary

| Pipeline | Method | Accuracy | Formula |
|----------|--------|----------|---------|
| **Check Metrics** | Per-interval | ✅ Instantaneous | `MetricSamples / Interval.Seconds()` |
| **DogStatsD** | Delta-based | ✅ Instantaneous | `(current - previous) / timeDelta` |
| **Logs** | Delta-based | ✅ Instantaneous | `(currentBytes - previousBytes) / timeDelta` |
| **Traces** | Delta-based | ✅ Instantaneous | `(currentTraces - previousTraces) / timeDelta` |

## Delta Tracking Pattern

All delta-based calculations follow this pattern:

```go
// State in doctorImpl
type doctorImpl struct {
    // ...
    deltaMu            sync.Mutex
    previousCounts     map[string]int64
    lastCollectionTime time.Time
}

// Collection function
func (d *doctorImpl) collectXxxServiceStats(serviceMap map[string]*def.ServiceStats) {
    // 1. Lock for thread safety
    d.deltaMu.Lock()
    defer d.deltaMu.Unlock()

    // 2. Calculate time delta
    now := time.Now()
    timeDelta := now.Sub(d.lastCollectionTime).Seconds()

    // 3. First collection check
    if d.lastCollectionTime.IsZero() || timeDelta < 0.1 {
        // Use average since start as fallback
        rate := float64(currentCount) / uptimeSeconds
        d.previousCounts[serviceName] = currentCount
        d.lastCollectionTime = now
        return
    }

    // 4. Calculate instantaneous rate
    for serviceName, currentCount := range currentCounts {
        previousCount := d.previousCounts[serviceName]

        if currentCount >= previousCount {
            // Normal case: calculate delta
            delta := currentCount - previousCount
            rate := float64(delta) / timeDelta
        } else {
            // Counter reset: fallback to average
            rate := float64(currentCount) / uptimeSeconds
        }

        d.previousCounts[serviceName] = currentCount
    }

    // 5. Update collection time
    d.lastCollectionTime = now
}
```

## Service Name Edge Cases

### Empty Service Name (`""`)

**Meaning**: Telemetry without explicit service attribution

**Sources**:
- Logs: Sources without `service:` in config
- DogStatsD: Metrics without `service:` tag
- Checks: Non-core checks without service tag

**Display**: Can be labeled as "untagged" or "default" in TUI

### "datadog-agent" Service

**Meaning**: Agent self-monitoring (corechecks)

**Sources**:
- CPU, memory, disk, network corechecks
- Agent health metrics
- System-level monitoring

**Identification**: Checks with `loader == "core"` and no explicit service tag

### Multiple Pipelines Per Service

**Example**: Service "nginx" may have:
- Check metrics: 45/s (from nginx integration check)
- DogStatsD metrics: 120/s (from application instrumentation)
- Logs: 2.3MB/s (from nginx access logs)
- Traces: 30/s (from APM)

**Aggregation**: All are summed into `serviceMap[serviceName].MetricsRate`

## Debugging and Monitoring

### Check Expvars

```bash
# Check stats from checks
curl localhost:5000/debug/vars | jq '.runner.Checks'

# DogStatsD service stats
curl localhost:5000/debug/vars | jq '.aggregator.DogstatsdServiceStats'

# Logs status
curl localhost:5000/debug/vars | jq '.logs'

# Trace receiver stats
curl localhost:5000/debug/vars | jq '.receiver'
```

### Verify Service Attribution

```bash
# Check instance configs
agent configcheck | grep -A 10 "service:"

# Log source configs
agent status | grep -A 5 "Logs Agent"

# DogStatsD metrics with service tag
echo "my.metric:1|c|#service:myapp" | nc -u localhost 8125
```

## Performance Considerations

### Hot Path Impact

- **Check Metrics**: Minimal (reads from expvars, no hot path)
- **DogStatsD**: ~1-2% overhead per metric (map lookup + atomic increment)
- **Logs**: Minimal (reads from logs status, no hot path)
- **Traces**: Minimal (reads from expvars, no hot path)

### Memory Usage

- **Check Metrics**: O(number of check instances) - typically 10-50
- **DogStatsD**: O(unique service tags) - typically 10-100
- **Logs**: O(log sources) - typically 5-30
- **Traces**: O(services sending traces) - typically 10-50

### Refresh Rate

Doctor component polls stats every N seconds (configurable):
- Default: Every 2 seconds
- Delta calculations benefit from stable polling intervals
- Too frequent: noisy rates
- Too infrequent: delayed updates

## Future Improvements

1. **Traces Rate Calculation**: Implement delta tracking for instantaneous trace rates
2. **Metric Type Breakdown**: Track gauge/counter/histogram separately
3. **Error Rates**: Track errors per service per pipeline
4. **Cardinality Tracking**: Monitor tag cardinality per service
5. **Custom Aggregations**: Allow grouping by env, team, etc.

## References

- Check stats: `pkg/collector/runner/expvars/expvars.go`
- DogStatsD service tracking: `pkg/aggregator/service_stats.go`
- Logs status: `pkg/logs/status/status.go`
- Trace receiver: Trace agent expvars

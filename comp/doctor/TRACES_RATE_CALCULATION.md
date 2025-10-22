# Trace Rate Calculation for Doctor Component

## Current State

The doctor component currently collects **cumulative** trace counts from the trace-agent, not instantaneous rates.

### Current Implementation

**Location**: `comp/doctor/doctorimpl/aggregator.go:collectTraceServiceStats()`

**Data Source**: `receiver` expvar from trace-agent
- Structure: Array of `TagStats` objects
- Each TagStats has:
  - `Tags.Service`: Service name
  - `Stats.TracesReceived`: **Atomic cumulative counter** since agent start

**Problem**:
```go
// Current code (line 355)
serviceMap[serviceName].TracesRate += tracesReceived  // ← This is cumulative, not a rate!
```

This stores the total traces received since the trace-agent started, not traces per second.

---

## Solution: Delta-Based Rate Calculation

Implement the same delta tracking pattern used for logs and DogStatsD.

### Data Flow

```
Trace Agent (receiver) → Expvar (cumulative counters per service)
                              ↓
            Doctor reads expvar every N seconds
                              ↓
            Calculate delta from previous reading
                              ↓
            Rate = (current - previous) / timeDelta
                              ↓
            Store in ServiceStats.TracesRate
```

### Implementation Strategy

#### 1. Add Delta Tracking State to `doctorImpl`

```go
// In comp/doctor/doctorimpl/doctor.go

type doctorImpl struct {
    // ... existing fields ...

    // Delta tracking for traces rate calculation
    tracesDeltaMu            sync.Mutex
    previousTracesReceived   map[string]int64 // Map of service name to traces received
    lastTracesCollectionTime time.Time
}

func newDoctor(deps dependencies) *doctorImpl {
    d := &doctorImpl{
        // ... existing initialization ...
        previousTracesReceived:   make(map[string]int64),
        lastTracesCollectionTime: time.Now(),
    }
    return d
}
```

#### 2. Modify `collectTraceServiceStats()` with Delta Calculation

```go
// In comp/doctor/doctorimpl/aggregator.go

func (d *doctorImpl) collectTraceServiceStats(serviceMap map[string]*def.ServiceStats) {
    // Get trace receiver stats from expvars
    receiverVar := expvar.Get("receiver")
    if receiverVar == nil {
        return
    }

    receiverJSON := []byte(receiverVar.String())
    var receiverStats []map[string]interface{}
    if err := json.Unmarshal(receiverJSON, &receiverStats); err != nil {
        d.log.Debugf("Failed to unmarshal trace receiver stats: %v", err)
        return
    }

    // Lock for thread-safe access to delta tracking state
    d.tracesDeltaMu.Lock()
    defer d.tracesDeltaMu.Unlock()

    // Calculate time delta since last collection
    now := time.Now()
    timeDelta := now.Sub(d.lastTracesCollectionTime).Seconds()

    // Collect current traces received per service
    currentTracesReceived := make(map[string]int64)

    for _, tagStats := range receiverStats {
        if tags, ok := tagStats["Tags"].(map[string]interface{}); ok {
            if serviceName, ok := tags["Service"].(string); ok && serviceName != "" {
                if stats, ok := tagStats["Stats"].(map[string]interface{}); ok {
                    if tracesReceived, ok := stats["TracesReceived"].(float64); ok {
                        currentTracesReceived[serviceName] += int64(tracesReceived)
                    }
                }
            }
        }
    }

    // First collection or time delta too small - use average rate
    if d.lastTracesCollectionTime.IsZero() || timeDelta < 0.1 {
        uptimeSeconds := time.Since(d.startTime).Seconds()
        if uptimeSeconds == 0 {
            uptimeSeconds = 1
        }

        for serviceName, currentTraces := range currentTracesReceived {
            if currentTraces > 0 {
                if _, exists := serviceMap[serviceName]; !exists {
                    serviceMap[serviceName] = &def.ServiceStats{Name: serviceName}
                }
                tracesRate := float64(currentTraces) / uptimeSeconds
                serviceMap[serviceName].TracesRate += tracesRate

                // Store for next iteration
                d.previousTracesReceived[serviceName] = currentTraces
            }
        }

        d.lastTracesCollectionTime = now
        return
    }

    // Calculate instantaneous rates using deltas
    for serviceName, currentTraces := range currentTracesReceived {
        previousTraces, hasPrevious := d.previousTracesReceived[serviceName]

        var tracesRate float64
        if hasPrevious && currentTraces >= previousTraces {
            // Calculate instantaneous rate from delta
            deltaTraces := currentTraces - previousTraces
            tracesRate = float64(deltaTraces) / timeDelta
        } else {
            // No previous data or counter reset - fallback to average
            uptimeSeconds := time.Since(d.startTime).Seconds()
            if uptimeSeconds > 0 {
                tracesRate = float64(currentTraces) / uptimeSeconds
            }
        }

        // Get or create service entry
        if _, exists := serviceMap[serviceName]; !exists {
            serviceMap[serviceName] = &def.ServiceStats{Name: serviceName}
        }
        serviceMap[serviceName].TracesRate += tracesRate

        // Update tracking state
        d.previousTracesReceived[serviceName] = currentTraces
    }

    // Update last collection time
    d.lastTracesCollectionTime = now
}
```

---

## Understanding Trace Agent Stats

### Trace Receiver Structure

The trace-agent exposes stats via the `receiver` expvar:

```json
{
  "receiver": [
    {
      "Tags": {
        "Lang": "go",
        "LangVersion": "1.21",
        "Interpreter": "",
        "TracerVersion": "1.56.0",
        "EndpointVersion": "v0.4",
        "Service": "my-api"  ← Service name here
      },
      "Stats": {
        "TracesReceived": 12543,      ← Cumulative count
        "TracesFiltered": 120,
        "SpansReceived": 45782,
        "SpansDropped": 23,
        "EventsExtracted": 340,
        "PayloadAccepted": 1024
      }
    },
    {
      "Tags": {
        "Service": "postgres"
      },
      "Stats": {
        "TracesReceived": 8932,
        ...
      }
    }
  ]
}
```

### Key Points

1. **Multiple TagStats per Service**: The receiver can have multiple TagStats entries for the same service if they differ by language/version/tracer
2. **Need to Aggregate**: Sum all `TracesReceived` counters for the same service name
3. **Atomic Counters**: `TracesReceived` is atomic and only increases (or resets on restart)

### Example Aggregation

```
Service: "my-api"
  - From Go tracer v1.56:   5,000 traces
  - From Python tracer v2.3: 3,000 traces
  - Total:                   8,000 traces

Service: "postgres"
  - From Go tracer:          2,500 traces
```

---

## Rate Calculation Edge Cases

### Case 1: First Collection

**Scenario**: Doctor component starts and reads trace stats for the first time.

**Solution**: Use average rate since agent start as fallback.

```go
if d.lastTracesCollectionTime.IsZero() {
    uptimeSeconds := time.Since(d.startTime).Seconds()
    tracesRate := float64(currentTraces) / uptimeSeconds
}
```

### Case 2: Counter Reset

**Scenario**: Trace-agent restarts, counters reset to 0.

**Detection**: `currentTraces < previousTraces`

**Solution**: Fall back to average-since-start rate.

```go
if currentTraces < previousTraces {
    // Counter reset detected
    uptimeSeconds := time.Since(d.startTime).Seconds()
    tracesRate := float64(currentTraces) / uptimeSeconds
}
```

### Case 3: New Service Appears

**Scenario**: A new service starts sending traces mid-flight.

**Handling**: No previous data exists for this service.

```go
previousTraces, hasPrevious := d.previousTracesReceived[serviceName]
if !hasPrevious {
    // New service - use current rate
    tracesRate := float64(currentTraces) / uptimeSeconds
}
```

### Case 4: Service Stops Sending Traces

**Scenario**: A service stops sending traces but still appears in expvar with the same count.

**Result**: Delta = 0, rate = 0/s (correct behavior)

```go
deltaTraces := currentTraces - previousTraces  // = 0
tracesRate := float64(deltaTraces) / timeDelta  // = 0
```

---

## Comparison with Other Pipelines

### Summary Table

| Pipeline | Counter Type | Data Source | Aggregation Needed | Special Handling |
|----------|--------------|-------------|-------------------|------------------|
| **Checks** | Per-interval | Check runner expvars | No | Loader-based service detection |
| **DogStatsD** | Cumulative | Aggregator expvars (added by us) | Yes (across shards) | Tag extraction on hot path |
| **Logs** | Cumulative | Logs status API | No | Bytes vs count metric |
| **Traces** | Cumulative | Trace receiver expvars | Yes (across Tag combinations) | Multiple TagStats per service |

### Why Traces Need Special Aggregation

Unlike other pipelines:
- **Multiple entries per service**: Same service can have different Lang/TracerVersion combinations
- **Need to sum**: Must aggregate `TracesReceived` across all TagStats for the same service
- **Atomic counters**: Similar to logs, uses atomic.Int64 counters

---

## Testing Strategy

### Unit Test Scenarios

1. **First Collection**
   - Input: No previous data
   - Expected: Uses average rate since start

2. **Steady State**
   - Input: Previous data exists, normal delta
   - Expected: Calculates rate = delta / timeDelta

3. **Counter Reset**
   - Input: currentTraces < previousTraces
   - Expected: Falls back to average rate

4. **Multiple TagStats per Service**
   - Input: Two TagStats both with Service="api"
   - Expected: Correctly sums both traces

5. **Zero Activity**
   - Input: No new traces (delta = 0)
   - Expected: Rate = 0/s

### Integration Test

```bash
# 1. Start trace-agent
# 2. Send traces from test service
# 3. Read doctor stats multiple times
# 4. Verify rate calculation matches expected

# Example:
# T=0: 0 traces
# T=10: 100 traces → rate = 10 traces/s
# T=20: 250 traces → rate = (250-100)/10 = 15 traces/s
```

---

## Performance Considerations

### Memory

- **Previous state**: O(unique services sending traces) - typically 10-100 services
- **Minimal overhead**: One int64 per service

### CPU

- **Read expvar**: JSON unmarshaling (already happening)
- **Delta calculation**: O(services) simple arithmetic
- **No hot path impact**: Runs in doctor collection loop, not trace ingestion

### Concurrency

- **Mutex protected**: Thread-safe access to delta state
- **Read-heavy**: Most operations are reads (current implementation), writes only on collection
- **No contention**: Doctor runs on separate goroutine from trace ingestion

---

## Alternative Approaches Considered

### Alternative 1: Trace Agent Exposes Rates Directly

**Pros**: No delta tracking needed in doctor
**Cons**: Requires modifying trace-agent, more complex

**Decision**: ❌ Not chosen - delta tracking is simpler and follows established pattern

### Alternative 2: Use DogStatsD Metrics Instead

**Pros**: Trace-agent already publishes `datadog.trace_agent.receiver.traces_received` to DogStatsD
**Cons**: Would require accessing DogStatsD pipeline, circular dependency

**Decision**: ❌ Not chosen - adds complexity, breaks separation of concerns

### Alternative 3: Approximate with Sampling

**Pros**: Could estimate rate from sample of traces
**Cons**: Inaccurate, doesn't match actual received count

**Decision**: ❌ Not chosen - accuracy is important for troubleshooting

---

## Expected Results

After implementation, the TUI should show:

```
Services (by activity):
  my-api           M: 450/s   L: 2.3MB/s  T: 35/s   ← Now showing traces per second!
  postgres         M: 230/s   L: 1.1MB/s  T: 12/s
  redis            M: 180/s                T: 8/s
  datadog-agent    M: 85/s                           ← Corechecks only
  nginx            M: 120/s   L: 500KB/s            ← DogStatsD + Logs
```

---

## Future Enhancements

1. **Span Rate**: Track `SpansReceived` in addition to traces
2. **Trace Size**: Calculate average trace size (spans/trace)
3. **Error Rate**: Track `TracesDropped` and `SpansDropped` per service
4. **Sampling Rate**: Display sampling percentage per service
5. **Language Breakdown**: Show which languages/tracers are used per service

---

## References

- Trace agent stats: `pkg/trace/info/stats.go`
- Receiver stats structure: `pkg/trace/info/stats.go:380-420`
- Tags structure: `pkg/trace/info/stats.go:516-551`
- Expvar publishing: `pkg/trace/info/info.go:355`

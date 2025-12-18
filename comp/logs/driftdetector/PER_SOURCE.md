# Per-Source Drift Detection

## Overview

The drift detector now operates in **per-source mode**, creating and managing a separate drift detection pipeline for each unique log source. This provides source isolation and more accurate anomaly detection.

## Architecture

```
External Apps
    ├─ App A (nginx)
    ├─ App B (mysql)
    └─ App C (custom)
         ↓
    Logs Processor
         ↓
   [Source Router]
         ↓
  Drift Detector Manager
         ├─ Pipeline for "file:nginx"
         ├─ Pipeline for "file:mysql"
         └─ Pipeline for "file:custom"
              ↓
         Per-Source Alerts
```

## Source Key Format

Each log source is identified by a unique key:
```
{source_type}:{source_identifier}
```

**Examples**:
- `file:nginx` - File logs from nginx service
- `file:mysql` - File logs from mysql service
- `docker:my-container` - Docker container logs
- `journald:systemd` - Systemd journal logs
- `tcp:custom-app` - TCP listener for custom app

The source identifier is derived from (in order of preference):
1. `Config.Service` - Service name from log config
2. `Config.Source` - Source name from log config
3. `LogSource.Name` - Log source name

## Benefits

### 1. **Source Isolation**
Each source has its own DMD model:
- A noisy source won't pollute models for other sources
- Different log patterns per service are properly handled
- More accurate anomaly detection

### 2. **Source-Specific Alerts**
Alerts include source information:
```json
{
  "message": "Anomaly detected in log stream",
  "source": "file:nginx",
  "severity": "warning",
  "window_id": 42,
  "normalized_error": 2.1
}
```

### 3. **Dynamic Source Management**
- New sources are detected automatically
- Pipelines created on-demand
- Idle sources cleaned up automatically

## Resource Management

### Memory Usage

Each drift detector pipeline uses approximately **11 MB**:
- Window manager: ~1 MB
- Template extractor: ~2 MB (4 workers)
- Embedding client: ~5 MB
- DMD analyzer: ~900 KB (120 windows × 768 dims)
- Alert manager: ~100 KB
- Channels: ~2 MB

**Example scenarios**:
- 10 sources = ~110 MB
- 50 sources = ~550 MB
- 100 sources = ~1.1 GB

### Automatic Cleanup

The manager automatically removes idle pipelines to conserve resources:

**Configuration**:
```yaml
logs_config:
  drift_detection:
    enabled: true
    cleanup_interval: 5m   # Check for idle sources every 5 minutes
    max_idle_time: 30m     # Remove sources idle for 30 minutes
```

**Cleanup behavior**:
1. Manager tracks last access time per source
2. Every `cleanup_interval`, checks for idle sources
3. Sources idle longer than `max_idle_time` are stopped and removed
4. If the source becomes active again, a new pipeline is created automatically

### Goroutine Count

Each pipeline uses **10 goroutines**:
- 1 window manager
- 4 template extraction workers
- 1 embedding batcher
- 1 DMD queue manager
- 1 DMD analyzer
- 1 anomaly detector
- 1 alert manager

**Total goroutines**: `10 × number_of_active_sources`

## Configuration

### Basic Configuration

```yaml
logs_config:
  drift_detection:
    enabled: true
    embedding_url: "http://localhost:11434/api/embed"
    embedding_model: "embeddinggemma"

    # Per-source resource management
    cleanup_interval: 5m    # How often to check for idle sources
    max_idle_time: 30m      # Max idle time before cleanup

    # Pipeline settings (applied to all sources)
    window_size: 120s
    window_step: 60s
    entropy_threshold: 2.5
    warning_threshold: 2.0
    critical_threshold: 3.0
    dmd_time_delay: 5
    dmd_rank: 50
```

### Tuning for Large Deployments

For deployments with many sources (100+):

```yaml
logs_config:
  drift_detection:
    enabled: true

    # Aggressive cleanup to limit active detectors
    cleanup_interval: 2m    # Check more frequently
    max_idle_time: 10m      # Remove idle sources faster

    # Reduce per-pipeline memory usage
    window_size: 60s        # Shorter windows
    dmd_rank: 30            # Lower SVD rank
```

## Monitoring

### Statistics

Query drift detector statistics via the component:

```go
stats := driftDetector.GetStats()
// Returns:
// {
//   "enabled": true,
//   "active_detectors": 15,
//   "sources": ["file:nginx", "file:mysql", ...]
// }
```

### Prometheus Metrics

Per-source metrics are **not** separated by default to avoid metric cardinality explosion. All metrics are aggregated across sources.

To track per-source metrics, you would need to add source labels to the metrics (not currently implemented).

### Logs

The manager logs when sources are added/removed:

```
INFO | Creating drift detector for source: file:nginx (total detectors: 1)
INFO | Drift detector created for source: file:nginx (total detectors: 1)
...
INFO | Removing idle drift detector for source: file:nginx
INFO | Drift detector removed (remaining detectors: 0)
INFO | Cleaned up 1 idle drift detectors
```

## Performance Considerations

### CPU Usage

CPU usage scales linearly with active sources:
- 1 source: <5% CPU
- 10 sources: ~20-30% CPU
- 50 sources: Can exceed 100% on single core

**Recommendation**: Use multi-core systems for deployments with 20+ sources.

### Embedding Service Load

Each source creates separate embedding requests:
- Current: Batching across all sources
- Per-source: Batching per source (less efficient)

**Impact**: Embedding service may need to scale horizontally to handle N× the request volume.

**Mitigation**: Increase `batch_timeout` to accumulate more templates per request.

### Back-pressure

If a specific source generates logs faster than the pipeline can process:
- The input channel (10,000 capacity) will fill up
- Additional logs from that source are dropped
- Other sources are not affected

## Troubleshooting

### Too many active detectors

**Symptom**: Memory usage growing, many active sources
**Solution**:
1. Reduce `max_idle_time` to clean up faster
2. Reduce `cleanup_interval` to check more frequently
3. Consider if all sources need drift detection

### Source not being detected

**Symptom**: Logs processed but no drift detector created
**Solution**:
1. Check that `msg.Origin.LogSource` is not nil
2. Verify source has `Config.Service`, `Config.Source`, or `Name`
3. Check agent logs for "Creating drift detector for source: ..."

### Embedding service overload

**Symptom**: High latency, failed embedding requests
**Solution**:
1. Increase embedding service capacity
2. Increase `batch_timeout` for better batching efficiency
3. Consider reducing number of active sources

## Migration from Single Pipeline

The component maintains backward compatibility:

**Old API** (still works, routes to "default" source):
```go
driftDetector.ProcessLog(timestamp, content)
```

**New API** (recommended):
```go
driftDetector.ProcessLogWithSource(sourceKey, timestamp, content)
```

The processor automatically uses the new API, so no code changes are required for the standard integration.

## Scaling Recommendations

| Deployment | Sources | Recommendation |
|-----------|---------|----------------|
| **Small** | <20 | Default settings work well |
| **Medium** | 20-50 | Monitor memory, tune cleanup |
| **Large** | 50-100 | Aggressive cleanup, consider grouping |
| **Very Large** | >100 | May need architectural changes |

For very large deployments, consider:
1. **Sampling**: Only enable drift detection for critical services
2. **Grouping**: Group similar services into shared pipelines
3. **Distributed**: Run multiple agents, each handling subset of sources

## Future Enhancements

Potential improvements:
1. **Per-source thresholds**: Different `warning_threshold` per source
2. **Source priority**: Keep high-priority sources active, clean up low-priority
3. **Per-source metrics**: Add source label to Prometheus metrics (opt-in to avoid cardinality)
4. **Adaptive cleanup**: Adjust `max_idle_time` based on available memory
5. **Source groups**: Manually group sources to share pipelines

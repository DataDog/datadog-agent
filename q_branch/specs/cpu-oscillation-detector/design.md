# Per-Container CPU Oscillation Detector - Technical Design

## Architecture Overview

The Per-Container CPU Oscillation Detector is implemented as a **long-running check** that samples CPU usage for each running container at 1Hz and detects rapid oscillation patterns. It uses WorkloadMeta for container discovery/lifecycle and the standard container metrics provider for CPU statistics.

```
+-------------------------------------------------------------------------+
|                    Container CPU Oscillation Check                       |
|  (Long-running: Interval() == 0)                                         |
+-------------------------------------------------------------------------+
|                                                                          |
|  +----------------+    +------------------+    +-----------------+        |
|  |  WorkloadMeta  |    |  1Hz CPU Sampler |    | Metric Emitter  |        |
|  |  Subscriber    |    |  (per container) |    | (15s interval)  |        |
|  +-------+--------+    +--------+---------+    +--------+--------+        |
|          |                      |                      |                 |
|          v                      v                      v                 |
|  +----------------+    +------------------+    +-----------------+        |
|  | Container      |    | Detector Map     |    | Tagger          |        |
|  | Lifecycle      |    | map[containerID] |    | Integration     |        |
|  | Events         |    | *OscillationDet  |    |                 |        |
|  +----------------+    +------------------+    +-----------------+        |
|                                                                          |
+-------------------------------------------------------------------------+
```

## Key Architectural Changes from Host-Level Design

| Aspect | Host-Level (Previous) | Per-Container (New) |
|--------|----------------------|---------------------|
| CPU Source | gopsutil host CPU | Container metrics provider (cgroup) |
| Detector State | Single `*OscillationDetector` | `map[containerID]*OscillationDetector` |
| Discovery | N/A (single host) | WorkloadMeta subscription |
| Lifecycle | N/A | Create/delete detectors on container start/stop |
| Tagging | None | Tagger component integration |
| Metric Namespace | `system.cpu.oscillation.*` | `container.cpu.oscillation.*` |

## Component Design

### OscillationDetector Struct (Unchanged Algorithm)

The core oscillation detection algorithm remains the same as the host-level design. Each container gets its own detector instance.

```go
// OscillationDetector analyzes CPU samples for oscillation patterns
// One instance per container
type OscillationDetector struct {
    // Ring buffer for CPU samples (fixed size, no allocation after init)
    samples     []float64
    sampleIndex int
    sampleCount int

    // Baseline tracking with exponential decay
    baselineVariance float64
    baselineMean     float64

    // Configuration (shared across all detectors)
    config *OscillationConfig

    // State
    warmupRemaining time.Duration
    lastSampleTime  time.Time
}

type OscillationConfig struct {
    WindowSize          int           // Number of samples in ring buffer (default: 60)
    MinZeroCrossings    int           // Minimum direction changes to flag (default: 6)
    AmplitudeMultiplier float64       // Baseline multiplier for significance (default: 4.0)
    MinAmplitude        float64       // Absolute minimum amplitude to trigger (default: 0, disabled)
    DecayFactor         float64       // Exponential decay alpha (default: 0.1)
    WarmupDuration      time.Duration // Initial learning period (default: 5m)
    SampleInterval      time.Duration // Time between samples (default: 1s)
}

type OscillationResult struct {
    Detected      bool
    Amplitude     float64 // Peak-to-trough percentage
    Frequency     float64 // Cycles per second (Hz)
    ZeroCrossings int     // Number of direction changes
}
```

### Check Struct (New: Per-Container Architecture)

```go
// Check implements the per-container CPU oscillation detection check
type Check struct {
    core.CheckBase

    // Per-container detector map
    detectors   map[string]*ContainerDetector
    detectorsMu sync.RWMutex

    // Shared configuration
    config *checkConfig

    // Component dependencies
    wmeta   workloadmeta.Component
    tagger  tagger.Component
    metrics metrics.Provider

    // Lifecycle management
    stopCh          chan struct{}
    wmetaEventCh    chan workloadmeta.EventBundle
}

// ContainerDetector wraps OscillationDetector with container-specific state
type ContainerDetector struct {
    detector     *OscillationDetector
    containerID  string
    namespace    string  // Container namespace (for metrics provider)
    runtime      string  // Container runtime
    runtimeFlavor string // Runtime flavor

    // CPU rate calculation (same pattern as pkg/process/util/containers)
    lastCPUTotal   float64
    lastSampleTime time.Time
}

type checkConfig struct {
    Enabled             bool    `yaml:"enabled"`
    AmplitudeMultiplier float64 `yaml:"amplitude_multiplier"`
    MinAmplitude        float64 `yaml:"min_amplitude"`
    WarmupSeconds       int     `yaml:"warmup_seconds"`
}
```

## Container Discovery and Lifecycle

### WorkloadMeta Integration

The check subscribes to WorkloadMeta for container lifecycle events:

```go
func (c *Check) subscribeToWorkloadMeta() {
    filter := workloadmeta.NewFilter(
        []workloadmeta.Kind{workloadmeta.KindContainer},
        workloadmeta.SourceAll,
        workloadmeta.EventTypeAll,
    )

    c.wmetaEventCh = c.wmeta.Subscribe(
        "container_cpu_oscillation",
        workloadmeta.NormalPriority,
        filter,
    )
}

func (c *Check) handleWorkloadMetaEvent(event workloadmeta.Event) {
    container, ok := event.Entity.(*workloadmeta.Container)
    if !ok {
        return
    }

    switch event.Type {
    case workloadmeta.EventTypeSet:
        // Container created or updated
        if container.State.Running {
            c.ensureDetector(container)
        }
    case workloadmeta.EventTypeUnset:
        // Container removed - immediate state cleanup (REQ-COD-002)
        c.removeDetector(container.ID)
    }
}

func (c *Check) ensureDetector(container *workloadmeta.Container) {
    c.detectorsMu.Lock()
    defer c.detectorsMu.Unlock()

    if _, exists := c.detectors[container.ID]; exists {
        return // Already tracking
    }

    c.detectors[container.ID] = &ContainerDetector{
        detector:      NewOscillationDetector(c.config.toOscillationConfig()),
        containerID:   container.ID,
        namespace:     container.Namespace,
        runtime:       string(container.Runtime),
        runtimeFlavor: string(container.RuntimeFlavor),
        lastCPUTotal:  -1, // Sentinel for "no previous sample"
    }
}

func (c *Check) removeDetector(containerID string) {
    c.detectorsMu.Lock()
    defer c.detectorsMu.Unlock()
    delete(c.detectors, containerID)
}
```

### Container CPU Sampling

Uses the existing container metrics provider, consistent with `pkg/process/util/containers`:

```go
func (c *Check) sampleContainerCPU(cd *ContainerDetector) (float64, error) {
    collector := c.metrics.GetCollector(provider.NewRuntimeMetadata(
        cd.runtime,
        cd.runtimeFlavor,
    ))
    if collector == nil {
        return 0, fmt.Errorf("no collector for runtime %s", cd.runtime)
    }

    stats, err := collector.GetContainerStats(cd.namespace, cd.containerID, 0)
    if err != nil {
        return 0, fmt.Errorf("failed to get container stats: %w", err)
    }

    if stats == nil || stats.CPU == nil || stats.CPU.Total == nil {
        return 0, fmt.Errorf("no CPU stats available")
    }

    // CPU.Total is in nanoseconds (cumulative)
    currentTotal := *stats.CPU.Total
    currentTime := stats.Timestamp
    if currentTime.IsZero() {
        currentTime = time.Now()
    }

    // First sample - need delta
    if cd.lastCPUTotal < 0 || cd.lastSampleTime.IsZero() {
        cd.lastCPUTotal = currentTotal
        cd.lastSampleTime = currentTime
        return 0, fmt.Errorf("first sample, need delta")
    }

    // Calculate CPU percentage since last sample
    timeDelta := currentTime.Sub(cd.lastSampleTime)
    if timeDelta <= 0 {
        return 0, fmt.Errorf("no time elapsed")
    }

    cpuDelta := currentTotal - cd.lastCPUTotal
    if cpuDelta < 0 {
        // Counter reset (container restarted)
        cd.lastCPUTotal = currentTotal
        cd.lastSampleTime = currentTime
        return 0, fmt.Errorf("CPU counter reset")
    }

    // Convert to percentage: (cpu_ns_used / elapsed_ns) * 100
    cpuPercent := (cpuDelta / float64(timeDelta.Nanoseconds())) * 100.0

    cd.lastCPUTotal = currentTotal
    cd.lastSampleTime = currentTime

    return cpuPercent, nil
}
```

## Algorithm Details

The oscillation detection algorithm is unchanged from the host-level design. See the previous design document for details on:
- Zero-crossing detection
- Amplitude calculation
- Baseline tracking (exponential decay)
- Detection logic

## File Structure

```
pkg/collector/corechecks/containers/cpu_oscillation/
    oscillation.go            # Check implementation (per-container)
    oscillation_test.go       # Unit tests
    detector.go               # OscillationDetector logic (unchanged algorithm)
    detector_test.go          # Detector unit tests
    config.go                 # Configuration parsing
    stub.go                   # Stub for non-Linux platforms
```

Note: File location changed from `system/cpu/oscillation/` to `containers/cpu_oscillation/` to reflect the per-container scope.

## Configuration

**conf.d/container_cpu_oscillation.d/conf.yaml.default:**
```yaml
init_config:

instances:
  - enabled: false                 # Explicit opt-in required (default: disabled)
    amplitude_multiplier: 4.0      # Swings must exceed 4x baseline stddev
    min_amplitude: 0               # Absolute minimum amplitude (0 = disabled)
    warmup_seconds: 300            # 5 minute warmup period per container
    # min_collection_interval: 15  # Metric emission interval (detection runs at 1Hz internally)
```

**datadog.yaml (alternative):**
```yaml
container_cpu_oscillation:
  enabled: false
  amplitude_multiplier: 4.0
  min_amplitude: 0
  warmup_seconds: 300
```

## Timing Model

```
+-------------------------------------------------------------------------+
|                         TIMING MODEL (Per Container)                     |
+-------------------------------------------------------------------------+
|                                                                          |
|  SAMPLE INTERVAL: 1 second                                               |
|  - CPU sampled once per second for each container                        |
|  - Each sample added to that container's ring buffer                     |
|                                                                          |
|  DETECTION WINDOW: 60 seconds (sliding, per container)                   |
|  - Ring buffer holds last 60 samples per container                       |
|  - No detection until window is full for that container                  |
|                                                                          |
|  EMISSION INTERVAL: 15 seconds                                           |
|  - Metrics emitted every 15 seconds for ALL containers                   |
|  - Each emission reflects current 60s sliding window per container       |
|                                                                          |
|  WARMUP PERIOD: 300 seconds (5 minutes, per container)                   |
|  - Each container has independent warmup starting at first sample        |
|  - Baseline variance learned, oscillation.detected always 0 during warmup|
|  - Other metrics (amplitude, zero_crossings) still emitted               |
|                                                                          |
|  CONTAINER LIFECYCLE:                                                    |
|  - New container: New detector, fresh warmup                             |
|  - Removed container: Immediate state deletion                           |
|  - Short-lived (<5min): Never triggers detection (acceptable)            |
|                                                                          |
+-------------------------------------------------------------------------+

Timeline (per container):

t=0s              t=60s         t=300s        t=315s
|                 |             |             |
v                 v             v             v
Container         First         Warmup        First possible
starts            emission      ends          detection=1
(collecting)
```

## Metrics Emitted

| Metric Name | Type | Tags | Description |
|-------------|------|------|-------------|
| `container.cpu.oscillation.detected` | Gauge (0/1) | container tags | 1 if oscillation detected in current 60s window. Always 0 during warmup. |
| `container.cpu.oscillation.amplitude` | Gauge | container tags | Peak-to-trough CPU% swing in current window (0-100+ scale) |
| `container.cpu.oscillation.frequency` | Gauge | container tags | Estimated oscillation frequency in Hz (zero_crossings / 60 / 2) |
| `container.cpu.oscillation.baseline_stddev` | Gauge | container tags | Current baseline standard deviation of CPU% (for threshold tuning) |
| `container.cpu.oscillation.zero_crossings` | Gauge | container tags | Direction changes in current 60s window (max 59) |

**Tags included (via tagger component with DD_CHECKS_TAG_CARDINALITY):**
- `container_id`
- `container_name`
- `image_name`, `image_tag`
- `kube_namespace`, `kube_deployment`, `kube_pod_name` (if K8s)
- `ecs_task_family`, `ecs_task_arn` (if ECS)
- Standard orchestrator tags based on environment

## Check Lifecycle

### Initialization (Configure)
1. Parse configuration from YAML
2. Validate configuration, exit if disabled
3. Initialize empty detector map
4. Get WorkloadMeta, Tagger, and Metrics Provider components

### Run Loop (Long-Running)
```go
func (c *Check) Run() error {
    // Subscribe to container lifecycle events
    c.subscribeToWorkloadMeta()
    defer c.wmeta.Unsubscribe(c.wmetaEventCh)

    // Initialize detectors for existing containers
    c.initializeExistingContainers()

    sampleTicker := time.NewTicker(1 * time.Second)
    defer sampleTicker.Stop()

    emitTicker := time.NewTicker(15 * time.Second)
    defer emitTicker.Stop()

    for {
        select {
        case eventBundle := <-c.wmetaEventCh:
            // Handle container lifecycle events
            eventBundle.Acknowledge()
            for _, event := range eventBundle.Events {
                c.handleWorkloadMetaEvent(event)
            }

        case <-sampleTicker.C:
            // Sample CPU for all containers at 1Hz
            c.sampleAllContainers()

        case <-emitTicker.C:
            // Emit metrics for all containers
            c.emitMetrics()

        case <-c.stopCh:
            return nil
        }
    }
}

func (c *Check) initializeExistingContainers() {
    containers := c.wmeta.ListContainersWithFilter(workloadmeta.GetRunningContainers)
    for _, container := range containers {
        c.ensureDetector(container)
    }
}

func (c *Check) sampleAllContainers() {
    c.detectorsMu.RLock()
    detectorsCopy := make([]*ContainerDetector, 0, len(c.detectors))
    for _, cd := range c.detectors {
        detectorsCopy = append(detectorsCopy, cd)
    }
    c.detectorsMu.RUnlock()

    for _, cd := range detectorsCopy {
        cpuPercent, err := c.sampleContainerCPU(cd)
        if err != nil {
            // Log at debug level (transient errors are expected)
            log.Debugf("Failed to sample CPU for container %s: %v", cd.containerID[:12], err)
            continue
        }
        cd.detector.AddSample(cpuPercent)

        // Update warmup timer
        if cd.detector.warmupRemaining > 0 {
            cd.detector.warmupRemaining -= time.Second
        }
    }
}
```

### Metric Emission

```go
func (c *Check) emitMetrics() {
    sender, err := c.GetSender()
    if err != nil {
        return
    }

    c.detectorsMu.RLock()
    defer c.detectorsMu.RUnlock()

    for containerID, cd := range c.detectors {
        // Get container tags via tagger
        entityID := types.NewEntityID(types.ContainerID, containerID)
        tags, err := c.tagger.Tag(entityID, types.LowCardinality) // Respect DD_CHECKS_TAG_CARDINALITY
        if err != nil {
            log.Debugf("Failed to get tags for container %s: %v", containerID[:12], err)
            tags = []string{}
        }

        result := cd.detector.Analyze()

        detected := 0.0
        if result.Detected {
            detected = 1.0
        }

        sender.Gauge("container.cpu.oscillation.detected", detected, "", tags)
        sender.Gauge("container.cpu.oscillation.amplitude", result.Amplitude, "", tags)
        sender.Gauge("container.cpu.oscillation.frequency", result.Frequency, "", tags)
        sender.Gauge("container.cpu.oscillation.zero_crossings", float64(result.ZeroCrossings), "", tags)
        sender.Gauge("container.cpu.oscillation.baseline_stddev",
            math.Sqrt(cd.detector.baselineVariance), "", tags)
    }

    sender.Commit()
}
```

## Error Handling

| Error Condition | Handling | Rationale |
|----------------|----------|-----------|
| No metrics collector for runtime | Skip container, log debug | Not all runtimes support CPU stats |
| Cgroup read failure | Skip container this interval, log debug | Transient errors expected in dynamic environments |
| Container removed mid-sample | Graceful via WorkloadMeta event | Container already removed from map |
| First sample (no delta) | Skip, return error | Need two samples for rate calculation |
| CPU counter reset | Skip, reset tracking state | Container likely restarted |
| No running containers | Run normally, emit no metrics | Not an error condition |
| Tagger failure | Emit metrics without tags | Degraded but still useful |

## Performance Considerations

### Memory Budget

| Component | Per Container | 100 Containers |
|-----------|--------------|----------------|
| OscillationDetector.samples | 480 bytes (60 x 8 bytes) | 48 KB |
| OscillationDetector struct fields | ~50 bytes | 5 KB |
| ContainerDetector wrapper | ~100 bytes | 10 KB |
| Map overhead | ~50 bytes | 5 KB |
| **Total** | **~500 bytes** | **~50 KB** |

### CPU Overhead

- Container metrics provider call: ~0.1-1ms per container (varies by runtime)
- Oscillation analysis: O(n) where n=60 samples, ~0.01ms
- For 100 containers at 1Hz: ~100-200ms total per second = ~10-20% of one core worst case
- Target: <1% of Agent process CPU (achieved by leveraging cached metrics)

### Optimizations

1. **Leverage cached metrics**: Container metrics provider already caches stats; use `cacheValidity: 0` to get freshest data without extra syscalls
2. **No allocations in hot path**: Ring buffer pre-allocated, reused per container
3. **RWMutex for detector map**: Allows concurrent reads during sampling
4. **Copy-on-iterate**: Copy detector slice before sampling to minimize lock hold time

## Testing Strategy

### Unit Tests (detector_test.go)
1. Zero crossings calculation with known patterns
2. Amplitude calculation edge cases
3. Baseline exponential decay behavior
4. Warmup period enforcement
5. Detection threshold tuning

### Unit Tests (oscillation_test.go)
1. Container discovery via WorkloadMeta mock
2. Container removal triggers state cleanup
3. CPU rate calculation from cumulative values
4. Metric emission with correct tags
5. Configuration validation

### Integration Tests
1. Check starts and stops cleanly
2. Handles container churn (create/delete cycles)
3. Metrics emitted with correct tags
4. Long-running behavior (no memory growth)
5. Multiple container runtimes (docker, containerd)

### Staging Validation
1. Deploy to staging K8s cluster with 50+ containers
2. Build dashboard grouping by namespace/deployment
3. Correlate `detected=1` events with known container issues
4. Verify tags enable direct navigation to logs/events
5. Tune thresholds based on false positive/negative rates

## Requirements Traceability

| Requirement | Implementation Location | Approach |
|-------------|------------------------|----------|
| **REQ-COD-001:** Detect Rapid CPU Cycling Per Container | `detector.go:Analyze()` | Zero-crossing count >= 6 AND amplitude > 4x baseline stddev AND amplitude > min_amplitude |
| **REQ-COD-002:** Establish Container-Specific Baseline | `detector.go:updateBaseline()`, `oscillation.go:handleWorkloadMetaEvent()` | Exponential decay (alpha=0.1) with 5-minute warmup; immediate cleanup on container removal |
| **REQ-COD-003:** Report Oscillation Characteristics with Container Tags | `oscillation.go:emitMetrics()` | Gauge metrics with tagger integration for container tags |
| **REQ-COD-004:** Minimal Performance Impact at Scale | `oscillation.go:Run()` | Container metrics provider (cached), ~500 bytes per container, <50KB for 100 containers |
| **REQ-COD-005:** Configurable Detection with Default Disabled | `config.go` | YAML config for enabled, amplitude_multiplier, min_amplitude, warmup_seconds; default disabled |
| **REQ-COD-006:** Metric Emission for All Tracked Containers | `oscillation.go:emitMetrics()` | Iterate all detectors, emit for each regardless of detection state |
| **REQ-COD-007:** Graceful Error Handling | `oscillation.go:sampleAllContainers()` | Log debug, skip container, continue; WorkloadMeta events for lifecycle |

## Future Considerations (Out of Scope)

- Per-process within container oscillation detection
- Multiple frequency band detection (fast vs. slow oscillations)
- Event emission on state transitions (in addition to metrics)
- Baseline persistence across agent restarts
- Host-level aggregation metric (can be computed at query time)
- Cross-container correlation (e.g., multiple containers oscillating in sync)

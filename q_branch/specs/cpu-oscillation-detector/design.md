# CPU Oscillation Detector - Technical Design

## Architecture Overview

The CPU Oscillation Detector is implemented as a **long-running check** that samples aggregate CPU usage at 1Hz and detects rapid oscillation patterns. It runs independently of the standard 15-second CPU check, maintaining its own sampling loop and baseline state.

```
┌─────────────────────────────────────────────────────────────────┐
│                     CPU Oscillation Check                       │
│  (Long-running: Interval() == 0)                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐       │
│  │   1Hz CPU    │───>│  Oscillation │───>│   Metric     │       │
│  │   Sampler    │    │   Detector   │    │   Emitter    │       │
│  └──────────────┘    └──────────────┘    └──────────────┘       │
│         │                   │                    │              │
│         v                   v                    v              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐       │
│  │ Ring Buffer  │    │   Baseline   │    │   Sender     │       │
│  │ (60 samples) │    │   Tracker    │    │  (Gauges)    │       │
│  └──────────────┘    └──────────────┘    └──────────────┘       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Component Design

### OscillationDetector Struct

```go
// OscillationDetector analyzes CPU samples for oscillation patterns
type OscillationDetector struct {
    // Ring buffer for CPU samples (fixed size, no allocation after init)
    samples     []float64
    sampleIndex int
    sampleCount int

    // Baseline tracking with exponential decay
    baselineVariance float64
    baselineMean     float64

    // Configuration
    config OscillationConfig

    // State
    warmupRemaining time.Duration
    lastSampleTime  time.Time
}

type OscillationConfig struct {
    WindowSize          int           // Number of samples in ring buffer (default: 60)
    MinZeroCrossings    int           // Minimum direction changes to flag (default: 6)
    AmplitudeMultiplier float64       // Baseline multiplier for significance (default: 2.0)
    DecayFactor         float64       // Exponential decay alpha (default: 0.1)
    WarmupDuration      time.Duration // Initial learning period (default: 5m)
    SampleInterval      time.Duration // Time between samples (default: 1s)
}

type OscillationResult struct {
    Detected   bool
    Amplitude  float64  // Peak-to-trough percentage
    Frequency  float64  // Cycles per second (Hz)
    ZeroCrossings int   // Number of direction changes
}
```

### Check Struct

```go
// Check implements the CPU oscillation detection check
type Check struct {
    core.CheckBase

    detector *OscillationDetector
    config   *checkConfig

    // Long-running check control
    stopCh   chan struct{}

    // CPU sampling (reuse gopsutil)
    lastCPUTimes cpu.TimesStat
    lastSampleTime time.Time
}

type checkConfig struct {
    Enabled             bool    `yaml:"enabled"`
    AmplitudeMultiplier float64 `yaml:"amplitude_multiplier"`
    WarmupSeconds       int     `yaml:"warmup_seconds"`
}
```

## Algorithm Details

### Zero-Crossing Detection

Oscillation is detected by counting "zero crossings" of the CPU usage derivative (rate of change). A zero crossing occurs when CPU transitions from increasing to decreasing or vice versa.

```
CPU %
  ^
  |    /\      /\
  |   /  \    /  \      <- 4 zero crossings in this pattern
  |  /    \  /    \
  | /      \/      \
  +-------------------> time
```

**Implementation:**
```go
func (d *OscillationDetector) countZeroCrossings() int {
    if d.sampleCount < 3 {
        return 0
    }

    crossings := 0
    var prevDiff float64

    for i := 1; i < d.sampleCount; i++ {
        curr := d.getSample(i)
        prev := d.getSample(i - 1)
        currDiff := curr - prev

        if i > 1 {
            // Sign change = zero crossing
            if (prevDiff > 0 && currDiff < 0) || (prevDiff < 0 && currDiff > 0) {
                crossings++
            }
        }
        prevDiff = currDiff
    }
    return crossings
}
```

### Amplitude Calculation

Amplitude is the difference between maximum and minimum CPU values in the current window:

```go
func (d *OscillationDetector) calculateAmplitude() float64 {
    if d.sampleCount < 2 {
        return 0
    }

    min, max := d.getSample(0), d.getSample(0)
    for i := 1; i < d.sampleCount; i++ {
        v := d.getSample(i)
        if v < min {
            min = v
        }
        if v > max {
            max = v
        }
    }
    return max - min
}
```

### Baseline Tracking (Exponential Decay)

The baseline variance adapts to each host's normal behavior using exponential moving average:

```go
func (d *OscillationDetector) updateBaseline(newVariance float64) {
    if d.baselineVariance == 0 {
        // First sample
        d.baselineVariance = newVariance
        return
    }

    // Exponential decay: new = α * current + (1-α) * old
    α := d.config.DecayFactor
    d.baselineVariance = α*newVariance + (1-α)*d.baselineVariance
}
```

### Detection Logic

```go
func (d *OscillationDetector) Analyze() OscillationResult {
    result := OscillationResult{}

    // No analysis until window is full (60 samples)
    if d.sampleCount < d.config.WindowSize {
        return result
    }

    // Still in warmup - learn baseline but don't flag oscillation
    if d.warmupRemaining > 0 {
        d.updateBaseline(d.calculateVariance())
        return result
    }

    zeroCrossings := d.countZeroCrossings()
    amplitude := d.calculateAmplitude()
    currentVariance := d.calculateVariance()

    // Update baseline (continuous learning)
    d.updateBaseline(currentVariance)

    // Check oscillation criteria
    baselineStdDev := math.Sqrt(d.baselineVariance)
    amplitudeThreshold := d.config.AmplitudeMultiplier * baselineStdDev

    result.ZeroCrossings = zeroCrossings
    result.Amplitude = amplitude
    result.Frequency = float64(zeroCrossings) / float64(d.config.WindowSize) / 2.0 // cycles per second

    if zeroCrossings >= d.config.MinZeroCrossings && amplitude > amplitudeThreshold {
        result.Detected = true
    }

    return result
}
```

## File Structure

```
pkg/collector/corechecks/system/cpu/
├── cpu/
│   ├── cpu.go                    # Existing CPU check (unchanged)
│   ├── cpu_windows.go            # Existing Windows impl (unchanged)
│   └── ...
└── oscillation/                  # NEW directory
    ├── oscillation.go            # Check implementation
    ├── oscillation_test.go       # Unit tests
    ├── detector.go               # OscillationDetector logic
    ├── detector_test.go          # Detector unit tests
    └── config.go                 # Configuration parsing
```

## Configuration

**conf.d/cpu_oscillation.d/conf.yaml.default:**
```yaml
init_config:

instances:
  - amplitude_multiplier: 2.0      # Swings must exceed 2x baseline stddev
    warmup_seconds: 300            # 5 minute warmup period
    # min_collection_interval: 15  # Metric emission interval (detection runs at 1Hz internally)
```

## Timing Model

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         TIMING MODEL                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  SAMPLE INTERVAL: 1 second                                              │
│  ├─ CPU sampled once per second via gopsutil                            │
│  └─ Each sample added to ring buffer                                    │
│                                                                         │
│  DETECTION WINDOW: 60 seconds (sliding)                                 │
│  ├─ Ring buffer holds last 60 samples                                   │
│  └─ No metrics emitted until window is full                             │
│                                                                         │
│  EMISSION INTERVAL: 15 seconds                                          │
│  ├─ Metrics emitted every 15 seconds (after window is full)             │
│  └─ Each emission reflects current 60s sliding window                   │
│                                                                         │
│  WARMUP PERIOD: 300 seconds (5 minutes)                                 │
│  ├─ Baseline variance learned, oscillation.detected always 0            │
│  └─ Other metrics (amplitude, zero_crossings, etc.) still emitted       │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

Timeline:

t=0s              t=60s         t=300s        t=315s
│                 │             │             │
▼                 ▼             ▼             ▼
Start             First         Warmup        First possible
(collecting)      emission      ends          detection=1
```

**Key behaviors:**
- No metrics emitted until 60 samples collected (window full)
- `oscillation.detected=1` only possible after warmup ends (t≥300s)
- An oscillation event stays in the window for 60s, so may appear in up to 4 consecutive emissions

## Metrics Emitted

| Metric Name | Type | Description |
|-------------|------|-------------|
| `system.cpu.oscillation.detected` | Gauge (0/1) | 1 if oscillation detected in current 60s window. Always 0 during warmup. |
| `system.cpu.oscillation.amplitude` | Gauge | Peak-to-trough CPU% swing in current window (0-100 scale) |
| `system.cpu.oscillation.frequency` | Gauge | Estimated oscillation frequency in Hz (zero_crossings / 60 / 2) |
| `system.cpu.oscillation.baseline_stddev` | Gauge | Current baseline standard deviation of CPU% (for threshold tuning) |
| `system.cpu.oscillation.zero_crossings` | Gauge | Direction changes in current 60s window (max 59) |

## Check Lifecycle

### Initialization (Configure)
1. Parse configuration from YAML
2. Initialize OscillationDetector with config
3. Allocate ring buffer (fixed 60 elements)
4. Set warmup timer

### Run Loop (Long-Running)
```go
func (c *Check) Run() error {
    ticker := time.NewTicker(c.detector.config.SampleInterval)
    defer ticker.Stop()

    emitTicker := time.NewTicker(15 * time.Second) // Emit metrics every 15s
    defer emitTicker.Stop()

    for {
        select {
        case <-ticker.C:
            // Sample CPU at 1Hz
            cpuPercent, err := c.sampleCPU()
            if err != nil {
                continue
            }
            c.detector.AddSample(cpuPercent)

            // Update warmup timer
            if c.detector.warmupRemaining > 0 {
                c.detector.warmupRemaining -= c.detector.config.SampleInterval
            }

        case <-emitTicker.C:
            // Emit metrics at standard interval
            c.emitMetrics()

        case <-c.stopCh:
            return nil
        }
    }
}
```

### CPU Sampling

Reuse gopsutil for consistency with existing CPU check:

```go
func (c *Check) sampleCPU() (float64, error) {
    times, err := cpu.Times(false) // Aggregate, not per-CPU
    if err != nil {
        return 0, err
    }

    if len(times) == 0 {
        return 0, errors.New("no CPU times returned")
    }

    t := times[0]
    total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal

    if c.lastSampleTime.IsZero() {
        c.lastCPUTimes = t
        c.lastSampleTime = time.Now()
        return 0, errors.New("first sample, need delta")
    }

    // Calculate CPU busy percentage since last sample
    prevTotal := c.lastCPUTimes.User + c.lastCPUTimes.System + c.lastCPUTimes.Idle +
                 c.lastCPUTimes.Nice + c.lastCPUTimes.Iowait + c.lastCPUTimes.Irq +
                 c.lastCPUTimes.Softirq + c.lastCPUTimes.Steal

    deltaTotal := total - prevTotal
    deltaIdle := t.Idle - c.lastCPUTimes.Idle

    c.lastCPUTimes = t
    c.lastSampleTime = time.Now()

    if deltaTotal == 0 {
        return 0, nil
    }

    busyPercent := 100.0 * (1.0 - deltaIdle/deltaTotal)
    return busyPercent, nil
}
```

### Metric Emission

```go
func (c *Check) emitMetrics() {
    sender, err := c.GetSender()
    if err != nil {
        return
    }

    result := c.detector.Analyze()

    detected := 0.0
    if result.Detected {
        detected = 1.0
    }

    sender.Gauge("system.cpu.oscillation.detected", detected, "", nil)
    sender.Gauge("system.cpu.oscillation.amplitude", result.Amplitude, "", nil)
    sender.Gauge("system.cpu.oscillation.frequency", result.Frequency, "", nil)
    sender.Gauge("system.cpu.oscillation.zero_crossings", float64(result.ZeroCrossings), "", nil)
    sender.Gauge("system.cpu.oscillation.baseline_stddev", math.Sqrt(c.detector.baselineVariance), "", nil)

    sender.Commit()
}
```

## Error Handling

- **CPU sampling failures:** Log warning, skip sample, continue (don't crash check)
- **First sample:** Skip (need delta for percentage calculation)
- **Warmup period:** Collect data but don't emit `detected=1`
- **Insufficient samples:** No metrics emitted until 60 samples collected

## Performance Considerations

- **CPU overhead:** gopsutil `cpu.Times()` is lightweight (~0.1ms per call)
- **Memory:** Fixed allocation: 60 floats = 480 bytes + struct overhead
- **No allocations in hot path:** Ring buffer reuses memory

## Testing Strategy

### Unit Tests (detector_test.go)
1. Zero crossings calculation with known patterns
2. Amplitude calculation edge cases
3. Baseline exponential decay behavior
4. Warmup period enforcement
5. Detection threshold tuning

### Integration Tests
1. Check starts and stops cleanly
2. Metrics emitted at expected interval
3. Long-running behavior (no memory growth)

### Staging Validation
1. Deploy to staging cluster
2. Build dashboard for oscillation metrics
3. Correlate `detected=1` events with known incidents
4. Tune thresholds based on false positive/negative rates

## Requirements Traceability

| Requirement | Implementation Location | Approach |
|-------------|------------------------|----------|
| **REQ-COD-001:** Detect Rapid CPU Cycling | `detector.go:Analyze()` | Zero-crossing count ≥6 AND amplitude >2x baseline stddev |
| **REQ-COD-002:** Establish Host-Specific Baseline | `detector.go:updateBaseline()` | Exponential decay (α=0.1) with 5-minute warmup |
| **REQ-COD-003:** Report Oscillation Characteristics | `oscillation.go:emitMetrics()` | Gauge metrics for amplitude, frequency, zero_crossings |
| **REQ-COD-004:** Minimal Performance Impact | `oscillation.go:Run()` | gopsutil cpu.Times() at 1Hz, fixed 480-byte ring buffer |
| **REQ-COD-005:** Configurable Detection Sensitivity | `config.go` | YAML instance config for amplitude_multiplier, warmup_seconds |

## Future Considerations (Out of Scope)

- Per-process oscillation detection
- Multiple frequency band detection
- Event emission on state transitions
- Baseline persistence across restarts (see Auditer/registry.json)
- Correlation with memory/IO oscillation patterns

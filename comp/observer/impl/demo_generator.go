// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"math/rand"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// GeneratorConfig configures the data generator behavior.
type GeneratorConfig struct {
	TimeScale     float64 // multiplier for time (1.0 = realtime, 0.1 = 10x faster)
	BaselineNoise float64 // amplitude of random baseline variation (default: 0.1 = 10%)
	Seed          int64   // random seed for deterministic noise (default: 42)
	BaseTimestamp int64   // base timestamp for deterministic timing (default: 1000000)
}

// Timeline constants (in simulation seconds at TimeScale=1.0)
// Total duration: 70s
const (
	// Phase boundaries
	phaseBaselineEnd     = 25.0 // Baseline: 0-25s
	phaseIncidentRampEnd = 30.0 // Incident ramp: 25-30s
	phaseIncidentPeakEnd = 45.0 // Incident peak: 30-45s
	phaseRecoveryEnd     = 50.0 // Recovery: 45-50s
	phaseTotalDuration   = 70.0 // Post-incident: 50-70s

	// Non-correlated spike timing (single metric spikes that recover quickly)
	spikeGCStart      = 10.0 // Brief GC spike at 10-12s
	spikeGCEnd        = 12.0
	spikeLatencyStart = 17.0 // Brief latency spike at 17-19s
	spikeLatencyEnd   = 19.0

	// Causal chain delays (seconds relative to phaseBaselineEnd)
	// Story: heap pressure builds -> GC struggles -> latency degrades -> errors rise
	heapLeadTime = -3.0 // Heap starts growing 3s BEFORE GC issues (t=22s)
	gcDelay      = 0.0  // GC pause time increases at t=25s
	latencyDelay = 1.0  // Latency degrades 1s after GC issues (t=26s)
	errorDelay   = 2.0  // Errors start 2s after GC issues (t=27s)
	cpuDelay     = 1.5  // CPU spikes slightly after GC (retries, GC work)
)

// errorLogMessages contains various realistic error messages to rotate through during the demo.
var errorLogMessages = []string{
	"request timeout: upstream service did not respond within 30s",
	"connection pool exhausted: max connections reached",
	"circuit breaker open: too many recent failures",
	"retry limit exceeded after 3 attempts",
	"memory pressure: request rejected",
	"GC overhead limit exceeded",
}

// backgroundMetric defines a metric that maintains steady baseline behavior.
type backgroundMetric struct {
	name           string
	baseline       float64
	incidentOffset float64 // subtle increase during incident peak (0 = no change)
}

// Background metrics that show subtle effects during the incident.
var defaultBackgroundMetrics = []backgroundMetric{
	{name: "system.disk.read_ops", baseline: 150, incidentOffset: 0},
	{name: "system.disk.write_ops", baseline: 80, incidentOffset: 20}, // slight increase from logging/swapping
	{name: "system.net.bytes_recv", baseline: 50000, incidentOffset: 0},
	{name: "system.net.bytes_sent", baseline: 45000, incidentOffset: -10000}, // drops during incident (fewer successful responses)
}

// DataGenerator simulates the incident scenario by sending observations to a Handle.
type DataGenerator struct {
	handle        observer.Handle
	config        GeneratorConfig
	rng           *rand.Rand
	baseTimestamp int64 // simulation start time for consistent bucketing
}

// NewDataGenerator creates a new data generator that sends observations to the given handle.
func NewDataGenerator(handle observer.Handle, config GeneratorConfig) *DataGenerator {
	// Apply defaults
	if config.TimeScale <= 0 {
		config.TimeScale = 1.0
	}
	// Use < 0 check to allow explicitly setting noise to 0
	if config.BaselineNoise < 0 {
		config.BaselineNoise = 0.1
	}
	// Use fixed seed for deterministic behavior (0 means use default)
	if config.Seed == 0 {
		config.Seed = 42
	}
	// Use fixed base timestamp for deterministic timing (0 means use default)
	if config.BaseTimestamp == 0 {
		config.BaseTimestamp = 1000000
	}

	return &DataGenerator{
		handle:        handle,
		config:        config,
		rng:           rand.New(rand.NewSource(config.Seed)),
		baseTimestamp: config.BaseTimestamp,
	}
}

// Run generates data until the context is cancelled.
//
// Single-host incident scenario: memory pressure causes cascading degradation
//
// Causal chain (all on one host):
//
//	heap grows -> GC pauses increase -> request latency degrades -> error rate rises
//
// Timeline (at TimeScale=1.0):
// | Phase             | Time    | heap.used_mb   | gc.pause_ms   | latency_p99   | error_rate   | cpu.percent  |
// |-------------------|---------|----------------|---------------|---------------|--------------|--------------|
// | Baseline          | 0-10s   | 512 +/- noise  | 15 +/- noise  | 45 +/- noise  | 0.1% +/- n   | 35 +/- noise |
// | Spike (GC)        | 10-12s  | normal         | 15->80->15    | normal        | normal       | normal       |
// | Baseline          | 12-17s  | normal         | normal        | normal        | normal       | normal       |
// | Spike (latency)   | 17-19s  | normal         | normal        | 45->200->45   | normal       | normal       |
// | Baseline          | 19-22s  | normal         | normal        | normal        | normal       | normal       |
// | Heap ramp         | 22-25s  | 512->900       | normal        | normal        | normal       | normal       |
// | Incident ramp     | 25-30s  | 900 (high)     | 15->150       | 45->500 (+1s) | 0.1->8 (+2s) | 35->75 (+1.5s)|
// | Incident peak     | 30-45s  | 900 +/- noise  | 150 +/- noise | 500 +/- noise | 8% +/- noise | 75 +/- noise |
// | Recovery          | 45-50s  | 900->512       | 150->15       | 500->45       | 8->0.1       | 75->35       |
// | Post-incident     | 50-70s  | normal         | normal        | normal        | normal       | normal       |
func (g *DataGenerator) Run(ctx context.Context) {
	// Scale the tick interval by TimeScale (faster TimeScale = shorter interval)
	tickInterval := time.Duration(float64(time.Second) * g.config.TimeScale)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	startTime := time.Now()
	var lastLogTime time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Calculate elapsed time in simulation seconds
			elapsed := time.Since(startTime).Seconds() / g.config.TimeScale

			// Use simulation time for timestamps to ensure consistent bucket alignment
			simTimestamp := float64(g.baseTimestamp + int64(elapsed))

			// === Incident metrics (correlated, with causal delays) ===

			// Heap usage: leading indicator, starts growing before other symptoms
			heapUsed := g.getHeapUsedValue(elapsed)
			g.handle.ObserveMetric(&metricSample{
				name:      "runtime.heap.used_mb",
				value:     heapUsed,
				timestamp: simTimestamp,
			})

			// GC pause time: increases as heap pressure builds
			gcPause := g.getGCPauseValue(elapsed)
			g.handle.ObserveMetric(&metricSample{
				name:      "runtime.gc.pause_ms",
				value:     gcPause,
				timestamp: simTimestamp,
			})

			// Request latency: degrades as GC pauses increase
			latency := g.getLatencyValue(elapsed)
			g.handle.ObserveMetric(&metricSample{
				name:      "app.request.latency_p99_ms",
				value:     latency,
				timestamp: simTimestamp,
			})

			// Error rate: rises as requests start timing out
			errorRate := g.getErrorRateValue(elapsed)
			g.handle.ObserveMetric(&metricSample{
				name:      "app.request.error_rate",
				value:     errorRate,
				timestamp: simTimestamp,
			})

			// CPU usage: spikes due to GC work and retry storms
			cpuPercent := g.getCPUValue(elapsed)
			g.handle.ObserveMetric(&metricSample{
				name:      "system.cpu.user_percent",
				value:     cpuPercent,
				timestamp: simTimestamp,
			})

			// === Background metrics (stable or subtle changes) ===
			for _, bg := range defaultBackgroundMetrics {
				g.handle.ObserveMetric(&metricSample{
					name:      bg.name,
					value:     g.getBackgroundValue(elapsed, bg),
					timestamp: simTimestamp,
				})
			}

			// === Logs ===
			logInterval := g.getLogInterval(elapsed)
			scaledLogInterval := time.Duration(float64(logInterval) * g.config.TimeScale)
			if time.Since(lastLogTime) >= scaledLogInterval {
				logsPerTick := 1
				if tickInterval > scaledLogInterval && scaledLogInterval > 0 {
					logsPerTick = int(tickInterval / scaledLogInterval)
				}
				for i := 0; i < logsPerTick; i++ {
					g.handle.ObserveLog(&logMessage{
						content:   []byte(errorLogMessages[g.rng.Intn(len(errorLogMessages))]),
						status:    "error",
						timestamp: g.baseTimestamp + int64(elapsed),
					})
				}
				lastLogTime = time.Now()
			}
		}
	}
}

// getHeapUsedValue returns heap memory usage (leading indicator).
func (g *DataGenerator) getHeapUsedValue(elapsed float64) float64 {
	const baseline, peak = 512.0, 900.0 // MB
	baseValue := g.getPhaseValueWithDelay(elapsed, baseline, peak, heapLeadTime)
	return g.applyNoise(baseValue)
}

// getGCPauseValue returns GC pause time in milliseconds.
// Includes a non-correlated spike at 10-12s.
func (g *DataGenerator) getGCPauseValue(elapsed float64) float64 {
	const baseline, peak, spikeLevel = 15.0, 150.0, 80.0 // ms

	// Non-correlated spike
	if elapsed >= spikeGCStart && elapsed < spikeGCEnd {
		return g.applyNoise(g.triangleSpike(elapsed, spikeGCStart, spikeGCEnd, baseline, spikeLevel))
	}

	baseValue := g.getPhaseValueWithDelay(elapsed, baseline, peak, gcDelay)
	return g.applyNoise(baseValue)
}

// getLatencyValue returns request latency P99 in milliseconds.
// Includes a non-correlated spike at 17-19s.
func (g *DataGenerator) getLatencyValue(elapsed float64) float64 {
	const baseline, peak, spikeLevel = 45.0, 500.0, 200.0 // ms

	// Non-correlated spike
	if elapsed >= spikeLatencyStart && elapsed < spikeLatencyEnd {
		return g.applyNoise(g.triangleSpike(elapsed, spikeLatencyStart, spikeLatencyEnd, baseline, spikeLevel))
	}

	baseValue := g.getPhaseValueWithDelay(elapsed, baseline, peak, latencyDelay)
	return g.applyNoise(baseValue)
}

// getErrorRateValue returns the error rate as a percentage.
func (g *DataGenerator) getErrorRateValue(elapsed float64) float64 {
	const baseline, peak = 0.1, 8.0 // percent
	baseValue := g.getPhaseValueWithDelay(elapsed, baseline, peak, errorDelay)
	return g.applyNoise(baseValue)
}

// getCPUValue returns CPU user percentage.
func (g *DataGenerator) getCPUValue(elapsed float64) float64 {
	const baseline, peak = 35.0, 75.0 // percent
	baseValue := g.getPhaseValueWithDelay(elapsed, baseline, peak, cpuDelay)
	return g.applyNoise(baseValue)
}

// triangleSpike generates a triangle-wave spike: ramps up to peak at midpoint, then back down.
func (g *DataGenerator) triangleSpike(elapsed, start, end, baseline, peak float64) float64 {
	duration := end - start
	midpoint := start + duration/2
	if elapsed < midpoint {
		progress := (elapsed - start) / (duration / 2)
		return baseline + (peak-baseline)*progress
	}
	progress := (elapsed - midpoint) / (duration / 2)
	return peak - (peak-baseline)*progress
}

// getPhaseValueWithDelay returns the phase-based value with a causal delay.
func (g *DataGenerator) getPhaseValueWithDelay(elapsed float64, baseline float64, peak float64, delay float64) float64 {
	adjustedBaselineEnd := phaseBaselineEnd + delay
	adjustedRampEnd := phaseIncidentRampEnd + delay
	adjustedPeakEnd := phaseIncidentPeakEnd + delay
	adjustedRecoveryEnd := phaseRecoveryEnd + delay

	switch {
	case elapsed < adjustedBaselineEnd:
		return baseline
	case elapsed < adjustedRampEnd:
		progress := (elapsed - adjustedBaselineEnd) / (adjustedRampEnd - adjustedBaselineEnd)
		return baseline + (peak-baseline)*progress
	case elapsed < adjustedPeakEnd:
		return peak
	case elapsed < adjustedRecoveryEnd:
		progress := (elapsed - adjustedPeakEnd) / (adjustedRecoveryEnd - adjustedPeakEnd)
		return peak - (peak-baseline)*progress
	default:
		return baseline
	}
}

// getBackgroundValue returns background metric value with subtle incident effects.
func (g *DataGenerator) getBackgroundValue(elapsed float64, bg backgroundMetric) float64 {
	if bg.incidentOffset == 0 {
		return g.applyNoise(bg.baseline)
	}

	var value float64
	switch {
	case elapsed < phaseBaselineEnd:
		value = bg.baseline
	case elapsed < phaseIncidentRampEnd:
		progress := (elapsed - phaseBaselineEnd) / (phaseIncidentRampEnd - phaseBaselineEnd)
		value = bg.baseline + bg.incidentOffset*progress
	case elapsed < phaseIncidentPeakEnd:
		value = bg.baseline + bg.incidentOffset
	case elapsed < phaseRecoveryEnd:
		progress := (elapsed - phaseIncidentPeakEnd) / (phaseRecoveryEnd - phaseIncidentPeakEnd)
		value = bg.baseline + bg.incidentOffset*(1-progress)
	default:
		value = bg.baseline
	}
	return g.applyNoise(value)
}

// getLogInterval returns the log emission interval for the given elapsed time.
func (g *DataGenerator) getLogInterval(elapsed float64) time.Duration {
	switch {
	case elapsed < phaseBaselineEnd:
		return 5 * time.Second
	case elapsed < phaseIncidentRampEnd:
		return 2 * time.Second
	case elapsed < phaseIncidentPeakEnd:
		return 500 * time.Millisecond
	case elapsed < phaseRecoveryEnd:
		return 2 * time.Second
	default:
		return 5 * time.Second
	}
}

// applyNoise applies random noise to a value.
func (g *DataGenerator) applyNoise(value float64) float64 {
	noiseFactor := 1 + g.config.BaselineNoise*(g.rng.Float64()*2-1)
	return value * noiseFactor
}

// metricSample implements observer.MetricView for generated metrics.
type metricSample struct {
	name      string
	value     float64
	tags      []string
	timestamp float64
}

func (m *metricSample) GetName() string        { return m.name }
func (m *metricSample) GetValue() float64      { return m.value }
func (m *metricSample) GetRawTags() []string   { return m.tags }
func (m *metricSample) GetTimestamp() float64  { return m.timestamp }
func (m *metricSample) GetSampleRate() float64 { return 1.0 }

// logMessage implements observer.LogView for generated logs.
type logMessage struct {
	content   []byte
	status    string
	tags      []string
	hostname  string
	timestamp int64
}

func (l *logMessage) GetContent() []byte  { return l.content }
func (l *logMessage) GetStatus() string   { return l.status }
func (l *logMessage) GetTags() []string   { return l.tags }
func (l *logMessage) GetHostname() string { return l.hostname }
func (l *logMessage) GetTimestamp() int64 { return l.timestamp }

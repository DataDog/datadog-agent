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
}

// Timeline constants (in simulation seconds at TimeScale=1.0)
// Total duration: 70s
const (
	// Phase boundaries
	phaseBaselineEnd     = 25.0  // Baseline: 0-25s
	phaseIncidentRampEnd = 30.0  // Incident ramp: 25-30s
	phaseIncidentPeakEnd = 45.0  // Incident peak: 30-45s
	phaseRecoveryEnd     = 50.0  // Recovery: 45-50s
	phaseTotalDuration   = 70.0  // Post-incident: 50-70s

	// Non-correlated spike timing (single metric spikes that recover quickly)
	spikeRetransmitStart = 10.0 // Brief retransmit spike at 10-12s
	spikeRetransmitEnd   = 12.0
	spikeLockStart       = 17.0 // Brief lock contention spike at 17-19s
	spikeLockEnd         = 19.0
)

// backgroundMetric defines a metric that maintains steady baseline behavior.
type backgroundMetric struct {
	name     string
	baseline float64
	// Optional: slow drift parameters (not used yet, for future extension)
}

// Standard background metrics that don't participate in incidents
var defaultBackgroundMetrics = []backgroundMetric{
	{name: "cpu.user_percent", baseline: 45},
	{name: "memory.used_percent", baseline: 62},
	{name: "disk.io_ops", baseline: 150},
	{name: "http.requests_per_sec", baseline: 500},
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

	return &DataGenerator{
		handle:        handle,
		config:        config,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
		baseTimestamp: time.Now().Unix(),
	}
}

// Run generates data until the context is cancelled.
// Timeline (at TimeScale=1.0):
// | Phase             | Time    | retransmits      | lock_contention   | error logs  | background |
// |-------------------|---------|------------------|-------------------|-------------|------------|
// | Baseline          | 0-10s   | 5 +/- noise      | 1000 +/- noise    | ~1 per 5s   | normal     |
// | Spike (retrans)   | 10-12s  | 5->30->5 spike   | 1000 +/- noise    | ~1 per 5s   | normal     |
// | Baseline          | 12-17s  | 5 +/- noise      | 1000 +/- noise    | ~1 per 5s   | normal     |
// | Spike (lock)      | 17-19s  | 5 +/- noise      | 1000->6000->1000  | ~1 per 5s   | normal     |
// | Baseline          | 19-25s  | 5 +/- noise      | 1000 +/- noise    | ~1 per 5s   | normal     |
// | Incident ramp     | 25-30s  | 5->50 linear     | 1000->10000       | ~1 per 2s   | normal     |
// | Incident peak     | 30-45s  | 50 +/- noise     | 10000 +/- noise   | ~1 per 0.5s | normal     |
// | Recovery          | 45-50s  | 50->5 linear     | 10000->1000       | ~1 per 2s   | normal     |
// | Post-incident     | 50-70s  | 5 +/- noise      | 1000 +/- noise    | ~1 per 5s   | normal     |
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
			// regardless of timescale. This keeps baseline/incident/recovery phases
			// in the same relative positions within the data.
			simTimestamp := float64(g.baseTimestamp + int64(elapsed))

			// Generate and emit incident-related metrics
			retransmits := g.getRetransmitValue(elapsed)
			lockContention := g.getLockContentionValue(elapsed)

			g.handle.ObserveMetric(&metricSample{
				name:      "network.retransmits",
				value:     retransmits,
				timestamp: simTimestamp,
			})

			g.handle.ObserveMetric(&metricSample{
				name:      "ebpf.lock_contention_ns",
				value:     lockContention,
				timestamp: simTimestamp,
			})

			// Emit background metrics (always normal, never anomalous)
			for _, bg := range defaultBackgroundMetrics {
				g.handle.ObserveMetric(&metricSample{
					name:      bg.name,
					value:     g.applyNoise(bg.baseline),
					timestamp: simTimestamp,
				})
			}

			// Emit logs at phase-dependent intervals
			// Calculate how many logs should be emitted per tick to match the target frequency
			logInterval := g.getLogInterval(elapsed)
			scaledLogInterval := time.Duration(float64(logInterval) * g.config.TimeScale)
			if time.Since(lastLogTime) >= scaledLogInterval {
				// Emit multiple logs if tick interval > log interval to maintain correct frequency
				logsPerTick := 1
				if tickInterval > scaledLogInterval && scaledLogInterval > 0 {
					logsPerTick = int(tickInterval / scaledLogInterval)
				}
				for i := 0; i < logsPerTick; i++ {
					g.handle.ObserveLog(&logMessage{
						content:   []byte("connection refused: unable to reach backend service"),
						status:    "error",
						timestamp: g.baseTimestamp + int64(elapsed),
					})
				}
				lastLogTime = time.Now()
			}
		}
	}
}

// getRetransmitValue returns the retransmit metric value for the given elapsed time.
// Includes a non-correlated spike at 10-12s.
func (g *DataGenerator) getRetransmitValue(elapsed float64) float64 {
	const baseline, peak, spikeLevel = 5.0, 50.0, 30.0

	// Check for non-correlated spike first (before the main incident)
	if elapsed >= spikeRetransmitStart && elapsed < spikeRetransmitEnd {
		return g.applyNoise(g.triangleSpike(elapsed, spikeRetransmitStart, spikeRetransmitEnd, baseline, spikeLevel))
	}

	baseValue := g.getPhaseValue(elapsed, baseline, peak)
	return g.applyNoise(baseValue)
}

// getLockContentionValue returns the lock contention metric value for the given elapsed time.
// Includes a non-correlated spike at 17-19s.
func (g *DataGenerator) getLockContentionValue(elapsed float64) float64 {
	const baseline, peak, spikeLevel = 1000.0, 10000.0, 6000.0

	// Check for non-correlated spike first (before the main incident)
	if elapsed >= spikeLockStart && elapsed < spikeLockEnd {
		return g.applyNoise(g.triangleSpike(elapsed, spikeLockStart, spikeLockEnd, baseline, spikeLevel))
	}

	baseValue := g.getPhaseValue(elapsed, baseline, peak)
	return g.applyNoise(baseValue)
}

// triangleSpike generates a triangle-wave spike: ramps up to peak at midpoint, then back down.
func (g *DataGenerator) triangleSpike(elapsed, start, end, baseline, peak float64) float64 {
	duration := end - start
	midpoint := start + duration/2
	if elapsed < midpoint {
		// Ramp up
		progress := (elapsed - start) / (duration / 2)
		return baseline + (peak-baseline)*progress
	}
	// Ramp down
	progress := (elapsed - midpoint) / (duration / 2)
	return peak - (peak-baseline)*progress
}

// getPhaseValue returns the value based on current phase, interpolating during ramps.
func (g *DataGenerator) getPhaseValue(elapsed float64, baseline float64, peak float64) float64 {
	switch {
	case elapsed < phaseBaselineEnd: // Baseline: 0-25s (includes non-correlated spike periods handled elsewhere)
		return baseline
	case elapsed < phaseIncidentRampEnd: // Incident ramp: 25-30s
		progress := (elapsed - phaseBaselineEnd) / (phaseIncidentRampEnd - phaseBaselineEnd)
		return baseline + (peak-baseline)*progress
	case elapsed < phaseIncidentPeakEnd: // Incident peak: 30-45s
		return peak
	case elapsed < phaseRecoveryEnd: // Recovery: 45-50s
		progress := (elapsed - phaseIncidentPeakEnd) / (phaseRecoveryEnd - phaseIncidentPeakEnd)
		return peak - (peak-baseline)*progress
	default: // Post-incident: 50-70s+
		return baseline
	}
}

// getLogInterval returns the log emission interval for the given elapsed time.
func (g *DataGenerator) getLogInterval(elapsed float64) time.Duration {
	switch {
	case elapsed < phaseBaselineEnd: // Baseline: ~1 per 5s
		return 5 * time.Second
	case elapsed < phaseIncidentRampEnd: // Incident ramp: ~1 per 2s
		return 2 * time.Second
	case elapsed < phaseIncidentPeakEnd: // Incident peak: ~1 per 0.5s
		return 500 * time.Millisecond
	case elapsed < phaseRecoveryEnd: // Recovery: ~1 per 2s
		return 2 * time.Second
	default: // Post-incident: ~1 per 5s
		return 5 * time.Second
	}
}

// applyNoise applies random noise to a value.
// Formula: value * (1 + noise * (rand.Float64()*2 - 1))
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

func (m *metricSample) GetName() string       { return m.name }
func (m *metricSample) GetValue() float64     { return m.value }
func (m *metricSample) GetRawTags() []string  { return m.tags }
func (m *metricSample) GetTimestamp() float64 { return m.timestamp }
func (m *metricSample) GetSampleRate() float64 { return 1.0 }

// logMessage implements observer.LogView for generated logs.
type logMessage struct {
	content   []byte
	status    string
	tags      []string
	hostname  string
	timestamp int64 // simulation timestamp for consistent bucketing
}

func (l *logMessage) GetContent() []byte   { return l.content }
func (l *logMessage) GetStatus() string    { return l.status }
func (l *logMessage) GetTags() []string    { return l.tags }
func (l *logMessage) GetHostname() string  { return l.hostname }
func (l *logMessage) GetTimestamp() int64  { return l.timestamp }

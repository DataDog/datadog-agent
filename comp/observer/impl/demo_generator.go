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

// DataGenerator simulates the incident scenario by sending observations to a Handle.
type DataGenerator struct {
	handle observer.Handle
	config GeneratorConfig
	rng    *rand.Rand
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
		handle: handle,
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Run generates data until the context is cancelled.
// Timeline (at TimeScale=1.0):
// | Phase          | Time    | retransmits   | lock_contention | error logs  |
// |----------------|---------|---------------|-----------------|-------------|
// | Baseline       | 0-10s   | 5 +/- noise   | 1000 +/- noise  | ~1 per 5s   |
// | Incident ramp  | 10-15s  | 5->50 linear  | 1000->10000     | ~1 per 2s   |
// | Incident peak  | 15-25s  | 50 +/- noise  | 10000 +/- noise | ~1 per 0.5s |
// | Recovery       | 25-30s  | 50->5 linear  | 10000->1000     | ~1 per 2s   |
// | Post-incident  | 30-40s  | 5 +/- noise   | 1000 +/- noise  | ~1 per 5s   |
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

			// Generate and emit metrics
			retransmits := g.getRetransmitValue(elapsed)
			lockContention := g.getLockContentionValue(elapsed)

			g.handle.ObserveMetric(&metricSample{
				name:      "network.retransmits",
				value:     retransmits,
				timestamp: float64(time.Now().Unix()),
			})

			g.handle.ObserveMetric(&metricSample{
				name:      "ebpf.lock_contention_ns",
				value:     lockContention,
				timestamp: float64(time.Now().Unix()),
			})

			// Emit logs at phase-dependent intervals
			logInterval := g.getLogInterval(elapsed)
			scaledLogInterval := time.Duration(float64(logInterval) * g.config.TimeScale)
			if time.Since(lastLogTime) >= scaledLogInterval {
				g.handle.ObserveLog(&logMessage{
					content: []byte("connection refused: unable to reach backend service"),
					status:  "error",
				})
				lastLogTime = time.Now()
			}
		}
	}
}

// getRetransmitValue returns the retransmit metric value for the given elapsed time.
func (g *DataGenerator) getRetransmitValue(elapsed float64) float64 {
	baseValue := g.getPhaseValue(elapsed, 5, 50)
	return g.applyNoise(baseValue)
}

// getLockContentionValue returns the lock contention metric value for the given elapsed time.
func (g *DataGenerator) getLockContentionValue(elapsed float64) float64 {
	baseValue := g.getPhaseValue(elapsed, 1000, 10000)
	return g.applyNoise(baseValue)
}

// getPhaseValue returns the value based on current phase, interpolating during ramps.
func (g *DataGenerator) getPhaseValue(elapsed float64, baseline float64, peak float64) float64 {
	switch {
	case elapsed < 10: // Baseline: 0-10s
		return baseline
	case elapsed < 15: // Incident ramp: 10-15s
		progress := (elapsed - 10) / 5
		return baseline + (peak-baseline)*progress
	case elapsed < 25: // Incident peak: 15-25s
		return peak
	case elapsed < 30: // Recovery: 25-30s
		progress := (elapsed - 25) / 5
		return peak - (peak-baseline)*progress
	default: // Post-incident: 30-40s+
		return baseline
	}
}

// getLogInterval returns the log emission interval for the given elapsed time.
func (g *DataGenerator) getLogInterval(elapsed float64) time.Duration {
	switch {
	case elapsed < 10: // Baseline: ~1 per 5s
		return 5 * time.Second
	case elapsed < 15: // Incident ramp: ~1 per 2s
		return 2 * time.Second
	case elapsed < 25: // Incident peak: ~1 per 0.5s
		return 500 * time.Millisecond
	case elapsed < 30: // Recovery: ~1 per 2s
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
	content  []byte
	status   string
	tags     []string
	hostname string
}

func (l *logMessage) GetContent() []byte  { return l.content }
func (l *logMessage) GetStatus() string   { return l.status }
func (l *logMessage) GetTags() []string   { return l.tags }
func (l *logMessage) GetHostname() string { return l.hostname }

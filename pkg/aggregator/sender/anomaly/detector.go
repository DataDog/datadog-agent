// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomaly

import (
	"math"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MetricSample represents a single metric measurement
type MetricSample struct {
	Value     float64
	Timestamp float64
}

// AnomalyType represents different types of anomalies
type AnomalyType string

const (
	AnomalyTypeSpike     AnomalyType = "spike"      // Sudden increase
	AnomalyTypeDrop      AnomalyType = "drop"       // Sudden decrease
	AnomalyTypeHigh      AnomalyType = "high"       // Sustained high value
	AnomalyTypeLow       AnomalyType = "low"        // Sustained low value
	AnomalyTypeRapidRise AnomalyType = "rapid_rise" // Continuous increase
	AnomalyTypeRapidFall AnomalyType = "rapid_fall" // Continuous decrease
)

// Anomaly represents a detected anomaly
type Anomaly struct {
	MetricName string
	Type       AnomalyType
	Value      float64
	Baseline   float64
	Severity   float64 // 0.0 - 1.0
	Timestamp  float64
}

// DetectionConfig configures the anomaly detection behavior
type DetectionConfig struct {
	// Window size for storing recent samples
	WindowSize uint64

	// Spike threshold - multiplier for spike detection (e.g., 2.0 = 200% of baseline)
	SpikeThreshold float64

	// Minimum samples before detection starts
	MinSamples uint64
}

// DefaultConfig returns sensible default detection settings
func DefaultConfig() DetectionConfig {
	return DetectionConfig{
		WindowSize:     100,
		SpikeThreshold: 2.0, // 200% of baseline
		MinSamples:     20,  // 5 minutes at 15s collection interval
	}
}

// Detector is the interface for anomaly detection
type Detector interface {
	// RecordMetric records a new metric value and checks for anomalies
	RecordMetric(
		metricName string,
		value float64,
		timestamp float64,
	)

	// Clear clears all stored metric history
	Clear()

	// GetMetricHistory returns the history for a specific metric
	GetMetricHistory(metricName string) []MetricSample

	// GetConfig returns the current detection configuration
	GetConfig() DetectionConfig
}

// HeuristicDetector implements Detector using heuristic-based detection
type HeuristicDetector struct {
	config  DetectionConfig
	metrics map[string]*ringbuffer.RingBuffer[MetricSample]
	mu      sync.RWMutex

	// Callback for anomaly notifications
	onAnomaly func(Anomaly)
}

// NewHeuristicDetector creates a new heuristic-based anomaly detector
func NewHeuristicDetector(
	config DetectionConfig,
	onAnomaly func(Anomaly),
) Detector {
	return &HeuristicDetector{
		config:    config,
		metrics:   make(map[string]*ringbuffer.RingBuffer[MetricSample]),
		onAnomaly: onAnomaly,
	}
}

// RecordMetric records a new metric value and checks for anomalies
func (d *HeuristicDetector) RecordMetric(
	metricName string,
	value float64,
	timestamp float64,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	log.Infof(
		"RecordMetric(%s, %f, %f)",
		metricName,
		value,
		timestamp,
	)

	// Get or create ring buffer for this metric
	rb, exists := d.metrics[metricName]
	if !exists {
		rb = ringbuffer.NewRingBuffer[MetricSample](d.config.WindowSize)
		d.metrics[metricName] = rb
	}

	// Use current time if timestamp not provided
	if timestamp == 0 {
		timestamp = float64(time.Now().Unix())
	}

	// Add sample
	sample := MetricSample{
		Value:     value,
		Timestamp: timestamp,
	}
	rb.Push(sample)

	// Check for anomalies if we have enough samples
	if rb.Size() >= d.config.MinSamples {
		d.detectAnomalies(
			metricName,
			rb,
			sample,
		)
	}
}

// detectAnomalies analyzes the metric history and detects anomalies
func (d *HeuristicDetector) detectAnomalies(
	metricName string,
	rb *ringbuffer.RingBuffer[MetricSample],
	latest MetricSample,
) {
	samples := rb.ReadAll()
	if len(samples) == 0 {
		return
	}

	// Calculate baseline statistics
	mean, stddev := d.calculateBaseline(
		samples,
		int(rb.Size()),
	)

	// Check for spike anomalies only (relative to baseline)
	if anomaly := d.checkSpike(
		metricName,
		latest,
		mean,
		stddev,
	); anomaly != nil {
		d.notifyAnomaly(*anomaly)
	}
}

// calculateBaseline computes mean and stddev from recent samples
func (d *HeuristicDetector) calculateBaseline(
	samples []MetricSample,
	size int,
) (
	mean float64,
	stddev float64,
) {
	if size == 0 {
		return 0, 0
	}

	// Calculate mean
	sum := 0.0
	count := 0
	for i := 0; i < len(samples) && i < size; i++ {
		if samples[i].Timestamp > 0 { // Only count valid samples
			sum += samples[i].Value
			count++
		}
	}
	if count == 0 {
		return 0, 0
	}
	mean = sum / float64(count)

	// Calculate standard deviation
	variance := 0.0
	for i := 0; i < len(samples) && i < size; i++ {
		if samples[i].Timestamp > 0 {
			diff := samples[i].Value - mean
			variance += diff * diff
		}
	}
	stddev = math.Sqrt(variance / float64(count))

	return mean, stddev
}

// checkSpike detects sudden spikes in value
func (d *HeuristicDetector) checkSpike(
	metricName string,
	sample MetricSample,
	baseline float64,
	stddev float64,
) *Anomaly {
	if sample.Value > baseline*d.config.SpikeThreshold {
		severity := math.Min(
			1.0,
			(sample.Value-baseline)/math.Max(
				1.0,
				baseline,
			),
		)
		return &Anomaly{
			MetricName: metricName,
			Type:       AnomalyTypeSpike,
			Value:      sample.Value,
			Baseline:   baseline,
			Severity:   severity,
			Timestamp:  sample.Timestamp,
		}
	}
	return nil
}

// notifyAnomaly sends anomaly notification
func (d *HeuristicDetector) notifyAnomaly(anomaly Anomaly) {
	log.Infof(
		"ANOMALY DETECTED: %s - %s (value: %.2f, baseline: %.2f, severity: %.2f)",
		anomaly.MetricName,
		anomaly.Type,
		anomaly.Value,
		anomaly.Baseline,
		anomaly.Severity,
	)

	if d.onAnomaly != nil {
		d.onAnomaly(anomaly)
	}
}

// Clear clears all stored metric history
func (d *HeuristicDetector) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, rb := range d.metrics {
		rb.Clear()
	}
}

// GetMetricHistory returns the history for a specific metric
func (d *HeuristicDetector) GetMetricHistory(metricName string) []MetricSample {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rb, exists := d.metrics[metricName]
	if !exists {
		return nil
	}

	return rb.ReadAll()
}

// GetConfig returns the current detection configuration
func (d *HeuristicDetector) GetConfig() DetectionConfig {
	return d.config
}

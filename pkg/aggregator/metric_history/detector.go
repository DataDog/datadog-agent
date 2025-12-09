// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

// Anomaly represents a detected anomaly in a metric series.
type Anomaly struct {
	SeriesKey    SeriesKey
	DetectorName string
	Timestamp    int64   // when the anomaly occurred
	Type         string  // e.g., "changepoint", "spike", "drop"
	Severity     float64 // 0.0-1.0, detector-specific
	Message      string  // human-readable description
}

// Detector analyzes metric history and reports anomalies.
// Implementations should be stateless between calls.
type Detector interface {
	// Name returns a unique identifier for this detector.
	Name() string

	// Analyze examines a single series and returns any detected anomalies.
	Analyze(key SeriesKey, history *MetricHistory) []Anomaly
}

// DetectorRegistry manages multiple detectors.
type DetectorRegistry struct {
	detectors []Detector
}

// NewDetectorRegistry creates a new detector registry.
func NewDetectorRegistry() *DetectorRegistry {
	return &DetectorRegistry{
		detectors: make([]Detector, 0),
	}
}

// Register adds a detector to the registry.
func (r *DetectorRegistry) Register(d Detector) {
	r.detectors = append(r.detectors, d)
}

// RunAll runs all detectors against all series in the cache.
func (r *DetectorRegistry) RunAll(reader HistoryReader) []Anomaly {
	var anomalies []Anomaly
	reader.Scan(func(key SeriesKey, history *MetricHistory) bool {
		for _, detector := range r.detectors {
			anomalies = append(anomalies, detector.Analyze(key, history)...)
		}
		return true
	})
	return anomalies
}

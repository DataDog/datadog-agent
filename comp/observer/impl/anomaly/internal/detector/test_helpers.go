// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

import (
	"testing"
)

// Common test data fixtures

// CreateNormalResults creates a slice of "normal" telemetry results (high scores, no anomalies)
func CreateNormalResults(count int) []TelemetryResult {
	results := make([]TelemetryResult, count)
	for i := 0; i < count; i++ {
		results[i] = TelemetryResult{
			CPU:                1.0,
			Mem:                1.0,
			Err:                1.0,
			ClientSentByClient: 1.0,
			ClientSentByServer: 1.0,
			ServerSentByClient: 1.0,
			ServerSentByServer: 1.0,
			TraceP50:           1.0,
			TraceP95:           1.0,
			TraceP99:           1.0,
			Metrics:            1.0,
		}
	}
	return results
}

// CreateAnomalousResults creates a slice of "anomalous" telemetry results (low scores)
func CreateAnomalousResults(count int) []TelemetryResult {
	results := make([]TelemetryResult, count)
	for i := 0; i < count; i++ {
		results[i] = TelemetryResult{
			CPU:                0.1,
			Mem:                0.1,
			Err:                0.0,
			ClientSentByClient: 0.1,
			ClientSentByServer: 0.1,
			ServerSentByClient: 0.1,
			ServerSentByServer: 0.1,
			TraceP50:           0.1,
			TraceP95:           0.1,
			TraceP99:           0.1,
			Metrics:            0.1,
		}
	}
	return results
}

// CreateMixedResults creates normal results with an anomalous one at the end
func CreateMixedResults() []TelemetryResult {
	normal := CreateNormalResults(10)
	anomalous := CreateAnomalousResults(1)
	return append(normal, anomalous...)
}

// CreateGradualDriftResults creates results that gradually drift from normal to anomalous
func CreateGradualDriftResults(count int) []TelemetryResult {
	results := make([]TelemetryResult, count)
	for i := 0; i < count; i++ {
		// Gradually decrease from 1.0 to 0.1
		value := 1.0 - (0.9 * float64(i) / float64(count-1))
		results[i] = TelemetryResult{
			CPU:                value,
			Mem:                value,
			Err:                value,
			ClientSentByClient: value,
			ClientSentByServer: value,
			ServerSentByClient: value,
			ServerSentByServer: value,
			TraceP50:           value,
			TraceP95:           value,
			TraceP99:           value,
			Metrics:            value,
		}
	}
	return results
}

// TestDetectorInterface tests that a detector properly implements the Detector interface
func TestDetectorInterface(t *testing.T, d Detector) {
	// Test Name method
	name := d.Name()
	if name == "" {
		t.Error("Detector Name() should not return empty string")
	}

	// Test HigherIsAnomalous method
	_ = d.HigherIsAnomalous() // Just ensure it doesn't panic

	// Test ComputeScore with normal results
	normalResults := CreateNormalResults(10)
	var normalScore float64
	for _, result := range normalResults {
		score, err := d.ComputeScore(result)
		if err != nil {
			t.Errorf("ComputeScore with normal result failed: %v", err)
		}
		normalScore = score // Keep last score
	}

	// Test ComputeScore with anomalous results
	anomalousResults := CreateAnomalousResults(10)
	var anomalousScore float64
	for _, result := range anomalousResults {
		score, err := d.ComputeScore(result)
		if err != nil {
			t.Errorf("ComputeScore with anomalous result failed: %v", err)
		}
		anomalousScore = score // Keep last score
	}

	// Verify the detector correctly indicates which direction is anomalous
	if d.HigherIsAnomalous() {
		// For detectors where higher is anomalous, anomalous score should be >= normal score
		if anomalousScore < normalScore {
			t.Logf("Warning: For %s (HigherIsAnomalous=true), anomalous score (%.3f) is less than normal score (%.3f)",
				name, anomalousScore, normalScore)
		}
	} else {
		// For detectors where lower is anomalous, anomalous score should be <= normal score
		if anomalousScore > normalScore {
			t.Logf("Warning: For %s (HigherIsAnomalous=false), anomalous score (%.3f) is greater than normal score (%.3f)",
				name, anomalousScore, normalScore)
		}
	}

	t.Logf("%s: Normal score=%.3f, Anomalous score=%.3f, HigherIsAnomalous=%v",
		name, normalScore, anomalousScore, d.HigherIsAnomalous())
}

// AssertFloatInRange checks if a float is within the expected range
func AssertFloatInRange(t *testing.T, value, min, max float64, name string) {
	if value < min || value > max {
		t.Errorf("%s: expected value in range [%.3f, %.3f], got %.3f", name, min, max, value)
	}
}

// AssertNoError checks that an error is nil
func AssertNoError(t *testing.T, err error, message string) {
	if err != nil {
		t.Errorf("%s: unexpected error: %v", message, err)
	}
}

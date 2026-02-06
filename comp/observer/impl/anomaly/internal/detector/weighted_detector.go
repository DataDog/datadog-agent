// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

// WeightedDetector computes a weighted average score from telemetry results
// It treats network metrics as one category and trace metrics as another
type WeightedDetector struct{}

// NewWeightedDetector creates a new WeightedDetector
func NewWeightedDetector() *WeightedDetector {
	return &WeightedDetector{}
}

// ComputeScore calculates a weighted average score from the telemetry result
// It groups metrics into 6 equal-weight categories:
// 1. CPU
// 2. Memory
// 3. Errors
// 4. Network (average of 4 network metrics)
// 5. Trace (average of 3 trace percentiles)
// 6. Custom Metrics
func (wd *WeightedDetector) ComputeScore(result TelemetryResult) (float64, error) {

	// Calculate average for network metrics (all network = one category)
	networkAvg := (result.ClientSentByClient + result.ClientSentByServer +
		result.ServerSentByClient + result.ServerSentByServer) / 4

	// Calculate average for trace metrics (all trace = one category)
	traceAvg := (result.TraceP50 + result.TraceP95 + result.TraceP99) / 3

	// Create score with equal weight for each category:
	// cpu, mem, err, network (avg), trace (avg), metrics
	score := (result.CPU + result.Mem + result.Err + networkAvg + traceAvg + result.Metrics) / 6

	return score, nil
}

// Name returns the name of this detector
func (wd *WeightedDetector) Name() string {
	return "Weighted"
}

// HigherIsAnomalous returns false since lower weighted scores (closer to 0) indicate anomalies
func (wd *WeightedDetector) HigherIsAnomalous() bool {
	return false
}

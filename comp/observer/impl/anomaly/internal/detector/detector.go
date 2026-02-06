// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package detector

import "fmt"

// TelemetryResult represents normalized telemetry metrics
type TelemetryResult struct {
	CPU                float64
	Mem                float64
	Err                float64
	ClientSentByClient float64
	ClientSentByServer float64
	ServerSentByClient float64
	ServerSentByServer float64
	TraceP50           float64
	TraceP95           float64
	TraceP99           float64
	Metrics            float64
}

func (t TelemetryResult) String() string {
	return fmt.Sprintf("cpu:%.3f mem:%.3f err:%.3f CC:%.3f CS:%.3f SC:%.3f SS:%.3f p50:%.3f p95:%.3f p99:%.3f metrics:%.3f",
		t.CPU, t.Mem, t.Err, t.ClientSentByClient, t.ClientSentByServer, t.ServerSentByClient, t.ServerSentByServer,
		t.TraceP50, t.TraceP95, t.TraceP99, t.Metrics)
}

func (t TelemetryResult) ToArray() []float64 {
	return []float64{
		t.CPU,
		t.Mem,
		t.Err,
		t.ClientSentByClient,
		t.ClientSentByServer,
		t.ServerSentByClient,
		t.ServerSentByServer,
		t.TraceP50,
		t.TraceP95,
		t.TraceP99,
		t.Metrics,
	}
}

// Detector is an interface for anomaly detection algorithms
type Detector interface {
	// ComputeScore computes an anomaly score from a telemetry result
	ComputeScore(result TelemetryResult) (float64, error)

	// Name returns the name of the detector
	Name() string

	// HigherIsAnomalous returns true if higher scores indicate anomalies,
	// false if lower scores indicate anomalies
	HigherIsAnomalous() bool
}

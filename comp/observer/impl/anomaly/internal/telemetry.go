// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector provides telemetry data collection and aggregation.
package collector

import (
	"fmt"
	"time"
)

// NetworkMetrics represents network monitoring data
type NetworkMetrics struct {
	BytesSentByClient   int64
	BytesSentByServer   int64
	PacketsSentByClient int64
	PacketsSentByServer int64
}

// TraceMetrics represents APM trace metrics
type TraceMetrics struct {
	P50Duration float64
	P95Duration float64
	P99Duration float64
}

// MetricTimeseries contains the time series data for a metric
type MetricTimeseries struct {
	MetricName string
	Average    float64 // Average value over the time period
}

type Telemetry struct {
	Time          time.Time
	CPU           TelemetrySignal
	Memory        TelemetrySignal
	Error         TelemetrySignal
	NetworkClient NetworkMetrics
	NetworkServer NetworkMetrics
	Trace         TraceMetrics
	Metrics       []MetricTimeseries
}

func (t Telemetry) String() string {
	return fmt.Sprintf("%v\n%v\n%v\n", t.CPU, t.Memory, t.Error)
}

type ComparisonMode struct {
	UseCPUMemV2  bool
	UseErrorV2   bool
	UseMetricV2  bool
	UseNetworkV2 bool
	UseTraceV2   bool
}

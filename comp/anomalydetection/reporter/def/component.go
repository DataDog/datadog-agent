// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reporter provides the injectable reporter component for the observer.
// It is intentionally standalone — reporter/def has no dependency on observer/def.
// The observer/impl converts its internal types to ReportOutput before calling Report.
package reporter

// team: q-branch

// ReportOutput carries the data reporters receive after each detection cycle.
type ReportOutput struct {
	// AdvancedToSec is the data time the engine advanced to.
	AdvancedToSec int64
	// NewAnomalies are anomalies detected in this advance cycle.
	NewAnomalies []Anomaly
	// ActiveCorrelations are the current correlation patterns.
	ActiveCorrelations []ActiveCorrelation
}

// Anomaly is the reporter's view of a detected anomaly.
type Anomaly struct {
	DetectorName string
	Title        string
	Description  string
	Timestamp    int64
	Score        *float64
	SeriesName   string
	Tags         []string
}

// ActiveCorrelation is the reporter's view of a detected correlation pattern.
type ActiveCorrelation struct {
	Pattern     string
	Title       string
	MemberCount int
	FirstSeen   int64
	LastUpdated int64
}

// Component is the injectable reporter.
// The live implementation sends Datadog events; the testbench pushes to SSE clients.
type Component interface {
	// Name returns the reporter name for identification.
	Name() string
	// Report is called by the observer after each detection cycle.
	Report(output ReportOutput)
}

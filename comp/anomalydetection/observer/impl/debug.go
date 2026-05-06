// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// DebugView is a read-only introspection surface implemented by observerImpl.
// It is defined in observer/impl (not observer/def) so production code is never
// coupled to the debug surface. The testbench obtains it via type assertion:
//
//	debug := obs.(observerimpl.DebugView)
type DebugView interface {
	StateView() StateView
	CatalogEntries() []CatalogEntry
	// Flush blocks until all observations queued in the dispatch channel have
	// been processed by the engine. The testbench calls this after feeding
	// parquet data to ensure StateView reflects all ingested observations.
	Flush()
}

// StateView is a read-only window into engine state.
// All methods correspond to existing methods on the unexported stateView struct
// in stateview.go — they are being promoted to a public interface.
type StateView interface {
	// Storage
	ListSeries(filter observerdef.SeriesFilter) []observerdef.SeriesMeta
	GetSeriesRange(ref observerdef.SeriesRef, start, end int64, agg observerdef.Aggregate) *observerdef.Series
	ScenarioBounds() (start, end int64, ok bool)

	// Anomalies
	Anomalies() []observerdef.Anomaly
	TotalAnomalyCount() int
	UniqueAnomalySourceCount() int
	DetectorAnomalies(name string) []observerdef.Anomaly
	AnomaliesByDetector() map[string][]observerdef.Anomaly
	AnomaliesForSource(sd observerdef.SeriesDescriptor) []observerdef.Anomaly

	// Correlations
	ActiveCorrelations() []observerdef.ActiveCorrelation
	CorrelationHistory() []observerdef.ActiveCorrelation

	// Detector / correlator metadata
	ListDetectors() []ComponentStateInfo
	ListCorrelators() []ComponentStateInfo

	// Telemetry
	Telemetry() []observerdef.ObserverTelemetry

	// Timing
	LastAnalyzedTime() int64
	LatestDataTime() int64
}

// ComponentStateInfo describes a component currently active in the engine.
type ComponentStateInfo struct {
	Name    string
	Enabled bool
}

// Ensure observerImpl satisfies DebugView at compile time.
var _ DebugView = (*observerImpl)(nil)

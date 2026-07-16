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
	// EffectiveComponentConfigs returns every catalog component's resolved
	// enabled state and hyperparameters after defaults and overrides are applied.
	EffectiveComponentConfigs() (map[string]map[string]any, error)
	// Flush blocks until all observations queued in the dispatch channel have
	// been processed by the engine. The testbench calls this after feeding
	// parquet data to ensure StateView reflects all ingested observations.
	Flush()
	// Reset clears all engine state, resets storage, and reconfigures components.
	// storageCfg is forwarded to the engine so callers control retention policy
	// (e.g. testbench passes StorageConfig{PointRetentionSecs: 0} for unbounded replay storage).
	Reset(settings ComponentSettings, storageCfg StorageConfig)
	// GetReplayProgress returns lock-free replay progress counters.
	GetReplayProgress() ReplayProgress
	// SetReplayPhase updates the replay phase string for progress reporting.
	SetReplayPhase(phase string)
	// ExtractorCount returns the number of extractors active in the engine.
	ExtractorCount() int
	// AddTelemetry writes a data point into the engine's telemetry namespace.
	// Used by the testbench to store per-detector timing stats for UI display.
	AddTelemetry(name string, value float64, timestamp int64, tags []string)
	// ReplayStoredData resets analysis state (preserving extractor context)
	// then replays all data currently in storage through the scheduler in
	// chronological order. Call after Flush().
	ReplayStoredData()
	// StorageReader returns a read-only view of the engine's time-series storage.
	// Used by the testbench to compute windowed log rates in change messages.
	StorageReader() observerdef.StorageReader
	// IngestMetricSync feeds a metric directly into the engine, bypassing
	// the dispatch channel. Synchronous: returns after IngestMetric and any
	// scheduler-triggered advances complete. Testbench-only.
	IngestMetricSync(source string, sample observerdef.MetricView)
	// IngestLogAndAdvance feeds a log directly into the engine and synchronously
	// executes scheduler-triggered advances. Used by one-shot headless streams.
	IngestLogAndAdvance(source string, msg observerdef.LogView)
	// FinishReplayStream flushes the scheduler at end-of-input without resetting
	// analysis state or replaying retained storage.
	FinishReplayStream()
	// IngestLogForReplay feeds a log directly into the engine without driving
	// any scheduler-triggered advances. Used while pre-loading retained data so
	// that extractor state is built up and log
	// metrics are written to storage, but detector/correlator advances are
	// deferred to the subsequent ReplayStoredData call.
	IngestLogForReplay(source string, msg observerdef.LogView)
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

	// Scoring
	ScoreState() observerdef.AnomalyScoreState
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

	// Timing
	LastAnalyzedTime() int64
	LatestDataTime() int64
	MaxTimestamp() int64

	// Storage stats (excluding a given namespace, typically TelemetryNamespace)
	TotalSeriesCount(excludeNamespace string) int
	TotalSampleCount(excludeNamespace string) int64

	// GetSeriesAll returns all points for a series.
	GetSeriesAll(ref observerdef.SeriesRef, agg observerdef.Aggregate) *observerdef.Series
}

// ComponentStateInfo describes a component currently active in the engine.
type ComponentStateInfo struct {
	Name    string
	Enabled bool
}

// Ensure observerImpl satisfies DebugView at compile time.
var _ DebugView = (*observerImpl)(nil)

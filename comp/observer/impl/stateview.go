// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// stateView provides read-only access to engine state.
// This is the primary introspection surface for consumers like the testbench UI.
// It computes views over existing engine state rather than maintaining a separate copy.
type stateView struct {
	engine *engine
}

// StateView returns a stateView backed by this engine's current state.
func (e *engine) StateView() *stateView {
	return &stateView{engine: e}
}

// --- Storage access ---

// ListSeries returns metadata for all series matching the filter.
func (sv *stateView) ListSeries(filter observerdef.SeriesFilter) []observerdef.SeriesMeta {
	return sv.engine.storage.ListSeries(filter)
}

// GetSeriesRange returns points within a time range for a series.
func (sv *stateView) GetSeriesRange(handle observerdef.SeriesHandle, start, end int64, agg observerdef.Aggregate) *observerdef.Series {
	return sv.engine.storage.GetSeriesRange(handle, start, end, agg)
}

// ScenarioBounds returns the time bounds of stored data.
func (sv *stateView) ScenarioBounds() (start int64, end int64, ok bool) {
	return sv.engine.storage.TimeBounds()
}

// --- Anomaly access ---

// Anomalies returns a copy of all currently tracked raw anomalies.
func (sv *stateView) Anomalies() []observerdef.Anomaly {
	return sv.engine.RawAnomalies()
}

// TotalAnomalyCount returns the total number of anomalies ever detected.
func (sv *stateView) TotalAnomalyCount() int {
	return sv.engine.TotalAnomalyCount()
}

// UniqueAnomalySourceCount returns the number of unique metric sources that had anomalies.
func (sv *stateView) UniqueAnomalySourceCount() int {
	return sv.engine.UniqueAnomalySourceCount()
}

// --- Detector info ---

// detectorInfo describes a detector registered with the engine.
type detectorInfo struct {
	Name    string
	Enabled bool
}

// ListDetectors returns info about all detectors currently in the engine.
func (sv *stateView) ListDetectors() []detectorInfo {
	sv.engine.mu.RLock()
	defer sv.engine.mu.RUnlock()

	result := make([]detectorInfo, len(sv.engine.detectors))
	for i, d := range sv.engine.detectors {
		result[i] = detectorInfo{
			Name:    d.Name(),
			Enabled: true, // detectors in the engine are always enabled
		}
	}
	return result
}

// DetectorAnomalies returns anomalies from a specific detector, filtered from the
// raw anomaly set. Computes on read rather than maintaining a per-detector index.
func (sv *stateView) DetectorAnomalies(name string) []observerdef.Anomaly {
	all := sv.engine.RawAnomalies()
	var result []observerdef.Anomaly
	for _, a := range all {
		if a.DetectorName == name {
			result = append(result, a)
		}
	}
	return result
}

// AnomaliesByDetector returns all anomalies grouped by detector name.
// Computes on read from the raw anomaly set.
func (sv *stateView) AnomaliesByDetector() map[string][]observerdef.Anomaly {
	all := sv.engine.RawAnomalies()
	result := make(map[string][]observerdef.Anomaly)
	for _, a := range all {
		result[a.DetectorName] = append(result[a.DetectorName], a)
	}
	return result
}

// AnomaliesForSeries returns anomalies associated with a specific series ID.
// Computes on read from the raw anomaly set.
func (sv *stateView) AnomaliesForSeries(id observerdef.SeriesID) []observerdef.Anomaly {
	all := sv.engine.RawAnomalies()
	var result []observerdef.Anomaly
	for _, a := range all {
		if a.SourceSeriesID == id {
			result = append(result, a)
		}
	}
	return result
}

// --- Correlator info ---

// correlatorInfo describes a correlator registered with the engine.
type correlatorInfo struct {
	Name    string
	Enabled bool
}

// ListCorrelators returns info about all correlators currently in the engine.
func (sv *stateView) ListCorrelators() []correlatorInfo {
	sv.engine.mu.RLock()
	defer sv.engine.mu.RUnlock()

	result := make([]correlatorInfo, len(sv.engine.correlators))
	for i, c := range sv.engine.correlators {
		result[i] = correlatorInfo{
			Name:    c.Name(),
			Enabled: true, // correlators in the engine are always enabled
		}
	}
	return result
}

// ActiveCorrelations returns current sliding-window correlations from all correlators.
func (sv *stateView) ActiveCorrelations() []observerdef.ActiveCorrelation {
	sv.engine.mu.RLock()
	defer sv.engine.mu.RUnlock()

	var result []observerdef.ActiveCorrelation
	for _, c := range sv.engine.correlators {
		result = append(result, c.ActiveCorrelations()...)
	}
	return result
}

// CorrelationHistory returns all correlations ever detected across the full run.
// Unlike ActiveCorrelations, this preserves correlations that correlators have
// evicted from their sliding windows, making it suitable for replay/testbench/headless use.
// It merges the accumulated history with current correlator state so that
// correlations injected outside the normal Advance flow are also visible.
func (sv *stateView) CorrelationHistory() []observerdef.ActiveCorrelation {
	sv.engine.mu.RLock()
	defer sv.engine.mu.RUnlock()

	accumulated := sv.engine.AccumulatedCorrelations()
	seen := make(map[string]bool, len(accumulated))
	for _, ac := range accumulated {
		seen[ac.Pattern] = true
	}

	// Include any current correlator state not yet in the accumulated set.
	for _, c := range sv.engine.correlators {
		for _, ac := range c.ActiveCorrelations() {
			if !seen[ac.Pattern] {
				accumulated = append(accumulated, ac)
				seen[ac.Pattern] = true
			}
		}
	}

	return accumulated
}

// --- Telemetry ---

// Telemetry returns accumulated telemetry from detection runs.
func (sv *stateView) Telemetry() []observerdef.ObserverTelemetry {
	sv.engine.telemetryMu.RLock()
	defer sv.engine.telemetryMu.RUnlock()

	result := make([]observerdef.ObserverTelemetry, len(sv.engine.accumulatedTelemetry))
	copy(result, sv.engine.accumulatedTelemetry)
	return result
}

// --- Scheduling state ---

// LastAnalyzedTime returns the data timestamp up to which detection has run.
func (sv *stateView) LastAnalyzedTime() int64 {
	sv.engine.mu.RLock()
	defer sv.engine.mu.RUnlock()
	return sv.engine.lastAnalyzedDataTime
}

// LatestDataTime returns the latest data timestamp seen across all ingested observations.
func (sv *stateView) LatestDataTime() int64 {
	sv.engine.mu.RLock()
	defer sv.engine.mu.RUnlock()
	return sv.engine.latestDataTime
}

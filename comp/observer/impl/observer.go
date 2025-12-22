// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl implements the observer component.
package observerimpl

import (
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// Requires declares the input types to the observer component constructor.
type Requires struct {
	// No dependencies yet - can add Lifecycle, Config, Log as needed
}

// Provides defines the output of the observer component.
type Provides struct {
	Comp observerdef.Component
}

// NewComponent creates an observer.Component.
func NewComponent(deps Requires) Provides {
	obs := &observerImpl{
		analyses: []observerdef.Analysis{
			&BadDetector{},
		},
		tsAnalyses: []observerdef.TimeSeriesAnalysis{
			&SpikeDetector{},
		},
		storage: newTimeSeriesStorage(),
	}
	return Provides{Comp: obs}
}

// observerImpl is the implementation of the observer component.
type observerImpl struct {
	analyses   []observerdef.Analysis
	tsAnalyses []observerdef.TimeSeriesAnalysis
	storage    *timeSeriesStorage
}

// GetHandle returns a lightweight handle for a named source.
func (o *observerImpl) GetHandle(name string) observerdef.Handle {
	return &handle{observer: o, source: name}
}

// handle is the lightweight observation interface passed to other components.
type handle struct {
	observer *observerImpl
	source   string
}

// ObserveMetric observes a DogStatsD metric sample.
func (h *handle) ObserveMetric(sample observerdef.MetricView) {
	// Add to storage
	timestamp := int64(sample.GetTimestamp())
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	series := h.observer.storage.Add(
		h.source,
		sample.GetName(),
		sample.GetValue(),
		timestamp,
		sample.GetRawTags(),
	)

	// Run time series analyses
	h.runTSAnalyses(series)
}

// ObserveLog observes a log message.
func (h *handle) ObserveLog(msg observerdef.LogView) {
	timestamp := time.Now().Unix()

	for _, analysis := range h.observer.analyses {
		result := analysis.Analyze(msg)

		// Add metrics from log analysis to storage
		for _, m := range result.Metrics {
			series := h.observer.storage.Add(h.source, m.Name, m.Value, timestamp, m.Tags)
			h.runTSAnalyses(series)
		}

		// TODO: forward anomalies to appropriate destination
		_ = result.Anomalies
	}
}

// runTSAnalyses runs all time series analyses on a series.
func (h *handle) runTSAnalyses(series *observerdef.SeriesStats) {
	for _, tsAnalysis := range h.observer.tsAnalyses {
		result := tsAnalysis.Analyze(series)
		// TODO: forward anomalies to appropriate destination
		_ = result.Anomalies
	}
}

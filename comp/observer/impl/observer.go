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
		logAnalyses: []observerdef.LogAnalysis{
			&BadDetector{},
		},
		tsAnalyses: []observerdef.TimeSeriesAnalysis{
			&SpikeDetector{},
		},
		consumers: []observerdef.AnomalyConsumer{
			&MemoryConsumer{},
		},
		storage: newTimeSeriesStorage(),
	}
	return Provides{Comp: obs}
}

// observerImpl is the implementation of the observer component.
type observerImpl struct {
	logAnalyses []observerdef.LogAnalysis
	tsAnalyses  []observerdef.TimeSeriesAnalysis
	consumers   []observerdef.AnomalyConsumer
	storage     *timeSeriesStorage
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
	timestamp := int64(sample.GetTimestamp())
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	name := sample.GetName()
	tags := sample.GetRawTags()

	// Add to storage
	h.observer.storage.Add(h.source, name, sample.GetValue(), timestamp, tags)

	// Run time series analyses (using average aggregation)
	if series := h.observer.storage.GetSeries(h.source, name, tags, AggregateAverage); series != nil {
		h.runTSAnalyses(*series)
	}
}

// ObserveLog observes a log message.
func (h *handle) ObserveLog(msg observerdef.LogView) {
	timestamp := time.Now().Unix()

	for _, analysis := range h.observer.logAnalyses {
		result := analysis.Analyze(msg)

		// Add metrics from log analysis to storage, then run TS analyses
		for _, m := range result.Metrics {
			h.observer.storage.Add(h.source, m.Name, m.Value, timestamp, m.Tags)
			if series := h.observer.storage.GetSeries(h.source, m.Name, m.Tags, AggregateAverage); series != nil {
				h.runTSAnalyses(*series)
			}
		}

		// Forward anomalies to consumers
		for _, anomaly := range result.Anomalies {
			h.consumeAnomaly(anomaly)
		}
	}
}

// runTSAnalyses runs all time series analyses on a series.
func (h *handle) runTSAnalyses(series observerdef.Series) {
	for _, tsAnalysis := range h.observer.tsAnalyses {
		result := tsAnalysis.Analyze(series)
		// Forward anomalies to consumers
		for _, anomaly := range result.Anomalies {
			h.consumeAnomaly(anomaly)
		}
	}
}

// consumeAnomaly sends an anomaly to all registered consumers.
func (h *handle) consumeAnomaly(anomaly observerdef.AnomalyOutput) {
	for _, consumer := range h.observer.consumers {
		consumer.Consume(anomaly)
	}
}

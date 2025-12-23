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

// observation is a message sent from handles to the observer.
type observation struct {
	source string
	metric *metricObs
	log    *logObs
}

// metricObs contains copied metric data.
type metricObs struct {
	name      string
	value     float64
	tags      []string
	timestamp int64
}

// logObs contains copied log data.
type logObs struct {
	content   []byte
	status    string
	tags      []string
	hostname  string
	timestamp int64
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
		obsCh:   make(chan observation, 1000),
	}
	go obs.run()
	return Provides{Comp: obs}
}

// observerImpl is the implementation of the observer component.
type observerImpl struct {
	logAnalyses []observerdef.LogAnalysis
	tsAnalyses  []observerdef.TimeSeriesAnalysis
	consumers   []observerdef.AnomalyConsumer
	storage     *timeSeriesStorage
	obsCh       chan observation
}

// run is the main dispatch loop, processing all observations sequentially.
func (o *observerImpl) run() {
	for obs := range o.obsCh {
		if obs.metric != nil {
			o.processMetric(obs.source, obs.metric)
		}
		if obs.log != nil {
			o.processLog(obs.source, obs.log)
		}
	}
}

// processMetric handles a metric observation.
func (o *observerImpl) processMetric(source string, m *metricObs) {
	// Add to storage
	o.storage.Add(source, m.name, m.value, m.timestamp, m.tags)

	// Run time series analyses (using average aggregation)
	if series := o.storage.GetSeries(source, m.name, m.tags, AggregateAverage); series != nil {
		o.runTSAnalyses(*series)
	}

	o.reportAll()
}

// processLog handles a log observation.
func (o *observerImpl) processLog(source string, l *logObs) {
	// Create a view for analyses
	view := &logView{obs: l}

	for _, analysis := range o.logAnalyses {
		result := analysis.Analyze(view)

		// Add metrics from log analysis to storage, then run TS analyses
		for _, m := range result.Metrics {
			o.storage.Add(source, m.Name, m.Value, l.timestamp, m.Tags)
			if series := o.storage.GetSeries(source, m.Name, m.Tags, AggregateAverage); series != nil {
				o.runTSAnalyses(*series)
			}
		}

		// Forward anomalies to consumers
		for _, anomaly := range result.Anomalies {
			o.consumeAnomaly(anomaly)
		}
	}

	o.reportAll()
}

// runTSAnalyses runs all time series analyses on a series.
func (o *observerImpl) runTSAnalyses(series observerdef.Series) {
	for _, tsAnalysis := range o.tsAnalyses {
		result := tsAnalysis.Analyze(series)
		for _, anomaly := range result.Anomalies {
			o.consumeAnomaly(anomaly)
		}
	}
}

// consumeAnomaly sends an anomaly to all registered consumers.
func (o *observerImpl) consumeAnomaly(anomaly observerdef.AnomalyOutput) {
	for _, consumer := range o.consumers {
		consumer.Consume(anomaly)
	}
}

// reportAll calls Report() on all consumers.
func (o *observerImpl) reportAll() {
	for _, consumer := range o.consumers {
		consumer.Report()
	}
}

// GetHandle returns a lightweight handle for a named source.
func (o *observerImpl) GetHandle(name string) observerdef.Handle {
	return &handle{ch: o.obsCh, source: name}
}

// handle is the lightweight observation interface passed to other components.
// It only holds a channel and source name - all processing happens in the observer.
type handle struct {
	ch     chan<- observation
	source string
}

// ObserveMetric observes a DogStatsD metric sample.
func (h *handle) ObserveMetric(sample observerdef.MetricView) {
	timestamp := int64(sample.GetTimestamp())
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}

	h.ch <- observation{
		source: h.source,
		metric: &metricObs{
			name:      sample.GetName(),
			value:     sample.GetValue(),
			tags:      copyTags(sample.GetRawTags()),
			timestamp: timestamp,
		},
	}
}

// ObserveLog observes a log message.
func (h *handle) ObserveLog(msg observerdef.LogView) {
	h.ch <- observation{
		source: h.source,
		log: &logObs{
			content:   copyBytes(msg.GetContent()),
			status:    msg.GetStatus(),
			tags:      copyTags(msg.GetTags()),
			hostname:  msg.GetHostname(),
			timestamp: time.Now().Unix(),
		},
	}
}

// logView wraps logObs to implement LogView interface.
type logView struct {
	obs *logObs
}

func (v *logView) GetContent() []byte  { return v.obs.content }
func (v *logView) GetStatus() string   { return v.obs.status }
func (v *logView) GetTags() []string   { return v.obs.tags }
func (v *logView) GetHostname() string { return v.obs.hostname }

// copyBytes creates a copy of a byte slice.
func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	result := make([]byte, len(b))
	copy(result, b)
	return result
}

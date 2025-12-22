// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl implements the observer component.
package observerimpl

import (
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
	}
	return Provides{Comp: obs}
}

// observerImpl is the implementation of the observer component.
type observerImpl struct {
	analyses []observerdef.Analysis
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
	// Placeholder - sampling/storage logic goes here later
}

// ObserveLog observes a log message.
func (h *handle) ObserveLog(msg observerdef.LogView) {
	for _, analysis := range h.observer.analyses {
		result := analysis.Analyze(msg)
		// TODO: forward metrics and anomalies to appropriate destinations
		_ = result
	}
}

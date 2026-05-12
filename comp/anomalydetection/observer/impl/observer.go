// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl implements the observer component.
package observerimpl

import (
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

type component struct{}

type handle struct{}

// NewComponent creates a new observer component.
func NewComponent() observer.Component {
	return component{}
}

// GetHandle returns a no-op handle for the named source.
func (component) GetHandle(_ string) observer.Handle {
	return handle{}
}

// DumpMetrics writes all stored metrics to the specified file (for debugging).
func (component) DumpMetrics(_ string) error {
	return nil
}

// ObserveMetric observes a DogStatsD metric sample.
func (handle) ObserveMetric(_ observer.MetricView) {}

// ObserveLog observes a log message.
func (handle) ObserveLog(_ observer.LogView) {}

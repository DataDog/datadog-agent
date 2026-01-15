// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fgmimpl

import observer "github.com/DataDog/datadog-agent/comp/observer/def"

var _ observer.MetricView = (*fgmMetricView)(nil)

type fgmMetricView struct {
	name      string
	value     float64
	tags      []string
	timestamp float64
}

// GetName returns the metric name
func (m *fgmMetricView) GetName() string {
	return m.name
}

// GetValue returns the metric value
func (m *fgmMetricView) GetValue() float64 {
	return m.value
}

// GetRawTags returns the metric tags
func (m *fgmMetricView) GetRawTags() []string {
	return m.tags
}

// GetTimestamp returns the metric timestamp
func (m *fgmMetricView) GetTimestamp() float64 {
	return m.timestamp
}

// GetSampleRate returns the sample rate (always 1.0 for FGM metrics)
func (m *fgmMetricView) GetSampleRate() float64 {
	return 1.0
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

// Gauge tracks the value of one health metric of the Agent.
type Gauge interface {
	// Set stores the value for the given tags.
	Set(value float64, tagsValue ...string)
	// Inc increments the Gauge value.
	Inc(tagsValue ...string)
	// Dec decrements the Gauge value.
	Dec(tagsValue ...string)
	// Add adds the value to the Gauge value.
	Add(value float64, tagsValue ...string)
	// Sub subtracts the value to the Gauge value.
	Sub(value float64, tagsValue ...string)
	// Delete deletes the value for the Gauge with the given tags.
	Delete(tagsValue ...string)
	// DeletePartialMatch deletes the values for the Gauges that match the subset of given tags
	DeletePartialMatch(tags map[string]string)
	// WithValues returns SimpleGauge for this metric with the given tag values.
	WithValues(tagsValue ...string) SimpleGauge
	// WithTags returns SimpleGauge for this metric with the given tag values.
	WithTags(tags map[string]string) SimpleGauge
}

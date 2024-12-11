// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

// Histogram tracks the value of one health metric of the Agent.
type Histogram interface {
	// Observe the value to the Histogram value.
	Observe(value float64, tagsValue ...string)
	// Delete deletes the value for the Histogram with the given tags.
	Delete(tagsValue ...string)
	// WithValues returns SimpleHistogram for this metric with the given tag values.
	WithValues(tagsValue ...string) SimpleHistogram
	// WithTags returns SimpleHistogram for this metric with the given tag values.
	WithTags(tags map[string]string) SimpleHistogram
}

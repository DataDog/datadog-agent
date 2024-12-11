// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package noopsimpl

import "github.com/DataDog/datadog-agent/comp/core/telemetry"

// Prometheus implements histograms using Prometheus.
type slsHistogram struct{}

// Observe samples the value for the given tags.
func (h *slsHistogram) Observe(float64, ...string) {}

// Delete deletes the value for the Histogram with the given tags.
func (h *slsHistogram) Delete(...string) {}

// WithValues returns SimpleHistogram for this metric with the given tag values.
func (h *slsHistogram) WithValues(...string) telemetry.SimpleHistogram {
	// Prometheus does not directly expose the underlying histogram so we have to cast it.
	return &simpleNoOpHistogram{}
}

// WithValues returns SimpleHistogram for this metric with the given tag values.
func (h *slsHistogram) WithTags(map[string]string) telemetry.SimpleHistogram {
	// Prometheus does not directly expose the underlying histogram so we have to cast it.
	return &simpleNoOpHistogram{}
}

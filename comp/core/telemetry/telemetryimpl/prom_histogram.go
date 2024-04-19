// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetryimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/prometheus/client_golang/prometheus"
)

// Prometheus implements histograms using Prometheus.
type promHistogram struct {
	ph *prometheus.HistogramVec
}

// Observe samples the value for the given tags.
func (h *promHistogram) Observe(value float64, tagsValue ...string) {
	h.ph.WithLabelValues(tagsValue...).Observe(value)
}

// Delete deletes the value for the Histogram with the given tags.
func (h *promHistogram) Delete(tagsValue ...string) {
	h.ph.DeleteLabelValues(tagsValue...)
}

// WithValues returns SimpleHistogram for this metric with the given tag values.
func (h *promHistogram) WithValues(tagsValue ...string) telemetry.SimpleHistogram {
	// Prometheus does not directly expose the underlying histogram so we have to cast it.
	return &simplePromHistogram{h: h.ph.WithLabelValues(tagsValue...).(prometheus.Histogram)}
}

// WithValues returns SimpleHistogram for this metric with the given tag values.
func (h *promHistogram) WithTags(tags map[string]string) telemetry.SimpleHistogram {
	// Prometheus does not directly expose the underlying histogram so we have to cast it.
	return &simplePromHistogram{h: h.ph.With(tags).(prometheus.Histogram)}
}

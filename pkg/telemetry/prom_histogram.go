// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
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

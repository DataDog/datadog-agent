// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Prometheus implements summaries using Prometheus.
type promSummary struct {
	ps *prometheus.SummaryVec
}

// Observe samples the value for the given tags.
func (s *promSummary) Observe(value float64, tagsValue ...string) {
	s.ps.WithLabelValues(tagsValue...).Observe(value)
}

// Delete deletes the value for the summary with the given tags.
func (s *promSummary) Delete(tagsValue ...string) {
	s.ps.DeleteLabelValues(tagsValue...)
}

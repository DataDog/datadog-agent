// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetryimpl

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// Counter implementation using Prometheus.
type simplePromCounter struct {
	c prometheus.Counter
}

// Inc increments the counter.
func (s *simplePromCounter) Inc() {
	s.c.Inc()
}

// Add increments the counter by given amount.
func (s *simplePromCounter) Add(val float64) {
	s.c.Add(val)
}

// Get gets the current counter value
func (s *simplePromCounter) Get() float64 {
	metric := &dto.Metric{}
	_ = s.c.Write(metric)
	return *metric.Counter.Value
}

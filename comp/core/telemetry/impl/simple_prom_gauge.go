// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetryimpl

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type simplePromGauge struct {
	g prometheus.Gauge
}

// Inc increments the gauge.
func (s *simplePromGauge) Inc() {
	s.g.Inc()
}

// Dec decrements the gauge.
func (s *simplePromGauge) Dec() {
	s.g.Dec()
}

// Add increments the gauge by given amount.
func (s *simplePromGauge) Add(val float64) {
	s.g.Add(val)
}

// Sub decrements the gauge by given amount.
func (s *simplePromGauge) Sub(val float64) {
	s.g.Sub(val)
}

// Set sets the value of the gauge.
func (s *simplePromGauge) Set(val float64) {
	s.g.Set(val)
}

// Get gets the value of the gauge.
func (s *simplePromGauge) Get() float64 {
	metric := &dto.Metric{}
	_ = s.g.Write(metric)
	return *metric.Gauge.Value
}

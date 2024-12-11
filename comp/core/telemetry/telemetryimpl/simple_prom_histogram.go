// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetryimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// Prometheus implements histograms using Prometheus.
type simplePromHistogram struct {
	h prometheus.Histogram
}

// Observe the value to the Histogram value.
func (s *simplePromHistogram) Observe(value float64) {
	s.h.Observe(value)
}

func (s *simplePromHistogram) Get() telemetry.HistogramValue {
	m := &dto.Metric{}
	_ = s.h.Write(m)
	hv := telemetry.HistogramValue{
		Count:   *m.Histogram.SampleCount,
		Sum:     *m.Histogram.SampleSum,
		Buckets: make([]telemetry.Bucket, 0, len(m.Histogram.Bucket)),
	}

	for _, b := range m.Histogram.Bucket {
		hv.Buckets = append(hv.Buckets, telemetry.Bucket{
			UpperBound: *b.UpperBound,
			Count:      *b.CumulativeCount,
		})

	}
	return hv
}

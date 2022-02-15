// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Histogram tracks the value of one health metric of the Agent.
type Histogram interface {
	// Observe the value to the Histogram value.
	Observe(value float64, tagsValue ...string)
	// Delete deletes the value for the Histogram with the given tags.
	Delete(tagsValue ...string)
}

type histogramNoOp struct{}

func (h histogramNoOp) Observe(_ float64, _ ...string) {}
func (h histogramNoOp) Delete(_ ...string)             {}

// NewHistogramNoOp creates a dummy Histogram
func NewHistogramNoOp() Histogram {
	return histogramNoOp{}
}

// NewHistogram creates a Histogram with default options for telemetry purpose.
// Current implementation used: Prometheus Histogram
func NewHistogram(subsystem, name string, tags []string, help string, buckets []float64) Histogram {
	return NewHistogramWithOpts(subsystem, name, tags, help, buckets, DefaultOptions)
}

// NewHistogramWithOpts creates a Histogram with the given options for telemetry purpose.
// See NewHistogram()
func NewHistogramWithOpts(subsystem, name string, tags []string, help string, buckets []float64, opts Options) Histogram {
	name = opts.NameWithSeparator(subsystem, name)

	h := &promHistogram{
		ph: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem: subsystem,
				Name:      name,
				Help:      help,
				Buckets:   buckets,
			},
			tags,
		),
	}
	telemetryRegistry.MustRegister(h.ph)
	return h
}

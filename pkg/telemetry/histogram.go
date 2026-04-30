// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
)

// Histogram tracks the value of one health metric of the Agent.
type Histogram interface {
	telemetryComponent.Histogram
}

type histogramNoOp struct{}

func (h histogramNoOp) Observe(_ float64, _ ...string)                            {}
func (h histogramNoOp) Delete(_ ...string)                                        {}
func (h histogramNoOp) WithValues(_ ...string) telemetryComponent.SimpleHistogram { return nil }
func (h histogramNoOp) WithTags(_ map[string]string) telemetryComponent.SimpleHistogram {
	return nil
}

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
	return GetCompatComponent().NewHistogramWithOpts(subsystem, name, tags, help, buckets, telemetryComponent.Options(opts))
}

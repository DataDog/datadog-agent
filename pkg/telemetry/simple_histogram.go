// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
)

// SimpleHistogram tracks how many times something is happening.
type SimpleHistogram interface {
	telemetryComponent.SimpleHistogram
}

// NewSimpleHistogram creates a new SimpleHistogram with default options.
func NewSimpleHistogram(subsystem, name, help string, buckets []float64) SimpleHistogram {
	return NewSimpleHistogramWithOpts(subsystem, name, help, buckets, DefaultOptions)
}

// NewSimpleHistogramWithOpts creates a new SimpleHistogram.
func NewSimpleHistogramWithOpts(subsystem, name, help string, buckets []float64, opts Options) SimpleHistogram {
	return GetCompatComponent().NewSimpleHistogramWithOpts(subsystem, name, help, buckets, telemetryComponent.Options(opts))
}

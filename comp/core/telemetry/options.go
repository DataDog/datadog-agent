// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import "fmt"

// Options for telemetry metrics.
// Creating an Options struct without specifying any of its fields should be the
// equivalent of using the DefaultOptions var.
type Options struct {
	// NoDoubleUnderscoreSep is set to true when you don't want to
	// separate the subsystem and the name with a double underscore separator.
	NoDoubleUnderscoreSep bool

	// All observations with an absolute value of less or equal
	// NativeHistogramZeroThreshold are accumulated into a “zero”
	// bucket. For best results, this should be close to a bucket
	// boundary. This is usually the case if picking a power of two. If
	// NativeHistogramZeroThreshold is left at zero,
	// prometheus.DefNativeHistogramZeroThreshold is used as the threshold.
	// To configure a zero bucket with an actual threshold of zero (i.e. only
	// observations of precisely zero will go into the zero bucket), set
	// NativeHistogramZeroThreshold to the NativeHistogramZeroThresholdZero
	// constant (or any negative float value).
	NativeHistogramZeroThreshold float64
}

// DefaultOptions for telemetry metrics which don't need to specify any option.
var DefaultOptions = Options{
	// By default, we want to separate the subsystem and the metric name with a
	// double underscore to be able to replace it later in the process.
	NoDoubleUnderscoreSep: false,

	// By default, NativeHistogramZeroThreshold is left at zero, thus
	// prometheus.DefNativeHistogramZeroThreshold is used as the threshold.
	NativeHistogramZeroThreshold: 0,
}

// NameWithSeparator returns name prefixed according to NoDoubleUnderscoreOption.
func (opts *Options) NameWithSeparator(subsystem, name string) string {
	// subsystem is optional
	if subsystem != "" && !opts.NoDoubleUnderscoreSep {
		// Prefix metrics with a _, prometheus will add a second _
		// It will create metrics with a custom separator and
		// will let us replace it to a dot later in the process.
		return fmt.Sprintf("_%s", name)
	}

	return name
}

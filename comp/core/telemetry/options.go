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
	//
	// This option is not compatible with the cross-org agent telemetry
	NoDoubleUnderscoreSep bool

	// DefaultMetric exports metric by default via built-in agent_telemetry core check.
	DefaultMetric bool
}

// DefaultOptions for telemetry metrics which don't need to specify any option.
var DefaultOptions = Options{
	// By default, we want to separate the subsystem and the metric name with a
	// double underscore to be able to replace it later in the process.
	NoDoubleUnderscoreSep: false,
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

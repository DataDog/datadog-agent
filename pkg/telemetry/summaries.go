// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Summary tracks the value of one health metric of the Agent.
type Summary interface {
	// Observe  the value to the Summary value.
	Observe(value float64, tagsValue ...string)
	// Delete deletes the value for the Summary with the given tags.
	Delete(tagsValue ...string)
}

type summaryNoOp struct{}

func (h summaryNoOp) Observe(_ float64, _ ...string) {}
func (h summaryNoOp) Delete(_ ...string)             {}

// NewSummaryNoOp creates a dummy Summary
func NewSummaryNoOp() Summary {
	return summaryNoOp{}
}

// NewSummary creates a Summary with default options for telemetry purpose.
// Current implementation used: Prometheus Summary
func NewSummary(subsystem, name string, tags []string, help string) Summary {
	return NewSummaryWithOpts(subsystem, name, tags, help, DefaultOptions)
}

// NewSummaryWithOpts creates a Summary with the given options for telemetry purpose.
// See NewSummary()
func NewSummaryWithOpts(subsystem, name string, tags []string, help string, opts Options) Summary {
	// subsystem is optional
	if subsystem != "" && !opts.NoDoubleUnderscoreSep {
		// Prefix metrics with a _, prometheus will add a second _
		// It will create metrics with a custom separator and
		// will let us replace it to a dot later in the process.
		name = fmt.Sprintf("_%s", name)
	}

	s := &promSummary{
		ps: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Subsystem: subsystem,
				Name:      name,
				Help:      help,
			},
			tags,
		),
	}
	telemetryRegistry.MustRegister(s.ps)
	return s
}

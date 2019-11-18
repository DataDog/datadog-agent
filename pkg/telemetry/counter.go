// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package telemetry

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
)

// Counter tracks how many times something is happening.
type Counter interface {
	// Inc increments the counter for the given tags.
	Inc(tags ...string)
	// Add adds the given value to the counter for the given tags.
	Add(value float64, tags ...string)
	// Delete deletes the value for the counter with the given tags.
	Delete(tags ...string)
}

// NewCounter creates a Counter for telemetry purpose.
// If the telemetry's not enabled, returns a noop Counter.
func NewCounter(subsystem, name string, tags []string, help string) Counter {
	if !config.Datadog.GetBool("telemetry.enabled") {
		return &noopCounter{}
	}
	c := &promCounter{
		pc: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      name,
				Help:      help,
			},
			tags,
		),
	}
	prometheus.MustRegister(c.pc)
	return c
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package telemetry

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// NewCounter creates a Counter for telemetry purpose.
func NewCounter(subsystem, name string, tags []string, help string) Counter {
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

// Counter implementation using Prometheus.
type promCounter struct {
	pc   *prometheus.CounterVec
	once sync.Once
}

// Add adds the given value to the counter for the given tags.
func (c *promCounter) Add(value float64, tags ...string) {
	c.pc.WithLabelValues(tags...).Add(value)
}

// Inc increments the counter for the given tags.
func (c *promCounter) Inc(tags ...string) {
	c.pc.WithLabelValues(tags...).Inc()
}

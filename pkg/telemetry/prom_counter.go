// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Counter implementation using Prometheus.
type promCounter struct {
	pc *prometheus.CounterVec
}

// Add adds the given value to the counter for the given tags.
func (c *promCounter) Add(value float64, tags ...string) {
	c.pc.WithLabelValues(tags...).Add(value)
}

// Inc increments the counter for the given tags.
func (c *promCounter) Inc(tags ...string) {
	c.pc.WithLabelValues(tags...).Inc()
}

// Delete deletes the value for the counter with the given tags.
func (c *promCounter) Delete(tags ...string) {
	c.pc.DeleteLabelValues(tags...)
}

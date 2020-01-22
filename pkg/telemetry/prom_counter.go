// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Counter implementation using Prometheus.
type promCounter struct {
	pc *prometheus.CounterVec
}

// Add adds the given value to the counter for the given tags.
func (c *promCounter) Add(value float64, tagsValue ...string) {
	c.pc.WithLabelValues(tagsValue...).Add(value)
}

// Inc increments the counter for the given tags.
func (c *promCounter) Inc(tagsValue ...string) {
	c.pc.WithLabelValues(tagsValue...).Inc()
}

// Delete deletes the value for the counter with the given tags.
func (c *promCounter) Delete(tagsValue ...string) {
	c.pc.DeleteLabelValues(tagsValue...)
}

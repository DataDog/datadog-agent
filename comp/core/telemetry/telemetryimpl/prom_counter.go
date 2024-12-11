// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetryimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/prometheus/client_golang/prometheus"
)

// Counter implementation using Prometheus.
type promCounter struct {
	pc *prometheus.CounterVec
}

// InitializeToZero creates the counter with the given tags and initializes it to 0.
// This method is intended to be used when the counter value is important to
// send even before any incrementing/addition is done on it.
func (c *promCounter) InitializeToZero(tagsValue ...string) {
	// By requesting a counter for a set of tags, we are creating and initializing
	// the counter at 0. See the following for more info:
	// https://github.com/prometheus/client_golang/blob/v1.9.0/prometheus/counter.go#L194-L196
	c.pc.WithLabelValues(tagsValue...)
}

// Add adds the given value to the counter with the given tags value.
//
// If the value is < 0, no add takes place, as the counter is monotonic.
// The prometheus client would panic in such a case.
func (c *promCounter) Add(value float64, tagsValue ...string) {
	if value > 0 {
		c.pc.WithLabelValues(tagsValue...).Add(value)
	}
}

// AddWithTags adds the given value to the counter with the given tags.
// Even if less convenient, this signature could be used in hot path
// instead of Add(float64, ...string) to avoid escaping the parameters on the heap.
//
// If the value is < 0, no add takes place, as the counter is monotonic.
// The prometheus client would panic in such a case.
func (c *promCounter) AddWithTags(value float64, tags map[string]string) {
	if value > 0 {
		c.pc.With(tags).Add(value)
	}
}

// Inc increments the counter with the given tags value.
func (c *promCounter) Inc(tagsValue ...string) {
	c.pc.WithLabelValues(tagsValue...).Inc()
}

// IncWithTags increments the counter with the given tags.
// Even if less convenient, this signature could be used in hot path
// instead of Inc(...string) to avoid escaping the parameters on the heap.
func (c *promCounter) IncWithTags(tags map[string]string) {
	c.pc.With(tags).Inc()
}

// Delete deletes the value for the counter with the given tags value.
func (c *promCounter) Delete(tagsValue ...string) {
	c.pc.DeleteLabelValues(tagsValue...)
}

// DeleteWithTags deletes the value for the counter with the given tags.
// Even if less convenient, this signature could be used in hot path
// instead of Delete(...string) to avoid escaping the parameters on the heap.
func (c *promCounter) DeleteWithTags(tags map[string]string) {
	c.pc.Delete(tags)
}

// WithValues returns SimpleCounter for this metric with the given tag values.
func (c *promCounter) WithValues(tagsValue ...string) telemetry.SimpleCounter {
	return &simplePromCounter{c: c.pc.WithLabelValues(tagsValue...)}
}

// Withtags returns SimpleCounter for this metric with the given tag values.
func (c *promCounter) WithTags(tags map[string]string) telemetry.SimpleCounter {
	return &simplePromCounter{c: c.pc.With(tags)}
}

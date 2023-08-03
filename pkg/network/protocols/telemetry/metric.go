// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"strings"

	"go.uber.org/atomic"
	"k8s.io/apimachinery/pkg/util/sets"
)

// Counter is a cumulative metric that grows monotonically
type Counter struct {
	*metricBase
}

// NewCounter returns a new metric of type `Counter`
func NewCounter(name string, tagsAndOptions ...string) *Counter {
	c := &Counter{
		newMetricBase(name, tagsAndOptions),
	}

	return globalRegistry.FindOrCreate(c).(*Counter)
}

// Add value atomically
func (c *Counter) Add(v int64) {
	if v < 0 {
		// Counters are always monotonic so we don't allow negative numbers. We
		// could enforce this by using an unsigned type, but that would make the
		// API a little bit more cumbersome to use.
		return
	}

	c.value.Add(v)
}

// Gauge is a metric that represents a numerical value that can arbitrarily go up and down
type Gauge struct {
	*metricBase
}

// NewGauge returns a new metric of type `Gauge`
func NewGauge(name string, tagsAndOptions ...string) *Gauge {
	c := &Gauge{
		newMetricBase(name, tagsAndOptions),
	}

	return globalRegistry.FindOrCreate(c).(*Gauge)
}

// Set value atomically
func (g *Gauge) Set(v int64) {
	g.value.Store(v)
}

// Add value atomically
func (g *Gauge) Add(v int64) {
	g.value.Add(v)
}

type metricBase struct {
	name  string
	tags  sets.String
	opts  sets.String
	value *atomic.Int64
}

func newMetricBase(name string, tagsAndOptions []string) *metricBase {
	tags, opts := splitTagsAndOptions(tagsAndOptions)

	return &metricBase{
		name:  name,
		value: atomic.NewInt64(0),
		tags:  tags,
		opts:  opts,
	}
}

// Name of the `Metric` (including tags)
func (m *metricBase) Name() string {
	return strings.Join(append([]string{m.name}, m.tags.List()...), ",")
}

// Get value atomically
func (m *metricBase) Get() int64 {
	return m.value.Load()
}

// this method is used to essentially convert the `metric`
// interface to the underlying `metricBase` in the code
// that has to deal with both `Counter` and `Gauge` types
func (m *metricBase) base() *metricBase {
	return m
}

// metric is the private interface shared by `Counter` and `Gauge`
// the base() method simply returns the embedded `*metricBase` struct
// which is all we need in the internal code
type metric interface {
	base() *metricBase
}

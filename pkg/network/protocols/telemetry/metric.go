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
	if v > 0 {
		// Counters are always monotonic so we don't allow non-positive numbers. We
		// could enforce this by using an unsigned type, but that would make the
		// API a little bit more cumbersome to use.
		c.value.Add(v)
	}
}

func (c *Counter) base() *metricBase {
	return c.metricBase
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

func (g *Gauge) base() *metricBase {
	return g.metricBase
}

type metricBase struct {
	name  string
	tags  sets.Set[string]
	opts  sets.Set[string]
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
	return strings.Join(append([]string{m.name}, sets.List(m.tags)...), ",")
}

// Get value atomically
func (m *metricBase) Get() int64 {
	return m.value.Load()
}

// metric is the private interface shared by `Counter` and `Gauge`
// the base() method simply returns the embedded `*metricBase` struct
// which is all we need in the internal code that has to deal with both types
type metric interface {
	base() *metricBase
}

// TLSAwareCounter is a TLS aware counter, it has a plain counter and a counter for TLS.
// It enables the use of a single metric that increments based on the encryption, avoiding the need for separate metrics for eash use-case.
type TLSAwareCounter struct {
	counterPlain *Counter
	counterTLS   *Counter
}

// NewTLSAwareCounter creates and returns a new instance of TLSCounter
func NewTLSAwareCounter(metricGroup *MetricGroup, metricName string, tags ...string) *TLSAwareCounter {
	return &TLSAwareCounter{
		counterPlain: metricGroup.NewCounter(metricName, append(tags, "encrypted:false")...),
		counterTLS:   metricGroup.NewCounter(metricName, append(tags, "encrypted:true")...),
	}
}

// Add adds the given delta to the counter based on the encryption.
func (c *TLSAwareCounter) Add(delta int64, isTLS bool) {
	if isTLS {
		c.counterTLS.Add(delta)
		return
	}
	c.counterPlain.Add(delta)
}

// Get returns the counter value based on the encryption.
func (c *TLSAwareCounter) Get(isTLS bool) int64 {
	if isTLS {
		return c.counterTLS.Get()
	}
	return c.counterPlain.Get()
}

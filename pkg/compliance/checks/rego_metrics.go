// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"sync"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/open-policy-agent/opa/metrics"
	"github.com/spf13/cast"
)

const regoMetricPrefix = "datadog.security_agent.compliance.opa."

type regoMetric struct {
	name   string
	client statsd.ClientInterface
}

type regoMetricsCounter struct {
	regoMetric
	regoCounter metrics.Counter
}

func (c *regoMetricsCounter) Incr() {
	c.regoCounter.Incr()
	_ = c.client.Incr(c.name, nil, 1)
}

func (c *regoMetricsCounter) Value() interface{} {
	return c.regoCounter.Value()
}

func (c *regoMetricsCounter) Add(n uint64) {
	c.regoCounter.Add(n)
	_ = c.client.Count(c.name, cast.ToInt64(c.Value())+int64(n), nil, 1)
}

type regoMetricsHistogram struct {
	regoMetric
	regoHistogram metrics.Histogram
}

func (h *regoMetricsHistogram) Value() interface{} {
	return h.regoHistogram.Value()
}

func (h *regoMetricsHistogram) Update(n int64) {
	h.regoHistogram.Update(n)
	_ = h.client.Histogram(h.name, float64(n), nil, 1)
}

type regoMetricsTimer struct {
	regoMetric
	regoTimer metrics.Timer
}

func (t *regoMetricsTimer) Value() interface{} {
	return t.regoTimer.Value()
}

func (t *regoMetricsTimer) Int64() int64 {
	return cast.ToInt64(t.Value())
}

func (t *regoMetricsTimer) Start() {
	t.regoTimer.Start()
}

func (t *regoMetricsTimer) Stop() int64 {
	delta := t.regoTimer.Stop()
	_ = t.client.Histogram(t.name, float64(delta), nil, 1)
	return delta
}

type regoMetrics struct {
	sync.Mutex
	client     statsd.ClientInterface
	inner      metrics.Metrics
	counters   map[string]*regoMetricsCounter
	timers     map[string]*regoMetricsTimer
	histograms map[string]*regoMetricsHistogram
}

func (m *regoMetrics) Info() metrics.Info {
	return m.inner.Info()
}

func (m *regoMetrics) Timer(name string) metrics.Timer {
	m.Lock()
	defer m.Unlock()
	t, ok := m.timers[name]
	if !ok {
		t = &regoMetricsTimer{
			regoMetric: regoMetric{
				name:   regoMetricPrefix + name,
				client: m.client,
			},
			regoTimer: m.inner.Timer(name),
		}
		m.timers[name] = t
	}
	return t
}

func (m *regoMetrics) Histogram(name string) metrics.Histogram {
	m.Lock()
	defer m.Unlock()
	h, ok := m.histograms[name]
	if !ok {
		h = &regoMetricsHistogram{
			regoMetric: regoMetric{
				name:   regoMetricPrefix + name,
				client: m.client,
			},
			regoHistogram: m.inner.Histogram(name),
		}
		m.histograms[name] = h
	}
	return h
}

func (m *regoMetrics) Counter(name string) metrics.Counter {
	m.Lock()
	defer m.Unlock()
	c, ok := m.counters[name]
	if !ok {
		c = &regoMetricsCounter{
			regoMetric: regoMetric{
				name:   regoMetricPrefix + name,
				client: m.client,
			},
			regoCounter: m.inner.Counter(name),
		}
		m.counters[name] = c
	}
	return c
}

func (m *regoMetrics) All() map[string]interface{} {
	return m.inner.All()
}

func (m *regoMetrics) Clear() {
	m.inner.Clear()
}

func (m *regoMetrics) MarshalJSON() ([]byte, error) {
	return m.inner.MarshalJSON()
}

func newRegoMetrics(inner metrics.Metrics, client statsd.ClientInterface) *regoMetrics {
	if inner == nil {
		inner = metrics.New()
	}
	return &regoMetrics{
		client:     client,
		inner:      inner,
		counters:   make(map[string]*regoMetricsCounter),
		timers:     make(map[string]*regoMetricsTimer),
		histograms: make(map[string]*regoMetricsHistogram),
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/open-policy-agent/opa/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cast"
)

var registeredHistograms map[string]telemetry.Histogram

func registerHistogram(name string) telemetry.Histogram {
	if registeredHistograms == nil {
		registeredHistograms = make(map[string]telemetry.Histogram)
	}

	if histogram, found := registeredHistograms[name]; found {
		return histogram
	}

	buckets := make([]float64, len(prometheus.DefBuckets))
	for i, v := range prometheus.DefBuckets {
		buckets[i] = v * float64(time.Millisecond)
	}

	histogram := telemetry.NewHistogram("opa", name, nil, "", buckets)
	registeredHistograms[name] = histogram
	return histogram
}

var registeredCounters map[string]telemetry.Counter

func registerCounter(name string) telemetry.Counter {
	if registeredCounters == nil {
		registeredCounters = make(map[string]telemetry.Counter)
	}

	if counter, found := registeredCounters[name]; found {
		return counter
	}

	counter := telemetry.NewCounter("opa", name, nil, "")
	registeredCounters[name] = counter
	return counter
}

type regoCounter struct {
	regoCounter metrics.Counter
	ddCounter   telemetry.Counter
}

func (c *regoCounter) Incr() {
	c.regoCounter.Incr()
	c.ddCounter.Inc()
}

func (c *regoCounter) Value() interface{} {
	return c.regoCounter.Value()
}

func (c *regoCounter) Add(n uint64) {
	c.regoCounter.Add(n)
	c.ddCounter.Add(float64(n))
}

type regoHistogram struct {
	regoHistogram metrics.Histogram
	ddHistogram   telemetry.Histogram
}

func (h *regoHistogram) Value() interface{} {
	return h.regoHistogram.Value()
}

func (h *regoHistogram) Update(n int64) {
	h.regoHistogram.Update(n)
	h.ddHistogram.Observe(float64(n))
}

type regoTimer struct {
	regoTimer   metrics.Timer
	ddHistogram telemetry.Histogram
}

func (t *regoTimer) Value() interface{} {
	return t.regoTimer.Value()
}

func (t *regoTimer) Int64() int64 {
	return cast.ToInt64(t.Value())
}

func (t *regoTimer) Start() {
	t.regoTimer.Start()
}

func (t *regoTimer) Stop() int64 {
	delta := t.regoTimer.Stop()
	t.ddHistogram.Observe(float64(delta))
	return delta
}

type regoTelemetry struct {
	sync.Mutex
	inner      metrics.Metrics
	counters   map[string]*regoCounter
	timers     map[string]*regoTimer
	histograms map[string]*regoHistogram
}

func (m *regoTelemetry) Info() metrics.Info {
	return m.inner.Info()
}

func (m *regoTelemetry) Timer(name string) metrics.Timer {
	m.Lock()
	defer m.Unlock()
	t, ok := m.timers[name]
	if !ok {
		ddHistogram := registerHistogram(name)
		t = &regoTimer{
			regoTimer:   m.inner.Timer(name),
			ddHistogram: ddHistogram,
		}
		m.timers[name] = t
	}
	return t
}

func (m *regoTelemetry) Histogram(name string) metrics.Histogram {
	m.Lock()
	defer m.Unlock()
	h, ok := m.histograms[name]
	if !ok {
		ddHistogram := registerHistogram(name)
		h = &regoHistogram{
			regoHistogram: m.inner.Histogram(name),
			ddHistogram:   ddHistogram,
		}
		m.histograms[name] = h
	}
	return h
}

func (m *regoTelemetry) Counter(name string) metrics.Counter {
	m.Lock()
	defer m.Unlock()
	c, ok := m.counters[name]
	if !ok {
		ddCounter := registerCounter(name)
		c = &regoCounter{
			regoCounter: m.inner.Counter(name),
			ddCounter:   ddCounter,
		}
		m.counters[name] = c
	}
	return c
}

func (m *regoTelemetry) All() map[string]interface{} {
	return m.inner.All()
}

func (m *regoTelemetry) Clear() {
	m.inner.Clear()
}

func (m *regoTelemetry) MarshalJSON() ([]byte, error) {
	return m.inner.MarshalJSON()
}

func newRegoTelemetry() *regoTelemetry {
	return &regoTelemetry{
		inner:      metrics.New(),
		counters:   make(map[string]*regoCounter),
		timers:     make(map[string]*regoTimer),
		histograms: make(map[string]*regoHistogram),
	}
}

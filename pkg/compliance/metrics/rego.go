// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	opametrics "github.com/open-policy-agent/opa/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cast"
)

var (
	registrationMu       sync.Mutex
	registeredHistograms map[string]telemetry.Histogram
	registeredCounters   map[string]telemetry.Counter
)

// NewRegoTelemetry returns a opa/metrics.Metrics interface to monitor rego's
// performance.
func NewRegoTelemetry() opametrics.Metrics {
	return &regoTelemetry{
		inner:      opametrics.New(),
		counters:   make(map[string]*regoCounter),
		timers:     make(map[string]*regoTimer),
		histograms: make(map[string]*regoHistogram),
	}
}

func registerHistogram(name string) telemetry.Histogram {
	registrationMu.Lock()
	defer registrationMu.Unlock()

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

func registerCounter(name string) telemetry.Counter {
	registrationMu.Lock()
	defer registrationMu.Unlock()

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
	regoCounter opametrics.Counter
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
	regoHistogram opametrics.Histogram
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
	regoTimer   opametrics.Timer
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
	inner      opametrics.Metrics
	counters   map[string]*regoCounter
	timers     map[string]*regoTimer
	histograms map[string]*regoHistogram
}

func (m *regoTelemetry) Info() opametrics.Info {
	return m.inner.Info()
}

func (m *regoTelemetry) Timer(name string) opametrics.Timer {
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

func (m *regoTelemetry) Histogram(name string) opametrics.Histogram {
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

func (m *regoTelemetry) Counter(name string) opametrics.Counter {
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

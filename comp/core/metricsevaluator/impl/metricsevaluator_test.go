// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metricsevaluatorimpl

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricsevaluator "github.com/DataDog/datadog-agent/comp/core/metricsevaluator/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

func validMonitor() metricsevaluator.Monitor {
	return metricsevaluator.Monitor{
		Name:        "high_memory",
		Metric:      "process_resident_memory_bytes",
		Window:      5 * time.Minute,
		Aggregation: metricsevaluator.AggregationAvg,
		Comparator:  metricsevaluator.ComparatorAbove,
		Threshold:   1 << 30,
		Callback:    func(metricsevaluator.Event) {},
	}
}

func TestRegister(t *testing.T) {
	e := newEvaluator(logmock.New(t), telemetryimpl.NewMock(t), time.Minute)

	require.NoError(t, e.Register(validMonitor()))
	assert.ErrorContains(t, e.Register(validMonitor()), "already registered")

	e.Unregister("high_memory")
	assert.NoError(t, e.Register(validMonitor()))
}

func TestRegisterDefaultsToKeepStateOnNoData(t *testing.T) {
	e := newEvaluator(logmock.New(t), telemetryimpl.NewMock(t), time.Minute)

	require.NoError(t, e.Register(validMonitor()))
	assert.Equal(t, metricsevaluator.NoDataKeepState, e.monitors["high_memory"].monitor.OnNoData)
}

func TestRegisterRejectsInvalidMonitors(t *testing.T) {
	e := newEvaluator(logmock.New(t), telemetryimpl.NewMock(t), time.Minute)

	for name, mutate := range map[string]func(*metricsevaluator.Monitor){
		"empty name":          func(m *metricsevaluator.Monitor) { m.Name = "" },
		"empty metric":        func(m *metricsevaluator.Monitor) { m.Metric = "" },
		"window too short":    func(m *metricsevaluator.Monitor) { m.Window = 30 * time.Second },
		"nil callback":        func(m *metricsevaluator.Monitor) { m.Callback = nil },
		"unknown aggregation": func(m *metricsevaluator.Monitor) { m.Aggregation = "p99" },
		"unknown comparator":  func(m *metricsevaluator.Monitor) { m.Comparator = "equal" },
		"unknown no data":     func(m *metricsevaluator.Monitor) { m.OnNoData = "resolve" },
	} {
		t.Run(name, func(t *testing.T) {
			m := validMonitor()
			mutate(&m)
			assert.Error(t, e.Register(m))
		})
	}
}

func TestNextState(t *testing.T) {
	above := metricsevaluator.Monitor{Comparator: metricsevaluator.ComparatorAbove, Threshold: 10, OnNoData: metricsevaluator.NoDataNotify}
	below := metricsevaluator.Monitor{Comparator: metricsevaluator.ComparatorBelow, Threshold: 10, OnNoData: metricsevaluator.NoDataNotify}
	sticky := above
	sticky.OnNoData = metricsevaluator.NoDataKeepState

	for _, tc := range []struct {
		name     string
		monitor  metricsevaluator.Monitor
		previous metricsevaluator.State
		value    float64
		hasData  bool
		want     metricsevaluator.State
	}{
		{"above breached", above, metricsevaluator.StateOK, 11, true, metricsevaluator.StateAlert},
		{"above at threshold", above, metricsevaluator.StateOK, 10, true, metricsevaluator.StateOK},
		{"above recovers", above, metricsevaluator.StateAlert, 9, true, metricsevaluator.StateOK},
		{"below breached", below, metricsevaluator.StateOK, 9, true, metricsevaluator.StateAlert},
		{"below at threshold", below, metricsevaluator.StateOK, 10, true, metricsevaluator.StateOK},
		{"below recovers", below, metricsevaluator.StateAlert, 11, true, metricsevaluator.StateOK},
		{"no data while alerting", above, metricsevaluator.StateAlert, 0, false, metricsevaluator.StateNoData},
		{"keep state: alert stays alert", sticky, metricsevaluator.StateAlert, 0, false, metricsevaluator.StateAlert},
		{"keep state: ok stays ok", sticky, metricsevaluator.StateOK, 0, false, metricsevaluator.StateOK},
		{"keep state: still no data before the first evaluation", sticky, metricsevaluator.StateNoData, 0, false, metricsevaluator.StateNoData},
		{"keep state: evaluates when data is back", sticky, metricsevaluator.StateAlert, 5, true, metricsevaluator.StateOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, nextState(tc.monitor, tc.previous, tc.value, tc.hasData))
		})
	}
}

func TestEvaluateFiresOnTransitionsOnly(t *testing.T) {
	tel := telemetryimpl.NewMock(t)
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{Name: "process_resident_memory_bytes"})
	tel.GetRegistry().MustRegister(gauge)

	var events []metricsevaluator.Event
	e := newEvaluator(logmock.New(t), tel, time.Minute)
	m := validMonitor()
	m.Window = time.Minute // 2 samples
	m.Aggregation = metricsevaluator.AggregationMax
	m.Threshold = 100
	m.Callback = func(ev metricsevaluator.Event) { events = append(events, ev) }
	require.NoError(t, e.Register(m))

	now := time.Now()
	for _, v := range []float64{50, 50, 50, 150, 150, 50, 50} {
		gauge.Set(v)
		e.evaluate(now)
	}

	require.Len(t, events, 3)
	assert.Equal(t, metricsevaluator.StateNoData, events[0].Previous)
	assert.Equal(t, metricsevaluator.StateOK, events[0].State)
	assert.Equal(t, metricsevaluator.StateAlert, events[1].State)
	assert.Equal(t, float64(150), events[1].Value)
	assert.Equal(t, metricsevaluator.StateOK, events[2].State)
}

// TestAgentMemoryMonitor runs the monitor from Example_agentMemoryUsage against the agent_performance gauges.
func TestAgentMemoryMonitor(t *testing.T) {
	tel := telemetryimpl.NewMock(t)
	usage := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "agent_performance__memory_usage"}, []string{"kind", "pod_name"})
	limit := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "agent_performance__memory_limit"}, []string{"kind", "pod_name"})
	tel.GetRegistry().MustRegister(usage, limit)
	// The cluster Agent series must not leak into the node Agent ratio.
	usage.WithLabelValues("cluster-agent", "dca").Set(1000)
	limit.WithLabelValues("cluster-agent", "dca").Set(1000)
	limit.WithLabelValues("agent", "node-agent").Set(1000)

	var events []metricsevaluator.Event
	e := newEvaluator(logmock.New(t), tel, time.Minute)
	require.NoError(t, e.Register(metricsevaluator.Monitor{
		Name:        "agent_memory_usage",
		Metric:      "agent_performance__memory_usage",
		DivideBy:    "agent_performance__memory_limit",
		Tags:        map[string]string{"kind": "agent"},
		Window:      5 * time.Minute,
		Aggregation: metricsevaluator.AggregationAvg,
		Comparator:  metricsevaluator.ComparatorAbove,
		Threshold:   0.9,
		Callback:    func(ev metricsevaluator.Event) { events = append(events, ev) },
	}))

	evaluate := func(used float64) {
		usage.WithLabelValues("agent", "node-agent").Set(used)
		e.evaluate(time.Now())
	}

	// The 5 minute window needs 6 samples before the first evaluation.
	for range 6 {
		evaluate(500)
	}
	require.Len(t, events, 1)
	assert.Equal(t, metricsevaluator.StateOK, events[0].State)
	assert.InDelta(t, 0.5, events[0].Value, 1e-9)

	// A short spike is smoothed by the average and does not alert.
	evaluate(1000)
	assert.Len(t, events, 1)

	// Sustained high usage alerts once.
	for range 6 {
		evaluate(950)
	}
	require.Len(t, events, 2)
	assert.Equal(t, metricsevaluator.StateAlert, events[1].State)

	// The container check stops reporting: the alert is kept, nothing fires.
	usage.Reset()
	for range 6 {
		e.evaluate(time.Now())
	}
	assert.Len(t, events, 2)

	// Usage goes back down: the monitor recovers.
	for range 6 {
		evaluate(400)
	}
	require.Len(t, events, 3)
	assert.Equal(t, metricsevaluator.StateOK, events[2].State)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package module

import (
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
)

type reportedMetric struct {
	kind  string
	name  string
	value float64
	tags  []string
}

type fakeMetricsClient struct {
	mu      sync.Mutex
	metrics []reportedMetric
}

func (c *fakeMetricsClient) Count(name string, value int64, tags []string, _ float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics = append(c.metrics, reportedMetric{"count", name, float64(value), tags})
	return nil
}

func (c *fakeMetricsClient) Gauge(name string, value float64, tags []string, _ float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics = append(c.metrics, reportedMetric{"gauge", name, value, tags})
	return nil
}

// take returns the metrics reported since the previous call.
func (c *fakeMetricsClient) take() []reportedMetric {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := c.metrics
	c.metrics = nil
	return m
}

// metricProbe returns an IR probe for a metric probe whose value, if any, has
// the given Go kind.
func metricProbe(id, kind, name string, tags []string, valueKind reflect.Kind) *ir.Probe {
	rootType := &ir.EventRootType{}
	if valueKind != reflect.Invalid {
		rootType.Expressions = []*ir.RootExpression{{
			Kind: ir.RootExpressionKindMetricValue,
			Expression: ir.Expression{Type: &ir.BaseType{
				GoTypeAttributes: ir.GoTypeAttributes{GoKind: valueKind},
			}},
		}}
	}
	return &ir.Probe{
		ProbeDefinition: &rcjson.MetricProbe{
			ProbeCommon: rcjson.ProbeCommon{ID: id, Tags: tags},
			Kind:        kind,
			MetricName:  name,
		},
		Instances: []ir.ProbeInstance{{
			Events: []*ir.Event{{Kind: ir.EventKindEntry, Type: rootType}},
		}},
	}
}

func TestMetricsReporter(t *testing.T) {
	client := &fakeMetricsClient{}
	var emittingMu sync.Mutex
	var emitting []string
	r := newMetricsReporter(client, time.Hour, func(_ ir.ProgramID, p ir.ProbeDefinition) {
		emittingMu.Lock()
		defer emittingMu.Unlock()
		emitting = append(emitting, p.GetID())
	})
	defer r.close()

	program := &ir.Program{Probes: []*ir.Probe{
		metricProbe("calls", "COUNT", "orders.calls", []string{"team:checkout"}, reflect.Invalid),
		// Not a metric probe; ignored.
		{ProbeDefinition: &rcjson.LogProbe{
			LogProbeCommon: rcjson.LogProbeCommon{ProbeCommon: rcjson.ProbeCommon{ID: "log"}},
		}},
		metricProbe("latency", "GAUGE", "orders latency|ms", nil, reflect.Float64),
	}}
	var statesMu sync.Mutex
	states := make([]loader.MetricState, len(program.Probes))
	setState := func(probeID int, s loader.MetricState) {
		statesMu.Lock()
		defer statesMu.Unlock()
		states[probeID] = s
	}
	readStates := func() []loader.MetricState {
		statesMu.Lock()
		defer statesMu.Unlock()
		return append([]loader.MetricState(nil), states...)
	}
	r.register(1, program, readStates, procRuntimeID{
		service: "checkout", environment: "prod", version: "1.2.3",
	})

	// Nothing has been hit yet.
	r.reportAll()
	require.Empty(t, client.take())
	require.Empty(t, emitting)

	countTags := []string{
		"team:checkout", "debugger.probeid:calls", "service:checkout", "env:prod", "version:1.2.3",
	}
	gaugeTags := []string{
		"debugger.probeid:latency", "service:checkout", "env:prod", "version:1.2.3",
	}
	setState(0, loader.MetricState{Sum: 3, Updates: 3})
	setState(2, loader.MetricState{Value: math.Float64bits(1.5), Updates: 1})
	r.reportAll()
	require.ElementsMatch(t, []reportedMetric{
		{"count", "dynamic.instrumentation.metric.probe.orders.calls", 3, countTags},
		{"gauge", "dynamic.instrumentation.metric.probe.orders_latency_ms", 1.5, gaugeTags},
	}, client.take())
	require.ElementsMatch(t, []string{"calls", "latency"}, emitting)

	// Counts are reported as deltas; probes that weren't hit aren't
	// reported.
	setState(0, loader.MetricState{Sum: 10, Updates: 5})
	r.reportAll()
	require.Equal(t, []reportedMetric{
		{"count", "dynamic.instrumentation.metric.probe.orders.calls", 7, countTags},
	}, client.take())

	// The BPF program adds to the sum before it increments the update count,
	// so a read can see the new sum with the old count. The new sum is reported
	// anyway, rather than lost.
	setState(0, loader.MetricState{Sum: 12, Updates: 5})
	r.reportAll()
	require.Equal(t, []reportedMetric{
		{"count", "dynamic.instrumentation.metric.probe.orders.calls", 2, countTags},
	}, client.take())
	setState(0, loader.MetricState{Sum: 12, Updates: 6})
	r.reportAll()
	require.Equal(t, []reportedMetric{
		{"count", "dynamic.instrumentation.metric.probe.orders.calls", 0, countTags},
	}, client.take())

	// A program can be reported on its own, which is how it's reported one
	// last time when it's detached.
	setState(2, loader.MetricState{Value: math.Float64bits(-2), Updates: 2})
	r.report(1)
	require.Equal(t, []reportedMetric{
		{"gauge", "dynamic.instrumentation.metric.probe.orders_latency_ms", -2, gaugeTags},
	}, client.take())

	// Unregistering stops the reports.
	r.unregister(1)
	setState(0, loader.MetricState{Sum: 20, Updates: 6})
	r.reportAll()
	require.Empty(t, client.take())
	// Each probe was reported as emitting once.
	require.ElementsMatch(t, []string{"calls", "latency"}, emitting)
}

// TestMetricsReporterEmittingBeforeLastReport checks that a report doesn't
// return while another report is still notifying probes as emitting. The
// runtime unlinks a program from its process right after the program's last
// report, after which emitting notifications for the program are dropped.
func TestMetricsReporterEmittingBeforeLastReport(t *testing.T) {
	notifying := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	r := newMetricsReporter(&fakeMetricsClient{}, time.Hour, func(ir.ProgramID, ir.ProbeDefinition) {
		once.Do(func() { close(notifying) })
		<-release
	})
	defer r.close()
	program := &ir.Program{Probes: []*ir.Probe{
		metricProbe("calls", "COUNT", "calls", nil, reflect.Invalid),
	}}
	r.register(1, program, func() []loader.MetricState {
		return []loader.MetricState{{Sum: 1, Updates: 1}}
	}, procRuntimeID{})

	// A periodic report finds the probe hit and notifies it as emitting.
	go r.reportAll()
	<-notifying
	// The last report, made when the program is detached, waits for it.
	lastReport := make(chan struct{})
	go func() {
		r.report(1)
		close(lastReport)
	}()
	select {
	case <-lastReport:
		t.Fatal("the last report returned while a probe was being notified as emitting")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-lastReport
}

func TestGaugeValueDecoder(t *testing.T) {
	for _, tc := range []struct {
		kind reflect.Kind
		bits uint64
		want float64
	}{
		{reflect.Float64, math.Float64bits(-3.25), -3.25},
		{reflect.Float32, uint64(math.Float32bits(2.5)), 2.5},
		// The BPF program sign-extends signed integers.
		{reflect.Int8, uint64(math.MaxUint64), -1},
		{reflect.Int64, uint64(1 << 40), 1 << 40},
		{reflect.Uint32, math.MaxUint32, math.MaxUint32},
		{reflect.Bool, 1, 1},
	} {
		decode := gaugeValueDecoder(metricProbe("p", "GAUGE", "m", nil, tc.kind))
		require.Equal(t, tc.want, decode(tc.bits), "%v", tc.kind)
	}
}

func TestSanitizeMetricNameAndTags(t *testing.T) {
	require.Equal(t, "orders_count.v2_", sanitizeMetricName("orders count.v2|"))
	require.Equal(t, "a_b_c:d", sanitizeMetricTag("a|b,c:d"))
	require.Equal(t, "multi_line", sanitizeMetricTag("multi\nline"))
}

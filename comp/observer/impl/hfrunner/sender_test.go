// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hfrunner

import (
	"testing"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/require"
)

type capturedMetric struct {
	name  string
	value float64
}

type testHandle struct {
	metrics []capturedMetric
}

func (h *testHandle) ObserveMetric(sample observerdef.MetricView) {
	h.metrics = append(h.metrics, capturedMetric{name: sample.GetName(), value: sample.GetValue()})
}

func (h *testHandle) ObserveLog(_ observerdef.LogView)               {}
func (h *testHandle) ObserveTrace(_ observerdef.TraceView)           {}
func (h *testHandle) ObserveTraceStats(_ observerdef.TraceStatsView) {}
func (h *testHandle) ObserveProfile(_ observerdef.ProfileView)       {}

func TestObserverSenderRateDropsNegativeDelta(t *testing.T) {
	h := &testHandle{}
	s := &observerSender{handle: h, prev: make(map[string]prevSample)}
	key := metricKey("container.cpu.usage", []string{"container_id:abc"})
	s.prev[key] = prevSample{value: 20, ts: time.Now().Add(-2 * time.Second).UnixNano()}

	s.Rate("container.cpu.usage", 30, "", []string{"container_id:abc"}) // +10 over ~2s
	s.Rate("container.cpu.usage", 5, "", []string{"container_id:abc"})  // reset: should drop

	require.Len(t, h.metrics, 1)
	require.Equal(t, "container.cpu.usage", h.metrics[0].name)
	require.InDelta(t, 5.0, h.metrics[0].value, 0.2)
}

func TestObserverSenderMonotonicCountDropsNegativeDeltaByDefault(t *testing.T) {
	h := &testHandle{}
	s := &observerSender{handle: h, prev: make(map[string]prevSample)}
	key := metricKey("container.io.read", []string{"container_id:abc"})
	s.prev[key] = prevSample{value: 100, ts: time.Now().Add(-1 * time.Second).UnixNano()}

	s.MonotonicCount("container.io.read", 140, "", []string{"container_id:abc"})
	s.MonotonicCount("container.io.read", 12, "", []string{"container_id:abc"}) // reset: should drop

	require.Len(t, h.metrics, 1)
	require.Equal(t, 40.0, h.metrics[0].value)
}

func TestObserverSenderMonotonicCountWithFlushFirstValueHandlesResets(t *testing.T) {
	h := &testHandle{}
	s := &observerSender{handle: h, prev: make(map[string]prevSample)}

	s.MonotonicCountWithFlushFirstValue("container.io.read", 7, "", []string{"container_id:abc"}, true)  // first sample flushes
	s.MonotonicCountWithFlushFirstValue("container.io.read", 20, "", []string{"container_id:abc"}, true) // +13
	s.MonotonicCountWithFlushFirstValue("container.io.read", 4, "", []string{"container_id:abc"}, true)  // reset: flush raw value

	require.Len(t, h.metrics, 3)
	require.Equal(t, 7.0, h.metrics[0].value)
	require.Equal(t, 13.0, h.metrics[1].value)
	require.Equal(t, 4.0, h.metrics[2].value)
}

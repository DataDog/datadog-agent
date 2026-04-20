// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package cnm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

func TestReceiverStartShutdown(t *testing.T) {
	sink := consumertest.NewNop()
	conns := makeTestConnections(1)

	recv := newCNMReceiver(testConfig(), zap.NewNop(), sink, nil, nil, nil)
	recv.source = &mockSource{conns: conns}

	require.NoError(t, recv.Start(context.Background(), componenttest.NewNopHost()))
	require.NoError(t, recv.Shutdown(context.Background()))
}

func TestReceiverCollectPushesMetrics(t *testing.T) {
	sink := new(consumertest.MetricsSink)
	conns := makeTestConnections(5)

	recv := newCNMReceiver(testConfig(), zap.NewNop(), sink, nil, nil, nil)
	recv.source = &mockSource{conns: conns}

	recv.collect(context.Background())

	allMetrics := sink.AllMetrics()
	require.Len(t, allMetrics, 1)

	md := allMetrics[0]
	require.Equal(t, 1, md.ResourceMetrics().Len())

	sm := md.ResourceMetrics().At(0).ScopeMetrics().At(0)
	assert.Equal(t, scopeName, sm.Scope().Name())

	// 9 metric definitions (7 sums + 2 gauges)
	assert.Equal(t, 9, sm.Metrics().Len())

	// Each metric should have 5 data points (one per connection)
	for i := 0; i < sm.Metrics().Len(); i++ {
		m := sm.Metrics().At(i)
		switch m.Type() {
		case pmetric.MetricTypeSum:
			assert.Equal(t, 5, m.Sum().DataPoints().Len(), "metric %s", m.Name())
		case pmetric.MetricTypeGauge:
			assert.Equal(t, 5, m.Gauge().DataPoints().Len(), "metric %s", m.Name())
		}
	}
}

func TestReceiverCollectError(t *testing.T) {
	sink := new(consumertest.MetricsSink)

	recv := newCNMReceiver(testConfig(), zap.NewNop(), sink, nil, nil, nil)
	recv.source = &mockSource{err: errors.New("tracer error")}

	// Should not panic, should log error and continue
	recv.collect(context.Background())

	assert.Empty(t, sink.AllMetrics())
}

func TestReceiverEmptyConnections(t *testing.T) {
	sink := new(consumertest.MetricsSink)
	conns := makeTestConnections(0)

	recv := newCNMReceiver(testConfig(), zap.NewNop(), sink, nil, nil, nil)
	recv.source = &mockSource{conns: conns}

	recv.collect(context.Background())

	// No metrics pushed for empty connections
	assert.Empty(t, sink.AllMetrics())
}

func TestReceiverCollectInterval(t *testing.T) {
	sink := new(consumertest.MetricsSink)
	conns := makeTestConnections(1)

	cfg := testConfig()
	cfg.CheckInterval = 50 * time.Millisecond

	recv := newCNMReceiver(cfg, zap.NewNop(), sink, nil, nil, nil)
	recv.source = &mockSource{conns: conns}

	require.NoError(t, recv.Start(context.Background(), componenttest.NewNopHost()))

	// Wait for a few collection cycles
	require.Eventually(t, func() bool {
		return len(sink.AllMetrics()) >= 3
	}, 2*time.Second, 10*time.Millisecond)

	require.NoError(t, recv.Shutdown(context.Background()))
}

func TestReceiverShutdownDrainsGoroutine(t *testing.T) {
	sink := consumertest.NewNop()
	conns := makeTestConnections(1)

	recv := newCNMReceiver(testConfig(), zap.NewNop(), sink, nil, nil, nil)
	recv.source = &mockSource{conns: conns}

	require.NoError(t, recv.Start(context.Background(), componenttest.NewNopHost()))
	require.NoError(t, recv.Shutdown(context.Background()))

	// Verify the WaitGroup is fully drained (Shutdown returned without hanging)
	recv.wg.Wait()
}

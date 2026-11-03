// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"context"
	"sync"
	"testing"
	"time"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
)

func TestLogTelemetryBatchTotals(t *testing.T) {
	component := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(component)
	for _, source := range []string{"internal", "kubelet", "containers"} {
		for i := 0; i < 5; i++ {
			tel.incrementLogsInFlight(source)
			tel.recordLogAccepted(source, 128)
			tel.decrementLogsInFlight(source)
		}
		tel.recordObservationsDropped("logs", source, 3)
		for _, priority := range []string{"high", "medium", "low"} {
			tel.recordInputRateLimiterDropped(source, priority)
		}
	}
	// Metric telemetry remains immediate while log deltas await publication.
	tel.recordMetricAccepted("check")
	families, err := component.Gather(telemetry.NoFilter)
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == "observer__"+telemetryObservationsAccepted {
			for _, metric := range family.GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetName() == "kind" {
						require.Equal(t, "metrics", label.GetValue())
					}
				}
			}
		}
	}
	tel.flushLogTelemetry()
	tel.flushLogTelemetry() // A second publisher must not duplicate a drained delta.
	for _, source := range []string{"internal", "kubelet", "containers"} {
		require.Equal(t, 5.0, observerMetric(t, component, telemetryObservationsAccepted, map[string]string{"kind": "logs", "source": source}).GetCounter().GetValue())
		require.Equal(t, 640.0, observerMetric(t, component, telemetryLogsAcceptedBytes, map[string]string{"source": source}).GetCounter().GetValue())
		require.Equal(t, 3.0, observerMetric(t, component, telemetryObservationsDropped, map[string]string{"kind": "logs", "source": source}).GetCounter().GetValue())
		require.Equal(t, 0.0, observerMetric(t, component, telemetryLogsInFlightCount, map[string]string{"log_source": source}).GetGauge().GetValue())
		for _, priority := range []string{"high", "medium", "low"} {
			require.Equal(t, 1.0, observerMetric(t, component, telemetryLogsInputRateLimiterDropped, map[string]string{"source": source, "priority": priority}).GetCounter().GetValue())
		}
	}
}

func TestLogTelemetryBatchConcurrentPublication(t *testing.T) {
	component := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(component)
	const workers, samples = 8, 512
	sources := []string{"internal", "kubelet", "containers"}
	var expected [3]int
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		source := sources[worker%3]
		expected[worker%3] += samples
		wg.Go(func() {
			for i := 0; i < samples; i++ {
				tel.incrementLogsInFlight(source)
				tel.recordLogAccepted(source, 64)
				tel.decrementLogsInFlight(source)
				if i%31 == 0 {
					tel.flushLogTelemetry()
				}
			}
		})
	}
	wg.Wait()
	tel.flushLogTelemetry()
	for i, source := range sources {
		require.Equal(t, float64(expected[i]), observerMetric(t, component, telemetryObservationsAccepted, map[string]string{"kind": "logs", "source": source}).GetCounter().GetValue())
		require.Equal(t, float64(expected[i]*64), observerMetric(t, component, telemetryLogsAcceptedBytes, map[string]string{"source": source}).GetCounter().GetValue())
		require.Equal(t, 0.0, observerMetric(t, component, telemetryLogsInFlightCount, map[string]string{"log_source": source}).GetGauge().GetValue())
	}
}

func TestLogTelemetryBatchUnknownLabelsRemainImmediate(t *testing.T) {
	component := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(component)
	tel.recordLogAccepted("custom", 12)
	tel.recordObservationsDropped("logs", "custom", 2)
	tel.recordInputRateLimiterDropped("custom", "high")
	tel.recordInputRateLimiterDropped("containers", "custom")
	require.Equal(t, 1.0, observerMetric(t, component, telemetryObservationsAccepted, map[string]string{"kind": "logs", "source": "custom"}).GetCounter().GetValue())
	require.Equal(t, 12.0, observerMetric(t, component, telemetryLogsAcceptedBytes, map[string]string{"source": "custom"}).GetCounter().GetValue())
	require.Equal(t, 2.0, observerMetric(t, component, telemetryObservationsDropped, map[string]string{"kind": "logs", "source": "custom"}).GetCounter().GetValue())
	require.Equal(t, 1.0, observerMetric(t, component, telemetryLogsInputRateLimiterDropped, map[string]string{"source": "custom", "priority": "high"}).GetCounter().GetValue())
	require.Equal(t, 1.0, observerMetric(t, component, telemetryLogsInputRateLimiterDropped, map[string]string{"source": "containers", "priority": "custom"}).GetCounter().GetValue())
}

func startLogTelemetryObserver(t *testing.T, tel *observerTelemetry, ticks <-chan time.Time) (*observerImpl, func()) {
	t.Helper()
	obs := &observerImpl{engine: newEngine(engineConfig{storage: newTimeSeriesStorage()}), metricsCh: make(chan observation, 1), logsCh: make(chan observation, 1), telemetry: tel}
	done := make(chan struct{})
	go func() { obs.runWithLogTelemetryTicker(ticks); close(done) }()
	return obs, func() { close(obs.metricsCh); close(obs.logsCh); <-done }
}

type notifyingLogCounter struct {
	telemetry.Counter
	published chan struct{}
}

func (c notifyingLogCounter) Add(value float64, labels ...string) {
	c.Counter.Add(value, labels...)
	c.published <- struct{}{}
}

func TestLogTelemetryBatchIdleTickerDoesNotAdvanceDetection(t *testing.T) {
	component := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(component)
	published := make(chan struct{}, 1)
	tel.observationsAccepted = notifyingLogCounter{tel.observationsAccepted, published}
	clk := clock.NewMock()
	ticker := clk.Ticker(logTelemetryPublishInterval)
	defer ticker.Stop()
	obs, stop := startLogTelemetryObserver(t, tel, ticker.C)
	defer stop()
	tel.recordLogAccepted("containers", 16)
	clk.Add(logTelemetryPublishInterval)
	select {
	case <-published:
	case <-time.After(5 * time.Second):
		t.Fatal("mock-clock tick did not publish idle log telemetry")
	}
	require.Equal(t, 1.0, observerMetric(t, component, telemetryObservationsAccepted, map[string]string{"kind": "logs", "source": "containers"}).GetCounter().GetValue())
	require.Zero(t, obs.engine.lastAnalyzedDataTime)
}

func TestLogTelemetryBatchFlushAndWorkerExit(t *testing.T) {
	t.Run("explicit_flush_samples_current_in_flight", func(t *testing.T) {
		component := telemetryimpl.NewMock(t)
		tel := newObserverTelemetry(component)
		obs, stop := startLogTelemetryObserver(t, tel, nil)
		defer stop()
		tel.incrementLogsInFlight("containers")
		tel.recordLogAccepted("containers", 16)
		obs.Flush()
		require.Equal(t, 1.0, observerMetric(t, component, telemetryObservationsAccepted, map[string]string{"kind": "logs", "source": "containers"}).GetCounter().GetValue())
		require.Equal(t, 1.0, observerMetric(t, component, telemetryLogsInFlightCount, map[string]string{"log_source": "containers"}).GetGauge().GetValue())
		tel.decrementLogsInFlight("containers")
		obs.Flush()
		require.Equal(t, 0.0, observerMetric(t, component, telemetryLogsInFlightCount, map[string]string{"log_source": "containers"}).GetGauge().GetValue())
	})
	t.Run("worker_exit", func(t *testing.T) {
		component := telemetryimpl.NewMock(t)
		tel := newObserverTelemetry(component)
		_, stop := startLogTelemetryObserver(t, tel, nil)
		tel.recordLogAccepted("containers", 16)
		stop()
		require.Equal(t, 1.0, observerMetric(t, component, telemetryObservationsAccepted, map[string]string{"kind": "logs", "source": "containers"}).GetCounter().GetValue())
	})
}

func TestLogTelemetryBatchLifecycleStopDrainsTail(t *testing.T) {
	component := telemetryimpl.NewMock(t)
	lifecycle := &testLifecycle{}
	cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  reporting:
    events:
      enabled: true
  logs:
    internal:
      enabled: false
`)
	provided, err := NewComponent(Requires{Lifecycle: lifecycle, Config: cfg, Telemetry: component})
	require.NoError(t, err)
	obs := provided.Comp.(*observerImpl)
	defer close(obs.metricsCh)
	defer close(obs.logsCh)
	obs.telemetry.recordLogAccepted("containers", 16)
	for i := len(lifecycle.hooks) - 1; i >= 0; i-- {
		if stop := lifecycle.hooks[i].OnStop; stop != nil {
			require.NoError(t, stop(context.Background()))
		}
	}
	require.Equal(t, 1.0, observerMetric(t, component, telemetryObservationsAccepted, map[string]string{"kind": "logs", "source": "containers"}).GetCounter().GetValue())
}

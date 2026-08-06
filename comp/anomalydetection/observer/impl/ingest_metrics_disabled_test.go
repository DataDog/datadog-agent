// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sync"
	"testing"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// TestObserverDropsMetricsWhenIngestMetricsDisabled verifies that when
// observer.ingest_metrics.enabled is false, the handle drops external
// ObserveMetric calls while logs and log-derived virtual metrics still
// reach the engine.
func TestObserverDropsMetricsWhenIngestMetricsDisabled(t *testing.T) {
	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	defaultFilter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)

	storage := newTimeSeriesStorage()
	extractor := NewLogMetricsExtractor(DefaultLogMetricsExtractorConfig())
	eng := newEngine(engineConfig{
		storage:    storage,
		extractors: []observerdef.LogMetricsExtractor{extractor},
	})

	obs := &observerImpl{
		engine:               eng,
		obsCh:                make(chan observation, 16),
		telemetry:            newObserverTelemetry(telComp),
		ingestMetricsEnabled: false,
		metricFilter:         defaultFilter,
	}
	obs.handleFunc = obs.innerHandle

	var (
		wg        sync.WaitGroup
		closeOnce sync.Once
	)
	stopFn := func() {
		// close is idempotent via Once; wg.Wait() is safe to call multiple times.
		closeOnce.Do(func() { close(obs.obsCh) })
		wg.Wait()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		obs.run()
	}()
	// Guarantee cleanup even if an assertion calls t.Fatal before stopFn().
	t.Cleanup(stopFn)

	h := obs.GetHandle("dogstatsd")

	drop, ok := h.(*metricDropHandle)
	require.Truef(t, ok, `GetHandle("dogstatsd") returned %T, want *metricDropHandle`, h)

	assert.True(t, drop.ObserveMetricAndReportDrop(&metricObs{
		name:      "system.cpu.user",
		value:     50,
		timestamp: 1000,
	}), "ObserveMetricAndReportDrop should report true when dropped by configuration (signals recorder to write Dropped=true)")

	drop.ObserveMetric(&metricObs{
		name:      "system.mem.used",
		value:     1024,
		timestamp: 1000,
	})

	drop.ObserveLog(&logObs{
		content:     "Request failed with unexpected error",
		status:      "error",
		tags:        []string{"service:web"},
		timestampMs: 1_000_000,
	})

	// Flush: close signals EOF; run() drains all buffered items before returning,
	// so the ObserveLog above is guaranteed to be processed before we assert storage.
	stopFn()

	dogstatsdSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: "dogstatsd"})
	assert.Empty(t, dogstatsdSeries,
		"external metrics must not be stored when observer.ingest_metrics.enabled=false")

	extractorSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: extractor.Name()})
	require.NotEmpty(t, extractorSeries,
		"log-extractor virtual metrics must keep flowing into storage even when observer.ingest_metrics.enabled=false")

	requireNoCounterMetricForNameBySource(t, telemetryFilteredMetrics, "dogstatsd", telComp)
}

func TestInternalAgentMetricsAreIngestedAndObserverTelemetryIsDropped(t *testing.T) {
	defaultFilter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)

	storage := newTimeSeriesStorage()
	eng := newEngine(engineConfig{storage: storage})

	obs := &observerImpl{
		engine:               eng,
		obsCh:                make(chan observation, 16),
		ingestMetricsEnabled: true,
		metricFilter:         defaultFilter,
	}
	obs.handleFunc = obs.innerHandle

	var (
		wg        sync.WaitGroup
		closeOnce sync.Once
	)
	stopFn := func() {
		closeOnce.Do(func() { close(obs.obsCh) })
		wg.Wait()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		obs.run()
	}()
	t.Cleanup(stopFn)

	h := obs.GetHandle("dogstatsd")
	h.ObserveMetric(&metricObs{
		name:      "system.cpu.user",
		value:     50,
		timestamp: 1000,
	})
	h.ObserveMetric(&metricObs{
		name:      "datadog.agent.running",
		value:     1,
		timestamp: 1000,
	})
	h.ObserveMetric(&metricObs{
		name:      observerTelemetryMetricPrefix + "metrics.filtered",
		value:     1,
		timestamp: 1000,
	})

	stopFn()

	dogstatsdSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: "dogstatsd"})
	require.Len(t, dogstatsdSeries, 1)
	assert.Equal(t, "system.cpu.user", dogstatsdSeries[0].Name)

	agentSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: observerdef.AgentNamespace})
	require.Len(t, agentSeries, 1)
	assert.Equal(t, "datadog.agent.running", agentSeries[0].Name)

	workloadSeries := storage.ListSeries(observerdef.WorkloadSeriesFilter())
	require.Len(t, workloadSeries, 2)
}

func TestIngestMetricSyncAllowsInternalAgentMetricsAndDropsObserverTelemetry(t *testing.T) {
	defaultFilter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)

	storage := newTimeSeriesStorage()
	obs := &observerImpl{
		engine:       newEngine(engineConfig{storage: storage}),
		metricFilter: defaultFilter,
	}

	obs.IngestMetricSync("dogstatsd", &metricObs{
		name:      "system.cpu.user",
		value:     50,
		timestamp: 1000,
	})
	obs.IngestMetricSync("dogstatsd", &metricObs{
		name:      "datadog.agent.running",
		value:     1,
		timestamp: 1000,
	})
	obs.IngestMetricSync("dogstatsd", &metricObs{
		name:      observerTelemetryMetricPrefix + "metrics.filtered",
		value:     1,
		timestamp: 1000,
	})

	dogstatsdSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: "dogstatsd"})
	require.Len(t, dogstatsdSeries, 1)
	assert.Equal(t, "system.cpu.user", dogstatsdSeries[0].Name)

	agentSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: observerdef.AgentNamespace})
	require.Len(t, agentSeries, 1)
	assert.Equal(t, "datadog.agent.running", agentSeries[0].Name)
}

// TestMetricDropHandle covers the metricDropHandle wrapper in isolation.
func TestMetricDropHandle(t *testing.T) {
	inner := &countingHandle{}
	wrap := &metricDropHandle{inner: inner}

	wrap.ObserveMetric(&sampleNoSource{name: "any.metric"})
	assert.Equal(t, 0, inner.received,
		"metricDropHandle: inner.received = %d, want 0 (ObserveMetric/Trace/TraceStats must be dropped)", inner.received)
	assert.True(t, wrap.ObserveMetricAndReportDrop(&sampleNoSource{name: "any.metric"}),
		"ObserveMetricAndReportDrop reports true (config drop) so recordingHandle writes Dropped=true")

	wrap.ObserveLog(&logObs{content: "hi", timestampMs: 1})
	assert.Equal(t, 1, inner.logReceived,
		"metricDropHandle: inner.logReceived = %d, want 1 (ObserveLog must forward to inner)", inner.logReceived)
}

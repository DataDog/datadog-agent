// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// TestObserverDropsMetricsWhenIngestMetricsDisabled verifies that when
// observer.ingest_metrics.enabled is false:
//
//   - the "all-metrics" handle is wrapped in metricDropHandle so
//     ObserveMetric calls do not reach engine storage,
//   - ObserveMetricAndReportDrop returns false (the call was dropped by
//     configuration, not by a full channel),
//   - ObserveLog still passes through to the engine and the
//     LogMetricsExtractor's virtual metrics land in storage under the
//     extractor's namespace.
//
// The third assertion is the structural regression guard: the gate must
// only block externally-ingested metrics on the handle path. Virtual
// metrics produced inside the engine by LogMetricsExtractors must keep
// flowing because they are what enables log-only anomaly detection when
// external metric ingestion is disabled.
func TestObserverDropsMetricsWhenIngestMetricsDisabled(t *testing.T) {
	storage := newTimeSeriesStorage()
	extractor := NewLogMetricsExtractor(DefaultLogMetricsExtractorConfig())
	eng := newEngine(engineConfig{
		storage:    storage,
		extractors: []observerdef.LogMetricsExtractor{extractor},
	})

	th := newTelemetryHandler(noopsimpl.GetCompatComponent())
	obs := &observerImpl{
		engine:               eng,
		obsCh:                make(chan observation, 16),
		telemetryHandler:     th,
		dropCounter:          th.telemetryCounters[telemetryObsChannelDropped],
		ingestMetricsEnabled: false,
	}
	obs.handleFunc = obs.innerHandle

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		obs.run()
	}()

	h := obs.GetHandle("all-metrics")

	dropHandle, ok := h.(*metricDropHandle)
	require.Truef(t, ok, "expected metricDropHandle wrapper, got %T", h)

	assert.False(t, dropHandle.ObserveMetricAndReportDrop(&metricObs{
		name:      "system.cpu.user",
		value:     50,
		timestamp: 1000,
	}), "ObserveMetricAndReportDrop should report false when dropped by configuration")

	dropHandle.ObserveMetric(&metricObs{
		name:      "system.mem.used",
		value:     1024,
		timestamp: 1000,
	})

	dropHandle.ObserveLog(&logObs{
		content:     []byte("Request completed in 45ms"),
		status:      "info",
		tags:        []string{"service:web"},
		timestampMs: 1_000_000,
	})

	close(obs.obsCh)
	wg.Wait()

	allMetricsSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: "all-metrics"})
	assert.Empty(t, allMetricsSeries,
		"external metrics must not be stored when observer.ingest_metrics.enabled=false")

	extractorSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: extractor.Name()})
	require.NotEmpty(t, extractorSeries,
		"log-extractor virtual metrics must keep flowing into storage even when observer.ingest_metrics.enabled=false")
}

// TestMetricDropHandleUnit covers the wrapper in isolation, mirroring
// the pattern used in system_filter_test.go for hfFilteredHandle.
func TestMetricDropHandleUnit(t *testing.T) {
	inner := &countingHandle{}
	wrap := &metricDropHandle{inner: inner}

	wrap.ObserveMetric(&sampleNoSource{name: "any.metric"})
	wrap.ObserveTrace(nil)
	wrap.ObserveTraceStats(nil)
	assert.Zero(t, inner.received, "ObserveMetric/Trace/TraceStats must not reach inner")
	assert.False(t, wrap.ObserveMetricAndReportDrop(&sampleNoSource{name: "any.metric"}),
		"ObserveMetricAndReportDrop reports false (config drop, not channel-full)")

	wrap.ObserveLog(&logObs{content: []byte("hi"), timestampMs: 1})
	wrap.ObserveProfile(nil)
}

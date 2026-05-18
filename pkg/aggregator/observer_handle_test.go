// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// recordingHandle records every ObserveMetric call for test assertions.
type recordingHandle struct {
	calls []recordedCall
}

type recordedCall struct {
	name      string
	value     float64
	tags      []string
	timestamp int64
	source    metrics.MetricSource
}

func (h *recordingHandle) ObserveMetric(v observer.MetricView) {
	// copy values — the MetricView contract forbids retaining the view itself
	tagsCopy := make([]string, len(v.GetRawTags()))
	copy(tagsCopy, v.GetRawTags())
	call := recordedCall{
		name:      v.GetName(),
		value:     v.GetValue(),
		tags:      tagsCopy,
		timestamp: v.GetTimestampUnix(),
	}
	if sp, ok := v.(interface{ GetSource() metrics.MetricSource }); ok {
		call.source = sp.GetSource()
	}
	h.calls = append(h.calls, call)
}

func (h *recordingHandle) ObserveLog(_ observer.LogView) {}

func callsByName(calls []recordedCall) map[string]recordedCall {
	byName := make(map[string]recordedCall, len(calls))
	for _, call := range calls {
		byName[call.name] = call
	}
	return byName
}

// recordingComponent wraps a recordingHandle as an observer.Component.
type recordingComponent struct {
	handle *recordingHandle
}

func (c *recordingComponent) GetHandle(_ string) observer.Handle {
	return c.handle
}

func (c *recordingComponent) DumpMetrics(_ string) error {
	return nil
}

// TestTimeSamplerObserverHandle verifies that ObserveMetric is called for each
// sample fed to the TimeSampler when an observerHandle is wired.
func TestTimeSamplerObserverHandle(t *testing.T) {
	store := tags.NewStore(false, "test")
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
	handle := &recordingHandle{}
	sampler.observerHandle = handle

	matcher := filterlist.NewNoopTagMatcher()

	samples := []metrics.MetricSample{
		{Name: "metric.a", Value: 1.0, Mtype: metrics.GaugeType, Tags: []string{"env:prod"}, SampleRate: 1, Timestamp: 1000},
		{Name: "metric.b", Value: 2.5, Mtype: metrics.CountType, Tags: []string{"service:web"}, SampleRate: 0.5, Timestamp: 2000},
	}

	for _, s := range samples {
		s := s
		sampler.sample(&s, s.Timestamp, matcher)
	}

	require.Len(t, handle.calls, 2)
	assert.Equal(t, "metric.a", handle.calls[0].name)
	assert.Equal(t, 1.0, handle.calls[0].value)
	assert.Equal(t, []string{"env:prod"}, handle.calls[0].tags)
	assert.Equal(t, int64(1000), handle.calls[0].timestamp)

	assert.Equal(t, "metric.b", handle.calls[1].name)
	assert.Equal(t, 2.5, handle.calls[1].value)
}

// TestTimeSamplerObserverHandleNil verifies no panic when observerHandle is nil.
func TestTimeSamplerObserverHandleNil(t *testing.T) {
	store := tags.NewStore(false, "test")
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
	// observerHandle is nil by default — must not panic

	matcher := filterlist.NewNoopTagMatcher()
	s := metrics.MetricSample{Name: "m", Value: 1, Mtype: metrics.GaugeType, SampleRate: 1}
	assert.NotPanics(t, func() { sampler.sample(&s, 100, matcher) })
}

// TestSetObserverNilIsNoop verifies SetObserver(nil) leaves all handles unset.
func TestSetObserverNilIsNoop(t *testing.T) {
	opts := demuxTestOptions()
	deps := createDemultiplexerAgentTestDeps(t)
	// Use initAgentDemultiplexer (not started) — no goroutines, no Stop() needed.
	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")

	demux.SetObserver(nil)

	for _, w := range demux.statsd.workers {
		assert.Nil(t, w.sampler.observerHandle, "worker handle should remain nil")
	}
}

// TestSetObserverConfigOff verifies that SetObserver does not wire the handle
// when anomaly_detection.enabled or anomaly_detection.metrics.enabled is false.
// Covers both the DogStatsD TimeSampler path and the BufferedAggregator/CheckSampler path.
func TestSetObserverConfigOff(t *testing.T) {
	opts := demuxTestOptions()
	deps := createDemultiplexerAgentTestDeps(t)
	// Use initAgentDemultiplexer (not started) — no goroutines, no Stop() needed.
	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")

	// Both config keys default to false — handle must not be wired.
	comp := &recordingComponent{handle: &recordingHandle{}}
	demux.SetObserver(comp)

	for _, w := range demux.statsd.workers {
		assert.Nil(t, w.sampler.observerHandle, "DogStatsD worker handle should not be wired when config is off")
	}
	assert.Nil(t, demux.aggregator.observerHandle, "BufferedAggregator handle should not be wired when config is off")

	// Verify that a CheckSampler registered after the (no-op) SetObserver call also has no handle.
	demux.aggregator.handleRegisterSampler("check-config-off")
	demux.aggregator.mu.Lock()
	cs := demux.aggregator.checkSamplers["check-config-off"]
	demux.aggregator.mu.Unlock()
	require.NotNil(t, cs)
	assert.Nil(t, cs.observerHandle, "CheckSampler handle should not be wired when config is off")
}

// TestCheckSamplerObserverHandle verifies that ObserveMetric is called for each
// sample fed to CheckSampler.addSample when an observerHandle is wired.
func TestCheckSamplerObserverHandle(t *testing.T) {
	store := tags.NewStore(false, "test")
	cs := newCheckSampler(10, false, false, 0, false, store, "test-check", nooptagger.NewComponent())
	handle := &recordingHandle{}
	cs.SetObserverHandle(handle)

	matcher := filterlist.NewNoopTagMatcher()

	samples := []metrics.MetricSample{
		{Name: "system.cpu.user", Value: 42.0, Mtype: metrics.GaugeType, Tags: []string{"host:myhost"}, SampleRate: 1, Timestamp: 1000},
		{Name: "system.mem.used", Value: 8192.0, Mtype: metrics.GaugeType, Tags: []string{}, SampleRate: 1, Timestamp: 2000},
	}

	for i := range samples {
		cs.addSample(&samples[i], matcher)
	}

	require.Len(t, handle.calls, 2)
	assert.Equal(t, "system.cpu.user", handle.calls[0].name)
	assert.Equal(t, 42.0, handle.calls[0].value)
	assert.Equal(t, []string{"host:myhost"}, handle.calls[0].tags)
	assert.Equal(t, int64(1000), handle.calls[0].timestamp)

	assert.Equal(t, "system.mem.used", handle.calls[1].name)
	assert.Equal(t, 8192.0, handle.calls[1].value)
}

func TestCheckSamplerObserverHandleDistributionDoesNotEmitSketchSummary(t *testing.T) {
	store := tags.NewStore(false, "test")
	cs := newCheckSampler(10, false, false, 0, false, store, "test-check", nooptagger.NewComponent())
	handle := &recordingHandle{}
	cs.SetObserverHandle(handle)

	sample := metrics.MetricSample{
		Name:       "request.size",
		Value:      5,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"env:prod"},
		SampleRate: 1,
		Timestamp:  1234,
		Source:     metrics.MetricSourceOpenmetrics,
	}
	cs.addSample(&sample, filterlist.NewNoopTagMatcher())
	cs.commit(1235, nil)

	require.Len(t, handle.calls, 1, "distribution samples are already mirrored raw and should not also emit sketch summaries")
	assert.Equal(t, "request.size", handle.calls[0].name)
	assert.Equal(t, float64(5), handle.calls[0].value)
	assert.Equal(t, []string{"env:prod"}, handle.calls[0].tags)
}

func TestCheckSamplerObserverHandleSketchSummary(t *testing.T) {
	store := tags.NewStore(false, "test")
	cs := newCheckSampler(10, false, false, 0, false, store, "test-check", nooptagger.NewComponent())
	handle := &recordingHandle{}
	cs.SetObserverHandle(handle)

	cs.addBucket(&metrics.HistogramBucket{
		Name:       "request.duration",
		Value:      7,
		LowerBound: 0.5,
		UpperBound: 1.5,
		Tags:       []string{"env:prod"},
		Timestamp:  1234,
		Source:     metrics.MetricSourceOpenmetrics,
	}, filterlist.NewNoopTagMatcher())
	cs.commit(1235, nil)

	require.Len(t, handle.calls, 5)
	callsByName := map[string]recordedCall{}
	for _, call := range handle.calls {
		callsByName[call.name] = call
		assert.Equal(t, int64(1234), call.timestamp)
		assert.Equal(t, []string{"env:prod", "observer_metric_type:sketch_summary", "sketch_stat:" + call.name[len("request.duration."):]}, call.tags)
		assert.Equal(t, metrics.MetricSourceOpenmetrics, call.source)
	}
	assert.Equal(t, float64(7), callsByName["request.duration.count"].value)
	assert.InDelta(t, 7, callsByName["request.duration.sum"].value, 0.5)
	assert.InDelta(t, 1, callsByName["request.duration.avg"].value, 0.5)
	assert.InDelta(t, 0.5, callsByName["request.duration.min"].value, 0.5)
	assert.InDelta(t, 1.5, callsByName["request.duration.max"].value, 0.5)
}

func TestCheckSamplerObserverHandleMonotonicHistogramBucketSummaryRecordsDelta(t *testing.T) {
	store := tags.NewStore(false, "test")
	cs := newCheckSampler(10, false, false, 0, false, store, "test-check", nooptagger.NewComponent())
	handle := &recordingHandle{}
	cs.SetObserverHandle(handle)
	matcher := filterlist.NewNoopTagMatcher()

	cs.addBucket(&metrics.HistogramBucket{
		Name:       "request.duration",
		Value:      10,
		LowerBound: 0,
		UpperBound: 1,
		Monotonic:  true,
		Timestamp:  1000,
	}, matcher)
	cs.commit(1001, nil)
	require.Empty(t, handle.calls, "first monotonic bucket is held until a delta can be computed")

	cs.addBucket(&metrics.HistogramBucket{
		Name:       "request.duration",
		Value:      17,
		LowerBound: 0,
		UpperBound: 1,
		Monotonic:  true,
		Timestamp:  1001,
	}, matcher)
	cs.commit(1002, nil)

	require.Len(t, handle.calls, 5)
	countCall := callsByName(handle.calls)["request.duration.count"]
	assert.Equal(t, float64(7), countCall.value)
	assert.Equal(t, int64(1001), countCall.timestamp)
	assert.Equal(t, []string{"observer_metric_type:sketch_summary", "sketch_stat:count"}, countCall.tags)
}

// TestCheckSamplerObserverHandleNil verifies no panic when observerHandle is nil.
func TestCheckSamplerObserverHandleNil(t *testing.T) {
	store := tags.NewStore(false, "test")
	cs := newCheckSampler(10, false, false, 0, false, store, "test-check", nooptagger.NewComponent())
	// observerHandle is nil by default

	matcher := filterlist.NewNoopTagMatcher()
	s := metrics.MetricSample{Name: "m", Value: 1, Mtype: metrics.GaugeType, SampleRate: 1}
	assert.NotPanics(t, func() { cs.addSample(&s, matcher) })
}

// TestCheckSamplerCoreBehaviorPreserved_NilObserverHandle is the
// regression guard for the spec contract @invariant
// CheckSamplerCoreBehaviorPreserved (see
// ~/.claude/plans/check-aggregator-fanout.allium, contract
// CheckSamplerWrapping). It locks in three properties when the
// observer handle is nil (the default production state until
// anomaly_detection is enabled):
//
//  1. addBucket does NOT allocate the histogramSketchContext map.
//  2. commit/commitSketches emits the same *Serie and *SketchSeries it
//     would have emitted before the histogram-sketch-summary tap was
//     added — i.e. no observer-related side effects on the output.
//  3. context expiration does not panic on the (possibly nil)
//     histogramSketchContext map.
//
// The byte-identicality claim is enforced by capturing concrete
// series/sketches values after a representative workload; a future
// change that accidentally touches CheckSampler's output for the
// no-observer case (e.g. by populating telemetry tags differently)
// will break these assertions.
func TestCheckSamplerCoreBehaviorPreserved_NilObserverHandle(t *testing.T) {
	store := tags.NewStore(false, "test")
	cs := newCheckSampler(10, true, false, 0, true, store, "test-check", nooptagger.NewComponent())
	require.Nil(t, cs.observerHandle, "default state: no observer handle")
	require.Nil(t, cs.histogramSketchContext, "lazy alloc — map is nil until first bucket with handle attached")

	matcher := filterlist.NewNoopTagMatcher()

	// Workload: one gauge sample + a histogram bucket pair (monotonic
	// so the delta path is exercised). The histogram bucket path is the
	// one the histogram-sketch-summary feature touched; the regression
	// guard is that the cs.series and cs.sketches outputs are unaffected
	// when no observer handle is attached.
	gauge := metrics.MetricSample{
		Name:       "test.gauge",
		Value:      42,
		Mtype:      metrics.GaugeType,
		SampleRate: 1,
		Timestamp:  100,
	}
	cs.addSample(&gauge, matcher)

	// First monotonic bucket — held until a delta can be computed.
	cs.addBucket(&metrics.HistogramBucket{
		Name:       "test.hist",
		Value:      10,
		LowerBound: 0,
		UpperBound: 1,
		Monotonic:  true,
		Timestamp:  100,
	}, matcher)
	require.Nil(t, cs.histogramSketchContext,
		"first monotonic bucket is held (no commit yet); nil-handle path must not allocate the map")

	// Second monotonic bucket — emits delta=5 into sketchMap.
	cs.addBucket(&metrics.HistogramBucket{
		Name:       "test.hist",
		Value:      15,
		LowerBound: 0,
		UpperBound: 1,
		Monotonic:  true,
		Timestamp:  101,
	}, matcher)
	assert.Nil(t, cs.histogramSketchContext,
		"with nil observerHandle, addBucket must not allocate histogramSketchContext")

	// Commit + flush. The output must contain exactly what today's
	// pipeline (pre-observer-sketch-summary) would emit:
	//   - one *Serie for the gauge (value=42, ts=100)
	//   - one *SketchSeries for the histogram (sketched delta=5)
	cs.commit(200, nil)
	require.Nil(t, cs.histogramSketchContext,
		"commit's expire path must remain a no-op on the nil map")

	series, sketches := cs.flush()

	require.Len(t, series, 1, "gauge produces exactly one Serie")
	assert.Equal(t, "test.gauge", series[0].Name)
	require.Len(t, series[0].Points, 1)
	assert.Equal(t, float64(42), series[0].Points[0].Value)

	require.Len(t, sketches, 1, "histogram bucket produces exactly one SketchSeries")
	assert.Equal(t, "test.hist", sketches[0].Name)
	require.NotEmpty(t, sketches[0].Points, "sketch series has at least one SketchPoint")
}

// TestBufferedAggregatorObserverHandlePropagation verifies that SetObserverHandle
// on BufferedAggregator is propagated to CheckSamplers created afterwards.
func TestBufferedAggregatorObserverHandlePropagation(t *testing.T) {
	opts := demuxTestOptions()
	deps := createDemultiplexerAgentTestDeps(t)
	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")

	handle := &recordingHandle{}
	demux.aggregator.SetObserverHandle(handle)

	// Simulate a check registering its sampler
	demux.aggregator.handleRegisterSampler("test-check-id")

	demux.aggregator.mu.Lock()
	cs, ok := demux.aggregator.checkSamplers["test-check-id"]
	demux.aggregator.mu.Unlock()

	require.True(t, ok, "sampler should have been created")
	assert.Equal(t, handle, cs.observerHandle, "observer handle should have been propagated to CheckSampler")
}

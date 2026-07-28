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
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
}

func (h *recordingHandle) ObserveMetric(v observer.MetricView) {
	// copy values — the MetricView contract forbids retaining the view itself
	tagsCopy := make([]string, len(v.GetRawTags()))
	copy(tagsCopy, v.GetRawTags())
	h.calls = append(h.calls, recordedCall{
		name:      v.GetName(),
		value:     v.GetValue(),
		tags:      tagsCopy,
		timestamp: v.GetTimestampUnix(),
	})
}

func (h *recordingHandle) ObserveLog(_ observer.LogView) {}

// recordingComponent wraps a recordingHandle as an observer.Component.
type recordingComponent struct {
	handle *recordingHandle
}

func (c *recordingComponent) GetHandle(_ string) observer.Handle {
	return c.handle
}

func (c *recordingComponent) RecordSamplerDropped(_, _ string) {}

func (c *recordingComponent) DumpMetrics(_ string) error {
	return nil
}

func (c *recordingComponent) SubscribeSeverityEvents(_ severityeventsdef.SeverityEventsConfiguration, _ severityeventsdef.SeverityEventListener) (severityeventsdef.SeverityEventsSubscription, error) {
	return severityeventsdef.SeverityEventsSubscription{Unsubscribe: func() {}}, nil
}

func (c *recordingComponent) SubscribeSeverityEventsReader(_ severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error) {
	return severityeventsdef.SeverityEventsReaderSubscription{Unsubscribe: func() {}}, nil
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
// when no active observer gate is enabled or anomaly_detection.metrics.enabled is false.
// Covers both the DogStatsD TimeSampler path and the BufferedAggregator/CheckSampler path.
func TestSetObserverConfigOff(t *testing.T) {
	opts := demuxTestOptions()
	deps := createDemultiplexerAgentTestDeps(t)
	// Use initAgentDemultiplexer (not started) — no goroutines, no Stop() needed.
	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")

	// Observer gates are off by default — handle must not be wired.
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

func TestSetObserverReportingEventsGateOn(t *testing.T) {
	opts := demuxTestOptions()
	deps := createDemultiplexerAgentTestDeps(t)
	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")

	cfg := pkgconfigsetup.Datadog()
	cfg.Set("anomaly_detection.reporting.events.enabled", true, model.SourceAgentRuntime)
	t.Cleanup(func() {
		cfg.Set("anomaly_detection.reporting.events.enabled", false, model.SourceAgentRuntime)
	})

	comp := &recordingComponent{handle: &recordingHandle{}}
	demux.SetObserver(comp)

	for _, w := range demux.statsd.workers {
		assert.Same(t, comp.handle, w.sampler.observerHandle, "DogStatsD worker handle should be wired when an observer gate is on")
	}
	assert.Same(t, comp.handle, demux.aggregator.observerHandle, "BufferedAggregator handle should be wired when an observer gate is on")

	demux.aggregator.handleRegisterSampler("check-reporting-events-on")
	demux.aggregator.mu.Lock()
	cs := demux.aggregator.checkSamplers["check-reporting-events-on"]
	demux.aggregator.mu.Unlock()
	require.NotNil(t, cs)
	assert.Same(t, comp.handle, cs.observerHandle, "CheckSampler handle should be wired when an observer gate is on")
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

// TestCheckSamplerObserverHandleNil verifies no panic when observerHandle is nil.
func TestCheckSamplerObserverHandleNil(t *testing.T) {
	store := tags.NewStore(false, "test")
	cs := newCheckSampler(10, false, false, 0, false, store, "test-check", nooptagger.NewComponent())
	// observerHandle is nil by default

	matcher := filterlist.NewNoopTagMatcher()
	s := metrics.MetricSample{Name: "m", Value: 1, Mtype: metrics.GaugeType, SampleRate: 1}
	assert.NotPanics(t, func() { cs.addSample(&s, matcher) })
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

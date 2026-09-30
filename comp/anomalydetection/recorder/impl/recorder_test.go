// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package recorderimpl

import (
	"testing"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/require"
)

type metricWriter struct {
	data   []recorder.MetricData
	events *[]string
}

func (w *metricWriter) WriteMetric(data recorder.MetricData) bool {
	*w.events = append(*w.events, "write metric")
	w.data = append(w.data, data)
	return true
}
func (*metricWriter) Close() error { return nil }

type logWriter struct {
	data   []recorder.LogData
	events *[]string
}

func (w *logWriter) WriteLog(data recorder.LogData) bool {
	*w.events = append(*w.events, "write log")
	w.data = append(w.data, data)
	return true
}
func (*logWriter) Close() error { return nil }

type testHandle struct {
	events  *[]string
	key     uint64
	metrics int
	logs    int
}

func (h *testHandle) ObserveMetric(_ observer.MetricView, key uint64) {
	*h.events = append(*h.events, "forward metric")
	h.metrics++
	h.key = key
}
func (h *testHandle) ObserveLog(_ observer.LogView) {
	*h.events = append(*h.events, "forward log")
	h.logs++
}

type reportingHandle struct {
	testHandle
	dropped bool
}

func (h *reportingHandle) ObserveMetricAndReportDrop(_ observer.MetricView, key uint64) bool {
	*h.events = append(*h.events, "forward metric")
	h.metrics++
	h.key = key
	return h.dropped
}

type testMetric struct {
	name, host string
	value      float64
	timestamp  int64
	tags       []string
	metricType metrics.MetricType
}

func (m *testMetric) GetName() string                   { return m.name }
func (m *testMetric) GetValue() float64                 { return m.value }
func (m *testMetric) GetTags() tagset.CompositeTags     { return tagset.CompositeTagsFromSlice(m.tags) }
func (m *testMetric) GetHost() string                   { return m.host }
func (m *testMetric) GetTimestampUnix() int64           { return m.timestamp }
func (m *testMetric) GetSampleRate() float64            { return 1 }
func (m *testMetric) GetMetricType() metrics.MetricType { return m.metricType }

type testLog struct {
	content, status, hostname string
	timestamp                 int64
	tags                      []string
}

func (l *testLog) GetContent() string           { return l.content }
func (l *testLog) GetStatus() string            { return l.status }
func (l *testLog) Tags() []string               { return l.tags }
func (l *testLog) GetHostname() string          { return l.hostname }
func (l *testLog) GetTimestampUnixMilli() int64 { return l.timestamp }

func TestRecorderForwardsOnceBeforeWriting(t *testing.T) {
	for _, reporting := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "drop-reporting"}[reporting], func(t *testing.T) {
			var events []string
			metrics := &metricWriter{events: &events}
			logs := &logWriter{events: &events}
			base := &testHandle{events: &events}
			var inner observer.Handle = base
			if reporting {
				inner = &reportingHandle{testHandle: *base, dropped: true}
			}
			var source string
			h := NewComponent(metrics, logs).GetHandle(func(name string) observer.Handle {
				source = name
				return inner
			})("check")
			metricTags := []string{"env:prod", "host:agent"}
			metric := &testMetric{name: "load", host: "agent", value: 2.5, timestamp: 123, tags: metricTags}
			h.ObserveMetric(metric, 42)
			logTags := []string{"team:core"}
			log := &testLog{content: "hello", status: "warn", hostname: "agent", timestamp: 456000, tags: logTags}
			h.ObserveLog(log)
			require.Equal(t, "check", source)
			require.Equal(t, []string{"forward metric", "write metric", "forward log", "write log"}, events)
			if reporting {
				require.Equal(t, 1, inner.(*reportingHandle).metrics)
				require.Equal(t, uint64(42), inner.(*reportingHandle).key)
			} else {
				require.Equal(t, 1, base.metrics)
				require.Equal(t, uint64(42), base.key)
			}
			require.Equal(t, recorder.MetricData{Source: "check", Name: "load", Value: 2.5, Timestamp: 123, Tags: []string{"env:prod", "host:agent"}, Dropped: reporting}, metrics.data[0])
			require.Equal(t, recorder.LogData{Source: "check", TimestampMs: 456000, Content: []byte("hello"), Status: "warn", Hostname: "agent", Tags: []string{"team:core"}}, logs.data[0])
			metricTags[0] = "changed"
			logTags[0] = "changed"
			log.content = "changed"
			require.Equal(t, "env:prod", metrics.data[0].Tags[0])
			require.Equal(t, "team:core", logs.data[0].Tags[0])
			require.Equal(t, []byte("hello"), logs.data[0].Content)
		})
	}
}

func TestRecorderTimestampFallbackAndHostTag(t *testing.T) {
	var events []string
	metrics := &metricWriter{events: &events}
	logs := &logWriter{events: &events}
	h := NewComponent(metrics, logs).GetHandle(func(string) observer.Handle { return &testHandle{events: &events} })("logs")
	before := time.Now()
	h.ObserveMetric(&testMetric{name: "count", host: "agent", tags: []string{"host:other"}}, 0)
	h.ObserveLog(&testLog{content: "message"})
	after := time.Now()
	require.Equal(t, []string{"host:other", "host:agent"}, metrics.data[0].Tags)
	require.GreaterOrEqual(t, metrics.data[0].Timestamp, before.Unix())
	require.LessOrEqual(t, metrics.data[0].Timestamp, after.Unix())
	require.GreaterOrEqual(t, logs.data[0].TimestampMs, before.UnixMilli())
	require.LessOrEqual(t, logs.data[0].TimestampMs, after.UnixMilli())
}

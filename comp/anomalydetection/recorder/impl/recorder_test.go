// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetection_recorder

package recorderimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

type recorderTestMetric struct {
	tags      []string
	host      string
	timestamp int64
}

func (m *recorderTestMetric) GetName() string   { return "requests.count" }
func (m *recorderTestMetric) GetValue() float64 { return 12.5 }
func (m *recorderTestMetric) GetTags() tagset.CompositeTags {
	return tagset.NewCompositeTags(m.tags[:1], m.tags[1:])
}
func (m *recorderTestMetric) GetHost() string         { return m.host }
func (m *recorderTestMetric) GetTimestampUnix() int64 { return m.timestamp }
func (m *recorderTestMetric) GetSampleRate() float64  { return 1 }

type recorderTestLog struct {
	content string
	tags    []string
	timeMs  int64
}

func (l *recorderTestLog) GetContent() string           { return l.content }
func (l *recorderTestLog) GetStatus() string            { return "warn" }
func (l *recorderTestLog) Tags() []string               { return l.tags }
func (l *recorderTestLog) GetHostname() string          { return "agent-a" }
func (l *recorderTestLog) GetTimestampUnixMilli() int64 { return l.timeMs }

type recorderTestInnerHandle struct {
	dropped     bool
	metricCalls int
	logCalls    int
	onMetric    func()
	onLog       func()
}

func (h *recorderTestInnerHandle) ObserveMetric(m observer.MetricView) {
	h.ObserveMetricAndReportDrop(m)
}
func (h *recorderTestInnerHandle) ObserveMetricAndReportDrop(observer.MetricView) bool {
	h.metricCalls++
	if h.onMetric != nil {
		h.onMetric()
	}
	return h.dropped
}
func (h *recorderTestInnerHandle) ObserveLog(observer.LogView) {
	h.logCalls++
	if h.onLog != nil {
		h.onLog()
	}
}

func TestRecorderMiddlewareRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"anomaly_detection.recording.enabled":        true,
		"anomaly_detection.recording.output_dir":     dir,
		"anomaly_detection.recording.flush_interval": 30,
		"anomaly_detection.recording.retention":      "2h",
	})
	provided, err := NewComponent(Requires{Config: cfg})
	require.NoError(t, err)
	r := provided.Comp.(*recorderImpl)
	require.Equal(t, 30*time.Second, r.metricParquetWriter.flushInterval)
	require.Equal(t, 2*time.Hour, r.metricParquetWriter.retentionDuration)

	inner := &recorderTestInnerHandle{dropped: true}
	inner.onMetric = func() { require.Len(t, r.metricParquetWriter.typedBuilder.metricNames, inner.metricCalls-1) }
	inner.onLog = func() { require.Len(t, r.logParquetWriter.typedBuilder.contents, inner.logCalls-1) }
	handle := r.GetHandle(func(name string) observer.Handle {
		require.Equal(t, "check", name)
		return inner
	})("check")

	metric := &recorderTestMetric{tags: []string{"env:prod", "service:api"}, host: "web-1", timestamp: 1234}
	handle.ObserveMetric(metric)
	metric.tags[0] = "env:changed"
	inner.dropped = false
	metric.host = ""
	metric.timestamp = 0
	handle.ObserveMetric(metric)

	log := &recorderTestLog{content: "warning", tags: []string{"service:api"}, timeMs: 1234567}
	handle.ObserveLog(log)
	log.tags[0] = "service:changed"
	log.content = "changed"

	require.Equal(t, 2, inner.metricCalls)
	require.Equal(t, 1, inner.logCalls)
	require.NoError(t, r.metricParquetWriter.Close())
	require.NoError(t, r.logParquetWriter.Close())

	metrics, err := r.ReadAllMetrics(dir)
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	require.Equal(t, "check", metrics[0].Source)
	require.Equal(t, "requests.count", metrics[0].Name)
	require.Equal(t, 12.5, metrics[0].Value)
	require.Equal(t, int64(1234), metrics[0].Timestamp)
	require.True(t, metrics[0].Dropped)
	require.ElementsMatch(t, []string{"env:prod", "service:api", "host:web-1"}, metrics[0].Tags)
	require.False(t, metrics[1].Dropped)
	require.WithinDuration(t, time.Now(), time.Unix(metrics[1].Timestamp, 0), 2*time.Second)

	logs, err := r.ReadAllLogs(dir)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, "check", logs[0].Source)
	require.Equal(t, int64(1234567), logs[0].TimestampMs)
	require.Equal(t, []byte("warning"), logs[0].Content)
	require.Equal(t, []string{"service:api"}, logs[0].Tags)
	require.Equal(t, "warn", logs[0].Status)
	require.Equal(t, "agent-a", logs[0].Hostname)
}

func TestRecorderDoesNotDuplicateHostTag(t *testing.T) {
	dir := t.TempDir()
	writer, err := newMetricParquetWriter(dir, time.Hour, 0)
	require.NoError(t, err)
	r := &recorderImpl{metricParquetWriter: writer}
	h := r.GetHandle(func(string) observer.Handle { return &recorderTestInnerHandle{} })("check")
	h.ObserveMetric(&recorderTestMetric{tags: []string{"host:web-1", "env:prod"}, host: "web-1", timestamp: 42})
	require.Equal(t, []string{"host:web-1", "env:prod"}, writer.typedBuilder.tags[0])
	require.NoError(t, writer.Close())
	rows, err := r.ReadAllMetrics(dir)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	hostTags := 0
	for _, tag := range rows[0].Tags {
		if tag == "host:web-1" {
			hostTags++
		}
	}
	require.Equal(t, 1, hostTags)
}

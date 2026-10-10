// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchQuery(t *testing.T) {
	l := &Log{
		Message: "Connection timeout to db", Status: "error", Service: "web", Host: "laptop",
		Tags:       []string{"env:prod", "team:core"},
		Attributes: map[string]any{"http": map[string]any{"status_code": float64(503), "method": "GET"}, "usr.id": "42"},
	}
	cases := map[string]bool{
		"":                                      true,
		"*":                                     true,
		"service:web":                           true,
		"service:api":                           false,
		"-service:api":                          true,
		"status:error env:prod":                 true,
		"status:error env:dev":                  false,
		"service:web OR service:api":            true,
		"(service:api OR status:error) timeout": true,
		`"connection timeout"`:                  true,
		"timeout":                               true,
		"time*":                                 true,
		"@http.status_code:503":                 true,
		"@http.status_code:>=500":               true,
		"@http.status_code:<500":                false,
		"@http.status_code:[500 TO 599]":        true,
		"@http.method:get":                      true,
		"@usr.id:42":                            true,
		"service:(api OR web)":                  true,
		"NOT status:error":                      false,
		"host:lap*":                             true,
		"@missing:*":                            false,
		"@http.status_code:*":                   true,
	}
	for q, want := range cases {
		assert.Equal(t, want, ParseQuery(q).Match(l), "query %q", q)
	}
}

func TestSpanFields(t *testing.T) {
	sp := &Span{Service: "web", Resource: "GET /users", Name: "express.request", Error: 1, Duration: 2_000_000,
		Meta: map[string]string{"http.status_code": "500"}, Metrics: map[string]float64{"_sampling_priority_v1": 1}, IsTopLevel: true}
	for q, want := range map[string]bool{
		`resource_name:"GET /users"`:     true,
		"status:error":                   true,
		"@duration:>1000000":             true,
		"@http.status_code:500":          true,
		"operation_name:express.request": true,
		"@_top_level:1":                  true,
	} {
		assert.Equal(t, want, ParseQuery(q).Match(sp), "query %q", q)
	}
}

func TestMetricQuery(t *testing.T) {
	mq, err := ParseMetricQuery("max:system.cpu.user{env:prod,!host:a} by {host,device}.rollup(avg, 60).as_count()")
	require.NoError(t, err)
	assert.Equal(t, "max", mq.SpaceAgg)
	assert.Equal(t, "system.cpu.user", mq.Metric)
	assert.Equal(t, []string{"env:prod", "!host:a"}, mq.Filters)
	assert.Equal(t, []string{"host", "device"}, mq.GroupBy)
	assert.Equal(t, "avg", mq.RollupFn)
	assert.Equal(t, int64(60), mq.RollupInterval)
	assert.True(t, mq.AsCount)

	st := New(DefaultOptions())
	base := time.Now().Unix() / 60 * 60
	st.AddPoints("cpu", "gauge", "a", "", 0, []string{"env:prod"}, []Point{{base, 10}, {base + 10, 20}})
	st.AddPoints("cpu", "gauge", "b", "", 0, []string{"env:prod"}, []Point{{base, 30}})
	st.AddPoints("cpu", "gauge", "c", "", 0, []string{"env:dev"}, []Point{{base, 100}})

	q, _ := ParseMetricQuery("avg:cpu{env:prod}")
	res := st.QueryMetrics(q, base*1000, base*1000+59_000, 60_000)
	require.Len(t, res.Series, 1)
	// time rollup per series (a: avg(10,20)=15, b: 30), then space avg: 22.5
	assert.Equal(t, 22.5, *res.Series[0].Values[0])

	q, _ = ParseMetricQuery("sum:cpu{!host:a} by {host}")
	res = st.QueryMetrics(q, base*1000, base*1000+59_000, 60_000)
	require.Len(t, res.Series, 2)
	assert.Equal(t, []string{"host:b"}, res.Series[0].GroupTags)
}

func TestRetentionAndSnapshot(t *testing.T) {
	st := New(Options{MaxLogs: 2, MaxSpans: 2, MaxPointsPerSeries: 2, Retention: time.Hour})
	now := time.Now().UnixMilli()
	st.AddLogs([]*Log{{Timestamp: now, Message: "1"}, {Timestamp: now, Message: "2"}, {Timestamp: now, Message: "3"}})
	st.AddSpans([]*Span{{TraceID: "1", SpanID: "1"}, {TraceID: "1", SpanID: "2"}, {TraceID: "2", SpanID: "3"}})
	st.AddPoints("m", "gauge", "h", "", 0, nil, []Point{{1, 1}, {2, 2}, {3, 3}})
	stats := st.Stats()
	assert.Equal(t, 2, stats.Logs)
	assert.Equal(t, 2, stats.Spans)
	assert.Len(t, st.Trace("1"), 1, "evicted spans leave the trace index")
	assert.Equal(t, 2, stats.Points)

	path := filepath.Join(t.TempDir(), "snap.gob.gz")
	require.NoError(t, st.SaveSnapshot(path))
	restored := New(DefaultOptions())
	require.NoError(t, restored.LoadSnapshot(path))
	assert.Equal(t, stats.Logs, restored.Stats().Logs)
	assert.Equal(t, stats.Spans, restored.Stats().Spans)
	assert.Equal(t, stats.MetricSeries, restored.Stats().MetricSeries)
	assert.NoError(t, New(DefaultOptions()).LoadSnapshot(filepath.Join(t.TempDir(), "missing")))
}

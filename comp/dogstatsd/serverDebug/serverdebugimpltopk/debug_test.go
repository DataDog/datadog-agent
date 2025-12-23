// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package serverdebugimpltopk

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func fulfillDeps(t testing.TB, overrides map[string]interface{}) serverdebug.Component {
	return fxutil.Test[serverdebug.Component](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockWithOverrides(t, overrides)
		}),
		Module(),
	))
}

func TestDebugStatsSpike(t *testing.T) {
	t.Skip("flaky")
	cfg := make(map[string]interface{})
	cfg["dogstatsd_logging_enabled"] = false
	debug := fulfillDeps(t, cfg)
	d := debug.(*serverDebugImpl)

	clk := clock.NewMock()
	d.clock = clk

	d.SetMetricStatsEnabled(true)
	sample := metrics.MetricSample{Name: "some.metric1", Tags: make([]string, 0)}

	send := func(count int) {
		for i := 0; i < count; i++ {
			d.StoreMetricStats(sample)
		}
	}

	send(10)

	clk.Add(1050 * time.Millisecond)
	send(10)

	clk.Add(1050 * time.Millisecond)
	send(10)

	clk.Add(1050 * time.Millisecond)
	send(10)

	clk.Add(1050 * time.Millisecond)
	send(500)

	// stop the debug loop to avoid data race
	d.SetMetricStatsEnabled(false)
	time.Sleep(500 * time.Millisecond)

	assert.True(t, d.hasSpike())

	d.SetMetricStatsEnabled(true)
	// This sleep is necessary as we need to wait for the goroutine function within 'EnableMetricsStats' to start.
	// If we remove the sleep, the debug loop ticker will not be triggered by the clk.Add() call and the 500 samples
	// added with 'send(500)' will be considered as if they had been added in the same second as the previous 500 samples.
	// This will lead to a spike because we have 1000 samples in 1 second instead of having 500 and 500 in 2 different seconds.
	time.Sleep(1050 * time.Millisecond)

	clk.Add(1050 * time.Millisecond)
	send(500)

	// stop the debug loop to avoid data race
	d.SetMetricStatsEnabled(false)
	time.Sleep(500 * time.Millisecond)

	// it is no more considered a spike because we had another second with 500 metrics
	assert.False(t, d.hasSpike())

}

func TestDebugStats(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_logging_enabled"] = false
	debug := fulfillDeps(t, cfg)
	d := debug.(*serverDebugImpl)

	clk := clock.NewMock()
	d.clock = clk

	d.SetMetricStatsEnabled(true)

	// data
	sample1 := metrics.MetricSample{Name: "some.metric1", Tags: make([]string, 0)}
	sample2 := metrics.MetricSample{Name: "some.metric2", Tags: []string{"a"}}
	sample3 := metrics.MetricSample{Name: "some.metric3", Tags: make([]string, 0)}
	sample4 := metrics.MetricSample{Name: "some.metric4", Tags: []string{"b", "c"}}
	sample5 := metrics.MetricSample{Name: "some.metric4", Tags: []string{"c", "b"}}
	hash1, _ := MakeKey(sample1)
	hash2, _ := MakeKey(sample2)
	hash3, _ := MakeKey(sample3)
	hash4, _ := MakeKey(sample4)
	hash5, _ := MakeKey(sample5)

	hash1Key := ckey.ContextKey(hash1)
	hash2Key := ckey.ContextKey(hash2)
	hash3Key := ckey.ContextKey(hash3)
	hash4Key := ckey.ContextKey(hash4)
	hash5Key := ckey.ContextKey(hash5)
	// test ingestion and ingestion time
	d.StoreMetricStats(sample1)
	d.StoreMetricStats(sample2)
	clk.Add(10 * time.Millisecond)
	d.StoreMetricStats(sample1)

	data, err := d.GetJSONDebugStats()
	require.NoError(t, err, "cannot get debug stats")
	require.NotNil(t, data)
	require.NotEmpty(t, data)

	var stats map[ckey.ContextKey]metricStat
	err = json.Unmarshal(data, &stats)
	require.NoError(t, err, "data is not valid")
	require.Len(t, stats, 2, "two metrics should have been captured")

	require.True(t, stats[hash1Key].LastSeen.After(stats[hash2Key].LastSeen), "some.metric1 should have appeared again after some.metric2")

	d.StoreMetricStats(sample3)
	clk.Add(10 * time.Millisecond)
	d.StoreMetricStats(sample1)

	d.StoreMetricStats(sample4)
	d.StoreMetricStats(sample5)
	data, _ = d.GetJSONDebugStats()
	err = json.Unmarshal(data, &stats)
	require.NoError(t, err, "data is not valid")
	require.Len(t, stats, 4, "4 metrics should have been captured")

	// test stats array
	metric1 := stats[hash1Key]
	metric2 := stats[hash2Key]
	metric3 := stats[hash3Key]
	metric4 := stats[hash4Key]
	metric5 := stats[hash5Key]

	require.True(t, metric1.LastSeen.After(metric2.LastSeen), "some.metric1 should have appeared again after some.metric2")
	require.True(t, metric1.LastSeen.After(metric3.LastSeen), "some.metric1 should have appeared again after some.metric3")
	require.True(t, metric3.LastSeen.After(metric2.LastSeen), "some.metric3 should have appeared again after some.metric2")

	require.Equal(t, metric1.Count, uint64(3))
	require.Equal(t, metric2.Count, uint64(1))
	require.Equal(t, metric3.Count, uint64(1))

	// test context correctness
	require.Equal(t, metric4.Tags, "b c")
	require.Equal(t, metric5.Tags, "b c")
	require.Equal(t, hash4, hash5)
}

func TestFormatDebugStats(t *testing.T) {
	// Create test data
	stats := map[uint64]metricStat{
		123: {
			Name:     "test.metric1",
			Count:    10,
			LastSeen: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Tags:     "env:prod service:api",
		},
		456: {
			Name:     "test.metric2",
			Count:    15,
			LastSeen: time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC),
			Tags:     "env:dev",
		},
	}

	// Convert test data to JSON (expected input format)
	statsJSON, err := json.Marshal(stats)
	require.NoError(t, err)

	// Call FormatDebugStats
	result, err := FormatDebugStats(statsJSON)
	require.NoError(t, err)

	// Expected formatted table
	expectedResult := `Metric                                   | Tags                 | Count      | Last Seen
-----------------------------------------|----------------------|------------|---------------------
test.metric2                             | env:dev              | 15         | 2025-01-01 11:00:00 +0000 UTC
test.metric1                             | env:prod service:api | 10         | 2025-01-01 12:00:00 +0000 UTC
`

	// Verify the entire formatted result
	assert.Equal(t, expectedResult, result)

	// Verify sorting (metrics should be sorted by count in descending order)
	assert.True(t, strings.Index(result, "test.metric2") < strings.Index(result, "test.metric1"))

	// Test empty stats
	emptyStats := map[uint64]metricStat{}
	emptyStatsJSON, err := json.Marshal(emptyStats)
	require.NoError(t, err)

	emptyResult, err := FormatDebugStats(emptyStatsJSON)
	require.NoError(t, err)
	assert.Contains(t, emptyResult, "No metrics processed yet.")

	// Test invalid JSON
	_, err = FormatDebugStats([]byte("invalid json"))
	assert.Error(t, err)
}

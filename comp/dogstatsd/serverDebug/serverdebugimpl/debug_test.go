// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package serverdebugimpl

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func fulfillDeps(t testing.TB, overrides map[string]interface{}) serverdebug.Component {
	return fxutil.Test[serverdebug.Component](t, fx.Options(
		core.MockBundle(),
		fx.Supply(core.BundleParams{}),
		fx.Replace(configComponent.MockParams{Overrides: overrides}),
		Module(),
	))
}

func TestDebugStatsSpike(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_logging_enabled"] = false
	debug := fulfillDeps(t, cfg)
	d := debug.(*serverDebugImpl)

	assert := assert.New(t)

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

	assert.True(d.hasSpike())

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
	assert.False(d.hasSpike())

}

func TestDebugStats(t *testing.T) {
	cfg := make(map[string]interface{})
	cfg["dogstatsd_logging_enabled"] = false
	debug := fulfillDeps(t, cfg)
	d := debug.(*serverDebugImpl)

	clk := clock.NewMock()
	d.clock = clk

	d.SetMetricStatsEnabled(true)

	keygen := ckey.NewKeyGenerator()

	// data
	sample1 := metrics.MetricSample{Name: "some.metric1", Tags: make([]string, 0)}
	sample2 := metrics.MetricSample{Name: "some.metric2", Tags: []string{"a"}}
	sample3 := metrics.MetricSample{Name: "some.metric3", Tags: make([]string, 0)}
	sample4 := metrics.MetricSample{Name: "some.metric4", Tags: []string{"b", "c"}}
	sample5 := metrics.MetricSample{Name: "some.metric4", Tags: []string{"c", "b"}}
	hash1 := keygen.Generate(sample1.Name, "", tagset.NewHashingTagsAccumulatorWithTags(sample1.Tags))
	hash2 := keygen.Generate(sample2.Name, "", tagset.NewHashingTagsAccumulatorWithTags(sample2.Tags))
	hash3 := keygen.Generate(sample3.Name, "", tagset.NewHashingTagsAccumulatorWithTags(sample3.Tags))
	hash4 := keygen.Generate(sample4.Name, "", tagset.NewHashingTagsAccumulatorWithTags(sample4.Tags))
	hash5 := keygen.Generate(sample5.Name, "", tagset.NewHashingTagsAccumulatorWithTags(sample5.Tags))

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

	require.True(t, stats[hash1].LastSeen.After(stats[hash2].LastSeen), "some.metric1 should have appeared again after some.metric2")

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
	metric1 := stats[hash1]
	metric2 := stats[hash2]
	metric3 := stats[hash3]
	metric4 := stats[hash4]
	metric5 := stats[hash5]

	require.True(t, metric1.LastSeen.After(metric2.LastSeen), "some.metric1 should have appeared again after some.metric2")
	require.True(t, metric1.LastSeen.After(metric3.LastSeen), "some.metric1 should have appeared again after some.metric3")
	require.True(t, metric3.LastSeen.After(metric2.LastSeen), "some.metric3 should have appeared again after some.metric2")

	require.Equal(t, metric1.Count, uint64(3))
	require.Equal(t, metric2.Count, uint64(1))
	require.Equal(t, metric3.Count, uint64(1))

	// test context correctness
	require.Equal(t, metric4.Tags, "c b")
	require.Equal(t, metric5.Tags, "c b")
	require.Equal(t, hash4, hash5)

}

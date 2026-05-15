// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package serverdebugimpl

import (
	"encoding/json"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/internal/identity"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestMilestone2StoreMetricStatsWithDebugViewKeyMatchesLegacyKey(t *testing.T) {
	debug := fulfillDeps(t, map[string]interface{}{"dogstatsd_logging_enabled": false})
	d := debug.(*serverDebugImpl)
	d.SetMetricStatsEnabled(true)
	defer func() {
		d.SetMetricStatsEnabled(false)
		time.Sleep(50 * time.Millisecond)
	}()

	sample := metrics.MetricSample{
		Name: "identity.metric",
		Host: "host-ignored-by-debug",
		Tags: []string{"env:prod", "service:web", "env:prod"},
	}
	context := identity.NewBuilder().ResolveHotPath(sample)

	d.StoreMetricStatsWithDebugViewKey(sample, context.DebugView)
	d.StoreMetricStats(sample)

	payload, err := d.GetJSONDebugStats()
	require.NoError(t, err)
	var stats map[ckey.ContextKey]metricStat
	require.NoError(t, json.Unmarshal(payload, &stats))
	require.Len(t, stats, 1, "precomputed and legacy debug view keys must hit the same stats row")

	stat := stats[context.DebugView.Key]
	assert.Equal(t, uint64(2), stat.Count)
	assert.Equal(t, context.DebugView.Client.Name, stat.Name)
	assert.Equal(t, context.DebugView.DisplayTags, stat.Tags)
}

func BenchmarkMilestone2StoreMetricStatsWithDebugViewKey(b *testing.B) {
	samples := make([]metrics.MetricSample, 8192)
	contexts := make([]identity.HotPathContext, len(samples))
	builder := identity.NewBuilder()
	for i := range samples {
		samples[i] = metrics.MetricSample{
			Name: "identity.metric",
			Tags: []string{"env:prod", "service:web", "instance:" + strconv.Itoa(i)},
		}
		contexts[i] = builder.ResolveHotPath(samples[i])
	}

	b.Run("legacy_compute_in_debug", func(b *testing.B) {
		debug := fulfillDeps(b, map[string]interface{}{"dogstatsd_logging_enabled": false})
		d := debug.(*serverDebugImpl)
		d.SetMetricStatsEnabled(true)
		defer d.SetMetricStatsEnabled(false)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			d.StoreMetricStats(samples[i%len(samples)])
		}
	})

	b.Run("precomputed_debug_view_key", func(b *testing.B) {
		debug := fulfillDeps(b, map[string]interface{}{"dogstatsd_logging_enabled": false})
		d := debug.(*serverDebugImpl)
		d.SetMetricStatsEnabled(true)
		defer d.SetMetricStatsEnabled(false)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			idx := i % len(samples)
			d.StoreMetricStatsWithDebugViewKey(samples[idx], contexts[idx].DebugView)
		}
	})
}

func TestMilestone2StoreMetricStatsWithDebugViewKeyConcurrent(t *testing.T) {
	debug := fulfillDeps(t, map[string]interface{}{"dogstatsd_logging_enabled": false})
	d := debug.(*serverDebugImpl)
	d.SetMetricStatsEnabled(true)
	defer func() {
		d.SetMetricStatsEnabled(false)
		time.Sleep(50 * time.Millisecond)
	}()

	sample := metrics.MetricSample{Name: "identity.metric", Tags: []string{"env:prod", "service:web"}}
	context := identity.NewBuilder().ResolveHotPath(sample)

	const goroutines = 8
	const samplesPerGoroutine = 128
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < samplesPerGoroutine; j++ {
				d.StoreMetricStatsWithDebugViewKey(sample, context.DebugView)
			}
		}()
	}
	wg.Wait()

	payload, err := d.GetJSONDebugStats()
	require.NoError(t, err)
	var stats map[ckey.ContextKey]metricStat
	require.NoError(t, json.Unmarshal(payload, &stats))
	require.Len(t, stats, 1)
	assert.Equal(t, uint64(goroutines*samplesPerGoroutine), stats[context.DebugView.Key].Count)
}

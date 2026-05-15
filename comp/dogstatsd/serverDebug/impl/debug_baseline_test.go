// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build test

package serverdebugimpl

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

func TestMilestone0DebugViewKeyBaseline(t *testing.T) {
	debug := fulfillDeps(t, map[string]interface{}{"dogstatsd_logging_enabled": false})
	d := debug.(*serverDebugImpl)
	d.SetMetricStatsEnabled(true)
	defer func() {
		d.SetMetricStatsEnabled(false)
		time.Sleep(50 * time.Millisecond)
	}()

	base := metrics.MetricSample{
		Name:       "identity.metric",
		Host:       "host-a",
		Tags:       []string{"env:prod", "service:web", "env:prod"},
		Mtype:      metrics.GaugeType,
		OriginInfo: taggertypes.OriginInfo{ContainerIDFromSocket: "container-a", Cardinality: "low"},
		ListenerID: "udp-127.0.0.1:8125",
	}
	reordered := base
	reordered.Tags = []string{"service:web", "env:prod"}
	differentHostOriginAndType := base
	differentHostOriginAndType.Host = "host-b"
	differentHostOriginAndType.Mtype = metrics.CounterType
	differentHostOriginAndType.OriginInfo = taggertypes.OriginInfo{ContainerIDFromSocket: "container-b", Cardinality: "high"}
	differentHostOriginAndType.ListenerID = "uds-unixgram-7"
	differentTags := base
	differentTags.Tags = []string{"env:prod", "service:api"}

	d.StoreMetricStats(base)
	d.StoreMetricStats(reordered)
	d.StoreMetricStats(differentHostOriginAndType)
	d.StoreMetricStats(differentTags)

	payload, err := d.GetJSONDebugStats()
	require.NoError(t, err)
	var stats map[ckey.ContextKey]metricStat
	require.NoError(t, json.Unmarshal(payload, &stats))
	require.Len(t, stats, 2, "debug view currently groups by metric name and client tags only")

	var counts []uint64
	for _, stat := range stats {
		counts = append(counts, stat.Count)
	}
	assert.ElementsMatch(t, []uint64{3, 1}, counts, "host/origin/type changes collapse, tag changes do not")
}

func BenchmarkMilestone0StoreMetricStats(b *testing.B) {
	b.Run("disabled", func(b *testing.B) {
		debug := fulfillDeps(b, map[string]interface{}{"dogstatsd_logging_enabled": false})
		d := debug.(*serverDebugImpl)
		sample := metrics.MetricSample{Name: "identity.metric", Tags: []string{"env:prod", "service:web"}}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			d.StoreMetricStats(sample)
		}
	})

	b.Run("enabled_single_series", func(b *testing.B) {
		debug := fulfillDeps(b, map[string]interface{}{"dogstatsd_logging_enabled": false})
		d := debug.(*serverDebugImpl)
		d.SetMetricStatsEnabled(true)
		defer d.SetMetricStatsEnabled(false)
		sample := metrics.MetricSample{Name: "identity.metric", Tags: []string{"env:prod", "service:web"}}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			d.StoreMetricStats(sample)
		}
	})

	b.Run("enabled_high_cardinality", func(b *testing.B) {
		debug := fulfillDeps(b, map[string]interface{}{"dogstatsd_logging_enabled": false})
		d := debug.(*serverDebugImpl)
		d.SetMetricStatsEnabled(true)
		defer d.SetMetricStatsEnabled(false)
		samples := make([]metrics.MetricSample, 8192)
		for i := range samples {
			samples[i] = metrics.MetricSample{
				Name: "identity.metric",
				Tags: []string{"env:prod", "service:web", "instance:" + strconv.Itoa(i)},
			}
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			d.StoreMetricStats(samples[i%len(samples)])
		}
	})
}

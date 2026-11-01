// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/stretchr/testify/assert"
)

func TestCheckMetrics(t *testing.T) {
	cm := NewCheckMetrics(true, 1000*time.Second)
	t0 := 16_0000_0000.0

	cfg := setupConfig(t)
	cm.AddSample(1, &MetricSample{Mtype: GaugeType}, t0, 1, cfg)
	assert.Contains(t, cm.metrics, ckey.ContextKey(1))

	cm.AddSample(2, &MetricSample{Mtype: MonotonicCountType}, t0, 1, cfg)
	assert.Contains(t, cm.metrics, ckey.ContextKey(2))

	cm.AddSample(3, &MetricSample{Mtype: MonotonicCountType}, t0, 1, cfg)
	assert.Contains(t, cm.metrics, ckey.ContextKey(3))

	cm.AddSample(4, &MetricSample{Mtype: GaugeType}, t0, 1, cfg)
	assert.Contains(t, cm.metrics, ckey.ContextKey(4))

	cm.Expire([]ckey.ContextKey{1, 2}, t0+100)
	assert.NotContains(t, cm.metrics, ckey.ContextKey(1))
	assert.Contains(t, cm.metrics, ckey.ContextKey(2))
	assert.Contains(t, cm.metrics, ckey.ContextKey(3))
	assert.Contains(t, cm.metrics, ckey.ContextKey(4))

	cm.RemoveExpired(t0 + 100)
	assert.Contains(t, cm.metrics, ckey.ContextKey(2))
	assert.Contains(t, cm.metrics, ckey.ContextKey(3))
	assert.Contains(t, cm.metrics, ckey.ContextKey(4))

	cm.RemoveExpired(t0 + 1100)
	assert.Contains(t, cm.metrics, ckey.ContextKey(2))
	assert.Contains(t, cm.metrics, ckey.ContextKey(3))
	assert.Contains(t, cm.metrics, ckey.ContextKey(4))

	cm.RemoveExpired(t0 + 1101)
	assert.NotContains(t, cm.metrics, ckey.ContextKey(2))
	assert.Contains(t, cm.metrics, ckey.ContextKey(3))
	assert.Contains(t, cm.metrics, ckey.ContextKey(4))
}

func TestCheckMetricsNoExpiry(t *testing.T) {
	cm := NewCheckMetrics(false, 1000*time.Second)
	t0 := 16_0000_0000.0

	cfg := setupConfig(t)
	cm.AddSample(1, &MetricSample{Mtype: GaugeType}, t0, 1, cfg)
	assert.Contains(t, cm.metrics, ckey.ContextKey(1))

	cm.AddSample(2, &MetricSample{Mtype: MonotonicCountType}, t0, 1, cfg)
	assert.Contains(t, cm.metrics, ckey.ContextKey(2))

	cm.AddSample(3, &MetricSample{Mtype: MonotonicCountType}, t0, 1, cfg)
	assert.Contains(t, cm.metrics, ckey.ContextKey(3))

	cm.AddSample(4, &MetricSample{Mtype: GaugeType}, t0, 1, cfg)
	assert.Contains(t, cm.metrics, ckey.ContextKey(4))

	cm.Expire([]ckey.ContextKey{1, 2}, t0+100)
	assert.Contains(t, cm.metrics, ckey.ContextKey(1))
	assert.Contains(t, cm.metrics, ckey.ContextKey(2))
	assert.Contains(t, cm.metrics, ckey.ContextKey(3))
	assert.Contains(t, cm.metrics, ckey.ContextKey(4))

	cm.RemoveExpired(t0 + 100)
	assert.Contains(t, cm.metrics, ckey.ContextKey(2))
	assert.Contains(t, cm.metrics, ckey.ContextKey(3))
	assert.Contains(t, cm.metrics, ckey.ContextKey(4))

	cm.RemoveExpired(t0 + 1100)
	assert.Contains(t, cm.metrics, ckey.ContextKey(2))
	assert.Contains(t, cm.metrics, ckey.ContextKey(3))
	assert.Contains(t, cm.metrics, ckey.ContextKey(4))

	cm.RemoveExpired(t0 + 1101)
	assert.Contains(t, cm.metrics, ckey.ContextKey(2))
	assert.Contains(t, cm.metrics, ckey.ContextKey(3))
	assert.Contains(t, cm.metrics, ckey.ContextKey(4))
}

func seriesKeys(series []*Serie) []ckey.ContextKey {
	keys := map[ckey.ContextKey]struct{}{}
	for _, s := range series {
		keys[s.ContextKey] = struct{}{}
	}
	result := make([]ckey.ContextKey, 0, len(keys))
	for k := range keys {
		result = append(result, k)
	}
	return result
}

func TestCheckMetricsFlushPeriodic(t *testing.T) {
	cm := NewCheckMetrics(true, 1000*time.Second)
	t0 := 16_0000_0000.0
	cfg := setupConfig(t)

	periodic := map[ckey.ContextKey]MetricType{
		1: GaugeType,
		2: CountType,
		3: CounterType,
		4: MonotonicCountType,
		5: GaugeWithTimestampType,
		6: CountWithTimestampType,
	}
	runScoped := map[ckey.ContextKey]MetricType{
		7:  RateType,
		8:  HistogramType,
		9:  HistorateType,
		10: SetType,
	}
	for _, types := range []map[ckey.ContextKey]MetricType{periodic, runScoped} {
		for key, mtype := range types {
			cm.AddSample(key, &MetricSample{Mtype: mtype, Value: 1, RawValue: "a"}, t0, 1, cfg)
			cm.AddSample(key, &MetricSample{Mtype: mtype, Value: 2, RawValue: "b"}, t0+1, 1, cfg)
		}
	}

	series, errors, keys := cm.FlushPeriodic(t0 + 5)
	assert.Empty(t, errors)
	assert.ElementsMatch(t, []ckey.ContextKey{1, 2, 3, 4, 5, 6}, keys)
	assert.ElementsMatch(t, []ckey.ContextKey{1, 2, 3, 4, 5, 6}, seriesKeys(series))

	series, errors = cm.Flush(t0 + 10)
	assert.Empty(t, errors)
	assert.ElementsMatch(t, []ckey.ContextKey{7, 8, 9, 10}, seriesKeys(series))
}

func TestCheckMetricsFlushPeriodicMonotonicCount(t *testing.T) {
	cm := NewCheckMetrics(true, 1000*time.Second)
	t0 := 16_0000_0000.0
	cfg := setupConfig(t)

	cm.AddSample(1, &MetricSample{Mtype: MonotonicCountType, Value: 10}, t0, 1, cfg)
	cm.AddSample(1, &MetricSample{Mtype: MonotonicCountType, Value: 12}, t0+1, 1, cfg)
	series, _, _ := cm.FlushPeriodic(t0 + 2)
	if assert.Len(t, series, 1) {
		assert.Equal(t, 2.0, series[0].Points[0].Value)
	}

	cm.AddSample(1, &MetricSample{Mtype: MonotonicCountType, Value: 15}, t0+3, 1, cfg)
	series, _, _ = cm.FlushPeriodic(t0 + 4)
	if assert.Len(t, series, 1) {
		assert.Equal(t, 3.0, series[0].Points[0].Value)
		assert.Equal(t, t0+4, series[0].Points[0].Ts)
	}

	series, _ = cm.Flush(t0 + 5)
	assert.Empty(t, series)
}

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

func TestPeriodicSetKeepsRunUniqueness(t *testing.T) {
	cm := NewCheckMetrics(true, time.Hour)
	cfg := setupConfig(t)
	add := func(v string) {
		assert.NoError(t, cm.AddSample(1, &MetricSample{Mtype: SetType, RawValue: v}, 1, 1, cfg))
	}
	add("a")
	add("b")
	series, errs := cm.FlushIntermediate(100)
	assert.Empty(t, series)
	assert.Empty(t, errs)
	add("b")
	add("c")
	series, errs = cm.FlushRunScoped(101)
	assert.Empty(t, errs)
	if assert.Len(t, series, 1) {
		assert.Equal(t, float64(3), series[0].Points[0].Value)
	}
	add("a")
	series, _ = cm.FlushRunScoped(102)
	if assert.Len(t, series, 1) {
		assert.Equal(t, float64(1), series[0].Points[0].Value)
	}
}

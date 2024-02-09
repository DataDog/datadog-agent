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

	cfg := setupConfig()
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

	cfg := setupConfig()
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

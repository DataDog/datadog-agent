// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func makeSerie(name, host string, value float64) *metrics.Serie {
	return &metrics.Serie{
		Name:   name,
		Host:   host,
		MType:  metrics.APIRateType,
		Points: []metrics.Point{{Value: value}},
	}
}

func observeDropRatio(d *droppedMetricsDetector, host string, ratio float64) {
	d.observe(makeSerie(dogstatsdClientBytesSent, host, 1-ratio))
	d.observe(makeSerie(dogstatsdClientBytesDropped, host, ratio))
}

func aboveDroppedBytesWarnThreshold() float64 {
	return droppedBytesWarnThreshold + (1-droppedBytesWarnThreshold)/2
}

func TestDroppedMetricsDetector_Violations(t *testing.T) {
	tests := []struct {
		name      string
		ratio     float64
		violation bool
	}{
		{name: "below threshold", ratio: droppedBytesWarnThreshold / 2},
		{name: "at threshold", ratio: droppedBytesWarnThreshold},
		{name: "above threshold", ratio: aboveDroppedBytesWarnThreshold(), violation: true},
		{name: "all dropped", ratio: 1, violation: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDroppedMetricsDetector()
			observeDropRatio(d, "host-a", tt.ratio)

			violations := d.violations()
			if !tt.violation {
				assert.Empty(t, violations)
				return
			}
			require.Contains(t, violations, "host-a")
			assert.InDelta(t, tt.ratio, violations["host-a"], 1e-12)
		})
	}
}

func TestDroppedMetricsDetector_TracksHostsIndependently(t *testing.T) {
	d := newDroppedMetricsDetector()
	badRatio := aboveDroppedBytesWarnThreshold()
	observeDropRatio(d, "host-ok", droppedBytesWarnThreshold/2)
	observeDropRatio(d, "host-bad", badRatio)

	violations := d.violations()
	require.Len(t, violations, 1)
	assert.NotContains(t, violations, "host-ok")
	assert.InDelta(t, badRatio, violations["host-bad"], 1e-12)
}

func TestDroppedMetricsDetector_IgnoresInvalidSeries(t *testing.T) {
	wrongType := makeSerie(dogstatsdClientBytesDropped, "host-x", 100)
	wrongType.MType = metrics.APIGaugeType

	tests := map[string]*metrics.Serie{
		"unrelated metric":  makeSerie("some.other.metric", "host-x", 100),
		"wrong metric type": wrongType,
		"negative value":    makeSerie(dogstatsdClientBytesDropped, "host-x", -1),
		"NaN value":         makeSerie(dogstatsdClientBytesDropped, "host-x", math.NaN()),
		"infinite value":    makeSerie(dogstatsdClientBytesDropped, "host-x", math.Inf(1)),
	}
	for name, serie := range tests {
		t.Run(name, func(t *testing.T) {
			d := newDroppedMetricsDetector()
			d.observe(serie)

			assert.Empty(t, d.hosts)
		})
	}
}

func TestDroppedMetricsDetector_SumsPoints(t *testing.T) {
	d := newDroppedMetricsDetector()
	serie := &metrics.Serie{
		Name:   dogstatsdClientBytesDropped,
		Host:   "host-a",
		MType:  metrics.APIRateType,
		Points: []metrics.Point{{Value: 10}, {Value: 20}, {Value: 30}},
	}
	d.observe(serie)

	assert.Equal(t, 60.0, d.hosts["host-a"].dropped)
}

func TestObservingSerieSink_ForwardsAndObserves(t *testing.T) {
	var collected metrics.Series
	d := newDroppedMetricsDetector()
	sink := &observingSerieSink{inner: &collected, detector: d}
	ratio := aboveDroppedBytesWarnThreshold()

	sink.Append(makeSerie(dogstatsdClientBytesSent, "host-a", 1-ratio))
	sink.Append(makeSerie(dogstatsdClientBytesDropped, "host-a", ratio))
	sink.Append(makeSerie("unrelated.metric", "host-a", 42))

	assert.Len(t, collected, 3)
	require.NotNil(t, d.hosts["host-a"])
	assert.Equal(t, 1-ratio, d.hosts["host-a"].sent)
	assert.Equal(t, ratio, d.hosts["host-a"].dropped)
}

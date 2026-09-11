// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/metricname"
)

func setSDCTestConfig(t *testing.T, overrides map[string]interface{}) {
	cfg := pkgconfigsetup.Datadog()
	for k, v := range overrides {
		key, previous := k, cfg.Get(k)
		cfg.SetInTest(k, v)
		t.Cleanup(func() { cfg.SetInTest(key, previous) })
	}
}

func newSDCTestSampler(checkName string) *CheckSampler {
	return newCheckSampler(
		2, true, true, time.Second, true,
		tags.NewStore(true, "test"), checkid.ID(checkName+":1"), nooptagger.NewComponent(),
	)
}

func sdcTagMatcher() filterlist.TagMatcher {
	return filterlistimpl.NewNoopTagMatcher()
}

func addSDCGauge(cs *CheckSampler, name string, value, ts float64, tagList []string) {
	cs.addSample(&metrics.MetricSample{
		Name: name, Value: value, Mtype: metrics.GaugeType,
		Tags: tagList, SampleRate: 1, Timestamp: ts,
	}, sdcTagMatcher())
}

func addSDCGaugeWithTimestamp(cs *CheckSampler, name string, value, ts float64, tagList []string) {
	cs.addSample(&metrics.MetricSample{
		Name: name, Value: value, Mtype: metrics.GaugeWithTimestampType,
		Tags: tagList, SampleRate: 1, Timestamp: ts,
	}, sdcTagMatcher())
}

func sdcCommit(cs *CheckSampler, ts float64) {
	matcher := metricname.NewMatcher(nil, false)
	cs.commit(ts, &matcher)
}

func sdcCommitAndFlush(cs *CheckSampler, ts float64) metrics.Series {
	sdcCommit(cs, ts)
	series, _ := cs.flush()
	return series
}

func findSDCSerie(series metrics.Series, name string) *metrics.Serie {
	for _, serie := range series {
		if serie.Name == name {
			return serie
		}
	}
	return nil
}

func TestSDC_NotEligibleCheckPassesThrough(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":    false,
		"checks.sdc_compression_checks": []string{},
	})
	cs := newSDCTestSampler("not_eligible")
	require.Nil(t, cs.sdcCompressor)

	addSDCGauge(cs, "my.gauge", 42, 0, nil)
	serie := findSDCSerie(sdcCommitAndFlush(cs, 0), "my.gauge")
	require.NotNil(t, serie)
	require.Equal(t, []metrics.Point{{Ts: 0, Value: 42}}, serie.Points)
}

func TestSDC_CompressesOneFlushWindowAndForcesLastPoint(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":    true,
		"checks.sdc_compression_warmup": 2,
	})
	cs := newSDCTestSampler("flat_window")

	// Several check commits accumulate in one aggregator flush window.
	for ts := 0.0; ts < 5; ts++ {
		addSDCGauge(cs, "my.gauge", 42, ts, nil)
		sdcCommit(cs, ts)
	}
	require.Len(t, cs.sdcCompressor.series, 1)

	series, _ := cs.flush()
	serie := findSDCSerie(series, "my.gauge")
	require.NotNil(t, serie)
	// Warmup is emitted verbatim; the trailing open segment is closed at
	// the window's last real point.
	require.Equal(t, []metrics.Point{{Ts: 0, Value: 42}, {Ts: 1, Value: 42}, {Ts: 4, Value: 42}}, serie.Points)
	require.Len(t, cs.sdcCompressor.series, 1, "the compressor context must survive the flush")
	require.Nil(t, cs.sdcCompressor.series[serie.ContextKey].serie, "only pending points are cleared")
}

func TestSDC_CompressorStateCrossesFlushes(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":    true,
		"checks.sdc_compression_warmup": 2,
	})
	cs := newSDCTestSampler("flush_boundary")

	for ts := 0.0; ts < 2; ts++ {
		addSDCGauge(cs, "my.gauge", 7, ts, nil)
		sdcCommit(cs, ts)
	}
	firstWindow, _ := cs.flush()
	require.Equal(t, []metrics.Point{{Ts: 0, Value: 7}, {Ts: 1, Value: 7}}, findSDCSerie(firstWindow, "my.gauge").Points)

	for ts := 2.0; ts < 4; ts++ {
		addSDCGauge(cs, "my.gauge", 7, ts, nil)
		sdcCommit(cs, ts)
	}
	secondWindow, _ := cs.flush()
	require.Equal(t, []metrics.Point{{Ts: 3, Value: 7}}, findSDCSerie(secondWindow, "my.gauge").Points,
		"warmup and EWMA state must not restart at the second flush")
}

func TestSDC_CheckListEligibility(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":    false,
		"checks.sdc_compression_checks": []string{"selected"},
		"checks.sdc_compression_warmup": 1,
	})
	selected := newSDCTestSampler("selected")
	unselected := newSDCTestSampler("unselected")
	require.NotNil(t, selected.sdcCompressor)
	require.Nil(t, unselected.sdcCompressor)

	for ts := 0.0; ts < 3; ts++ {
		addSDCGauge(selected, "my.gauge", 7, ts, nil)
		addSDCGauge(unselected, "my.gauge", 7, ts, nil)
		sdcCommit(selected, ts)
		sdcCommit(unselected, ts)
	}
	selectedSeries, _ := selected.flush()
	unselectedSeries, _ := unselected.flush()
	require.Equal(t, []metrics.Point{{Ts: 0, Value: 7}, {Ts: 2, Value: 7}}, findSDCSerie(selectedSeries, "my.gauge").Points)
	require.Len(t, unselectedSeries, 3, "the unselected check must retain one uncompressed serie per commit")
	for i, serie := range unselectedSeries {
		require.Equal(t, []metrics.Point{{Ts: float64(i), Value: 7}}, serie.Points)
	}
}

func TestSDC_GaugeWithTimestampCompressesAllPointsInWindow(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":    true,
		"checks.sdc_compression_warmup": 1,
	})
	cs := newSDCTestSampler("gauge_with_timestamp")

	for ts := 100.0; ts < 105; ts++ {
		addSDCGaugeWithTimestamp(cs, "my.gauge", 10, ts, []string{"env:test"})
	}
	serie := findSDCSerie(sdcCommitAndFlush(cs, 999), "my.gauge")
	require.NotNil(t, serie)
	require.Equal(t, []metrics.Point{{Ts: 100, Value: 10}, {Ts: 104, Value: 10}}, serie.Points)
	require.Equal(t, "env:test", serie.Tags.Join(","))
}

func TestSDC_UsesResolvedContextMetricType(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all": true,
	})
	cs := newSDCTestSampler("metric_types")

	// Gauge and Rate both become APIGaugeType series. Only the resolved
	// gauge context must be routed through SDC.
	addSDCGauge(cs, "my.gauge", 1, 1, nil)
	cs.addSample(&metrics.MetricSample{
		Name: "my.rate", Value: 1, Mtype: metrics.RateType, SampleRate: 1, Timestamp: 1,
	}, sdcTagMatcher())
	sdcCommit(cs, 1)
	cs.addSample(&metrics.MetricSample{
		Name: "my.rate", Value: 2, Mtype: metrics.RateType, SampleRate: 1, Timestamp: 2,
	}, sdcTagMatcher())
	sdcCommit(cs, 2)

	require.Len(t, cs.sdcCompressor.series, 1)
	series, _ := cs.flush()
	require.NotNil(t, findSDCSerie(series, "my.gauge"))
	require.NotNil(t, findSDCSerie(series, "my.rate"))
}

func TestSDC_DryRunShipsOriginalAndMeasuresCompressedResult(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":     true,
		"checks.sdc_compression_dry_run": true,
		"checks.sdc_compression_warmup":  1,
	})
	cs := newSDCTestSampler("dry_run")

	for ts := 0.0; ts < 5; ts++ {
		addSDCGauge(cs, "my.gauge", 1, ts, nil)
		sdcCommit(cs, ts)
	}
	series, _ := cs.flush()
	serie := findSDCSerie(series, "my.gauge")
	require.NotNil(t, serie)
	require.Equal(t, []metrics.Point{
		{Ts: 0, Value: 1}, {Ts: 1, Value: 1}, {Ts: 2, Value: 1},
		{Ts: 3, Value: 1}, {Ts: 4, Value: 1},
	}, serie.Points)
	require.EqualValues(t, 5, cs.sdcCompressor.tlmSamples.Get())
	require.EqualValues(t, 2, cs.sdcCompressor.tlmBreakpoints.Get())
}

func TestSDC_SeparatesContexts(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":    true,
		"checks.sdc_compression_warmup": 1,
	})
	cs := newSDCTestSampler("contexts")

	for ts := 0.0; ts < 3; ts++ {
		addSDCGauge(cs, "my.gauge", 1, ts, []string{"instance:a"})
		addSDCGauge(cs, "my.gauge", 2, ts, []string{"instance:b"})
		sdcCommit(cs, ts)
	}
	require.Len(t, cs.sdcCompressor.series, 2)

	series, _ := cs.flush()
	require.Len(t, series, 2)
	for _, serie := range series {
		require.Len(t, serie.Points, 2)
	}
}

func TestSDC_ContextExpiryDoesNotDropPendingSerie(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all": true,
	})
	cs := newSDCTestSampler("expiry")

	addSDCGauge(cs, "my.gauge", 1, 0, []string{"env:test"})
	sdcCommit(cs, 0)
	// expirationCount is two. Expiring resolver metadata must not affect the
	// already-resolved serie owned by the flush-window wrapper.
	sdcCommit(cs, 1)
	sdcCommit(cs, 2)
	require.Zero(t, cs.contextResolver.length())

	series, _ := cs.flush()
	serie := findSDCSerie(series, "my.gauge")
	require.NotNil(t, serie)
	require.Equal(t, []metrics.Point{{Ts: 0, Value: 1}}, serie.Points)
	require.Equal(t, "env:test", serie.Tags.Join(","))
	require.Empty(t, cs.sdcCompressor.series, "expired state must be removed after its pending serie is flushed")
}

func TestSDC_IdleContextExpiryDropsCompressorImmediately(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all": true,
	})
	cs := newSDCTestSampler("idle_expiry")

	addSDCGauge(cs, "my.gauge", 1, 0, nil)
	sdcCommitAndFlush(cs, 0)
	require.Len(t, cs.sdcCompressor.series, 1)

	sdcCommit(cs, 1)
	sdcCommit(cs, 2)
	require.Empty(t, cs.sdcCompressor.series)
}

func TestSDC_FilteredGaugeIsNotStashed(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all": true,
	})
	cs := newSDCTestSampler("filter")
	matcher := metricname.NewMatcher([]string{"filtered.metric"}, false)

	addSDCGauge(cs, "filtered.metric", 1, 0, nil)
	cs.commit(0, &matcher)
	require.Empty(t, cs.sdcCompressor.series)
	series, _ := cs.flush()
	require.Nil(t, findSDCSerie(series, "filtered.metric"))
}

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

func findSDCSeries(series metrics.Series, name string) metrics.Series {
	var found metrics.Series
	for _, serie := range series {
		if serie.Name == name {
			found = append(found, serie)
		}
	}
	return found
}

func findSDCPoints(series metrics.Series, name string) []metrics.Point {
	var points []metrics.Point
	for _, serie := range findSDCSeries(series, name) {
		points = append(points, serie.Points...)
	}
	return points
}

func TestSDC_NotEligibleCheckPassesThrough(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all":    false,
		"adaptive_downsampling.checks": []string{},
	})
	cs := newSDCTestSampler("not_eligible")
	require.Nil(t, cs.sdcDownsampler.series)

	addSDCGauge(cs, "my.gauge", 42, 0, nil)
	serie := findSDCSerie(sdcCommitAndFlush(cs, 0), "my.gauge")
	require.NotNil(t, serie)
	require.Equal(t, []metrics.Point{{Ts: 0, Value: 42}}, serie.Points)
}

func TestSDC_DownsamplesOneFlushWindowAndForcesLastPoint(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all": true,
	})
	cs := newSDCTestSampler("flat_window")

	// Several check commits accumulate in one aggregator flush window.
	for ts := 0.0; ts < 13; ts++ {
		addSDCGauge(cs, "my.gauge", 42, ts, nil)
		sdcCommit(cs, ts)
	}
	require.Len(t, cs.sdcDownsampler.series, 1)
	for _, downsampled := range cs.sdcDownsampler.series {
		require.Len(t, downsampled.series, 13)
	}

	series, _ := cs.flush()
	serie := findSDCSerie(series, "my.gauge")
	require.NotNil(t, serie)
	// Warmup is emitted verbatim; the trailing open segment is closed at
	// the window's last real point.
	require.Equal(t, []metrics.Point{
		{Ts: 0, Value: 42}, {Ts: 1, Value: 42}, {Ts: 2, Value: 42},
		{Ts: 3, Value: 42}, {Ts: 4, Value: 42}, {Ts: 5, Value: 42},
		{Ts: 6, Value: 42}, {Ts: 7, Value: 42}, {Ts: 8, Value: 42},
		{Ts: 9, Value: 42}, {Ts: 12, Value: 42},
	}, findSDCPoints(series, "my.gauge"))
	require.Len(t, cs.sdcDownsampler.series, 1, "the downsampler context must survive the flush")
	require.Empty(t, cs.sdcDownsampler.series[serie.ContextKey].series, "only pending series are cleared")
}

func TestSDC_DownsamplerStateCrossesFlushes(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all": true,
	})
	cs := newSDCTestSampler("flush_boundary")

	for ts := 0.0; ts < 10; ts++ {
		addSDCGauge(cs, "my.gauge", 7, ts, nil)
		sdcCommit(cs, ts)
	}
	firstWindow, _ := cs.flush()
	require.Equal(t, []metrics.Point{
		{Ts: 0, Value: 7}, {Ts: 1, Value: 7}, {Ts: 2, Value: 7},
		{Ts: 3, Value: 7}, {Ts: 4, Value: 7}, {Ts: 5, Value: 7},
		{Ts: 6, Value: 7}, {Ts: 7, Value: 7}, {Ts: 8, Value: 7},
		{Ts: 9, Value: 7},
	}, findSDCPoints(firstWindow, "my.gauge"))

	for ts := 10.0; ts < 13; ts++ {
		addSDCGauge(cs, "my.gauge", 7, ts, nil)
		sdcCommit(cs, ts)
	}
	secondWindow, _ := cs.flush()
	require.Equal(t, []metrics.Point{{Ts: 12, Value: 7}}, findSDCPoints(secondWindow, "my.gauge"),
		"warmup and EWMA state must not restart at the second flush")
}

func TestSDC_CheckListEligibility(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all":    false,
		"adaptive_downsampling.checks": []string{"selected"},
	})
	selected := newSDCTestSampler("selected")
	unselected := newSDCTestSampler("unselected")
	require.NotNil(t, selected.sdcDownsampler.series)
	require.Nil(t, unselected.sdcDownsampler.series)

	for ts := 0.0; ts < 12; ts++ {
		addSDCGauge(selected, "my.gauge", 7, ts, nil)
		addSDCGauge(unselected, "my.gauge", 7, ts, nil)
		sdcCommit(selected, ts)
		sdcCommit(unselected, ts)
	}
	selectedSeries, _ := selected.flush()
	unselectedSeries, _ := unselected.flush()
	require.Equal(t, []metrics.Point{
		{Ts: 0, Value: 7}, {Ts: 1, Value: 7}, {Ts: 2, Value: 7},
		{Ts: 3, Value: 7}, {Ts: 4, Value: 7}, {Ts: 5, Value: 7},
		{Ts: 6, Value: 7}, {Ts: 7, Value: 7}, {Ts: 8, Value: 7},
		{Ts: 9, Value: 7}, {Ts: 11, Value: 7},
	}, findSDCPoints(selectedSeries, "my.gauge"))
	require.Len(t, unselectedSeries, 12, "the unselected check must retain one uncompressed serie per commit")
	for i, serie := range unselectedSeries {
		require.Equal(t, []metrics.Point{{Ts: float64(i), Value: 7}}, serie.Points)
	}
}

func TestSDC_GaugeWithTimestampCompressesAllPointsInWindow(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all": true,
	})
	cs := newSDCTestSampler("gauge_with_timestamp")

	for ts := 100.0; ts < 112; ts++ {
		addSDCGaugeWithTimestamp(cs, "my.gauge", 10, ts, []string{"env:test"})
	}
	serie := findSDCSerie(sdcCommitAndFlush(cs, 999), "my.gauge")
	require.NotNil(t, serie)
	require.Equal(t, []metrics.Point{
		{Ts: 100, Value: 10}, {Ts: 101, Value: 10}, {Ts: 102, Value: 10},
		{Ts: 103, Value: 10}, {Ts: 104, Value: 10}, {Ts: 105, Value: 10},
		{Ts: 106, Value: 10}, {Ts: 107, Value: 10}, {Ts: 108, Value: 10},
		{Ts: 109, Value: 10}, {Ts: 111, Value: 10},
	}, serie.Points)
	require.Equal(t, "env:test", serie.Tags.Join(","))
}

func TestSDC_WarnsOnNonIncreasingTimestampAcrossCommittedSeries(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all": true,
	})
	cs := newSDCTestSampler("non_increasing_timestamp")

	addSDCGaugeWithTimestamp(cs, "my.gauge", 1, 100, nil)
	sdcCommit(cs, 0)
	addSDCGaugeWithTimestamp(cs, "my.gauge", 2, 99, nil)
	sdcCommit(cs, 1)

	require.Len(t, cs.sdcDownsampler.series, 1)
	for _, downsampled := range cs.sdcDownsampler.series {
		require.Equal(t, float64(100), downsampled.latestTimestamp)
	}
	series, _ := cs.flush()
	require.Equal(t, []metrics.Point{{Ts: 100, Value: 1}, {Ts: 99, Value: 2}},
		findSDCPoints(series, "my.gauge"))
}

func TestSDC_UsesResolvedContextMetricType(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all": true,
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

	require.Len(t, cs.sdcDownsampler.series, 1)
	series, _ := cs.flush()
	require.NotNil(t, findSDCSerie(series, "my.gauge"))
	require.NotNil(t, findSDCSerie(series, "my.rate"))
}

func TestSDC_DryRunShipsOriginalAndMeasuresDownsampledResult(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all":     true,
		"adaptive_downsampling.dry_run": true,
	})
	cs := newSDCTestSampler("dry_run")

	for ts := 0.0; ts < 12; ts++ {
		addSDCGauge(cs, "my.gauge", 1, ts, nil)
		sdcCommit(cs, ts)
	}
	series, _ := cs.flush()
	dryRunSeries := findSDCSeries(series, "my.gauge")
	require.Len(t, dryRunSeries, 12, "dry-run must preserve committed series boundaries")
	for _, serie := range dryRunSeries {
		require.Len(t, serie.Points, 1)
	}
	require.Equal(t, []metrics.Point{
		{Ts: 0, Value: 1}, {Ts: 1, Value: 1}, {Ts: 2, Value: 1},
		{Ts: 3, Value: 1}, {Ts: 4, Value: 1}, {Ts: 5, Value: 1},
		{Ts: 6, Value: 1}, {Ts: 7, Value: 1}, {Ts: 8, Value: 1},
		{Ts: 9, Value: 1}, {Ts: 10, Value: 1}, {Ts: 11, Value: 1},
	}, findSDCPoints(series, "my.gauge"))
	require.EqualValues(t, 12, cs.sdcDownsampler.tlmSamples.Get())
	require.EqualValues(t, 11, cs.sdcDownsampler.tlmBreakpoints.Get())
}

func TestSDC_DryRunPreservesCommittedSeries(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all":                true,
		"adaptive_downsampling.dry_run":            true,
		"serializer_max_series_points_per_payload": 10,
	})
	cs := newSDCTestSampler("dry_run_boundaries")

	for commit := 0; commit < 2; commit++ {
		for i := 0; i < 6; i++ {
			ts := float64(commit*6 + i)
			addSDCGaugeWithTimestamp(cs, "my.gauge", 1, ts, nil)
		}
		sdcCommit(cs, float64(commit))
	}

	series, _ := cs.flush()
	dryRunSeries := findSDCSeries(series, "my.gauge")
	require.Len(t, dryRunSeries, 2)
	for _, serie := range dryRunSeries {
		require.Len(t, serie.Points, 6)
	}
}

func TestSDC_SeparatesContexts(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all": true,
	})
	cs := newSDCTestSampler("contexts")

	for ts := 0.0; ts < 12; ts++ {
		addSDCGauge(cs, "my.gauge", 1, ts, []string{"instance:a"})
		addSDCGauge(cs, "my.gauge", 2, ts, []string{"instance:b"})
		sdcCommit(cs, ts)
	}
	require.Len(t, cs.sdcDownsampler.series, 2)

	series, _ := cs.flush()
	require.Len(t, series, 22)
	for _, serie := range series {
		require.Len(t, serie.Points, 1)
	}
}

func TestSDC_ContextExpiryDoesNotDropPendingSerie(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all": true,
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
	require.Empty(t, cs.sdcDownsampler.series, "expired state must be removed after its pending serie is flushed")
}

func TestSDC_IdleContextExpiryDropsDownsamplerImmediately(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all": true,
	})
	cs := newSDCTestSampler("idle_expiry")

	addSDCGauge(cs, "my.gauge", 1, 0, nil)
	sdcCommitAndFlush(cs, 0)
	require.Len(t, cs.sdcDownsampler.series, 1)

	sdcCommit(cs, 1)
	sdcCommit(cs, 2)
	require.Empty(t, cs.sdcDownsampler.series)
}

func TestSDC_FilteredGaugeIsNotStashed(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all": true,
	})
	cs := newSDCTestSampler("filter")
	matcher := metricname.NewMatcher([]string{"filtered.metric"}, false)

	addSDCGauge(cs, "filtered.metric", 1, 0, nil)
	cs.commit(0, &matcher)
	require.Empty(t, cs.sdcDownsampler.series)
	series, _ := cs.flush()
	require.Nil(t, findSDCSerie(series, "filtered.metric"))
}

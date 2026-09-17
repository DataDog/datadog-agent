// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"math"
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

// sdcWirePoints exercises the real point serializer's timestamp precision.
func sdcWirePoints(t *testing.T, series metrics.Series, name string) []metrics.Point {
	t.Helper()
	var wire []metrics.Point
	for _, point := range findSDCPoints(series, name) {
		encoded, err := point.MarshalJSON()
		require.NoError(t, err)
		var decoded metrics.Point
		require.NoError(t, decoded.UnmarshalJSON(encoded))
		wire = append(wire, decoded)
	}
	return wire
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

func TestSDC_PeriodicClosing(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all":                   true,
		"adaptive_downsampling.close_every_n_flushes": 4,
	})
	cs := newSDCTestSampler("periodic_close")

	for ts := 0.0; ts < 13; ts++ {
		addSDCGauge(cs, "my.gauge", 42, ts, nil)
		sdcCommit(cs, ts)
	}
	first, _ := cs.flush()
	require.Len(t, findSDCPoints(first, "my.gauge"), 10, "only warmup points ship on flush one")

	for window := 2; window <= 8; window++ {
		ts := float64(11 + window)
		addSDCGauge(cs, "my.gauge", 42, ts, nil)
		sdcCommit(cs, ts)
		output, _ := cs.flush()
		if window%4 == 0 {
			require.Equal(t, []metrics.Point{{Ts: ts, Value: 42}}, findSDCPoints(output, "my.gauge"))
		} else {
			require.Empty(t, output, "flush %d should not close the segment", window)
		}
		for _, ds := range cs.sdcDownsampler.series {
			require.Empty(t, ds.series, "pending output must be drained even when closing is skipped")
			require.Nil(t, ds.template.Points, "the metadata template must not retain raw points")
		}
	}
	require.Len(t, findSDCPoints(first, "my.gauge"), 10, "later flushes must not mutate returned output")
}

func TestSDC_DeferredEndpointAfterQuietWindow(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		for _, expire := range []bool{false, true} {
			name := "active"
			if dryRun {
				name = "dry_run"
			}
			if expire {
				name += "_expired"
			}
			t.Run(name, func(t *testing.T) {
				setSDCTestConfig(t, map[string]interface{}{
					"adaptive_downsampling.all":                   true,
					"adaptive_downsampling.dry_run":               dryRun,
					"adaptive_downsampling.close_every_n_flushes": 4,
				})
				cs := newSDCTestSampler("quiet_" + name)
				before := cs.sdcDownsampler.tlmBreakpoints.Get()
				for ts := 0.0; ts < 13; ts++ {
					addSDCGauge(cs, "my.gauge", 42, ts, []string{"env:test"})
					sdcCommit(cs, ts)
				}
				first, _ := cs.flush()
				expectedCount := 10
				if dryRun {
					expectedCount = 13
				}
				require.Len(t, findSDCPoints(first, "my.gauge"), expectedCount)
				require.EqualValues(t, 10, cs.sdcDownsampler.tlmBreakpoints.Get()-before)
				original := first[len(first)-1]
				snapshot := *original
				snapshot.Points = append([]metrics.Point(nil), original.Points...)

				if expire {
					// No new samples: expiration must preserve the endpoint even
					// though the previous flush cleared all pending output series.
					sdcCommit(cs, 13)
					sdcCommit(cs, 14)
					require.Zero(t, cs.contextResolver.length())
					require.Len(t, cs.sdcDownsampler.series, 1)
				} else {
					for window := 2; window <= 3; window++ {
						output, _ := cs.flush()
						require.Empty(t, output)
					}
				}

				closed, _ := cs.flush()
				require.EqualValues(t, 11, cs.sdcDownsampler.tlmBreakpoints.Get()-before)
				if dryRun {
					require.Empty(t, closed, "dry-run must never emit an additional endpoint")
				} else {
					require.Len(t, closed, 1)
					expected := snapshot
					expected.Points = []metrics.Point{{Ts: 12, Value: 42}}
					require.Equal(t, expected, *closed[0], "quiet closes must preserve series metadata")
					require.NotSame(t, original, closed[0])
				}
				require.Equal(t, snapshot, *original, "returned series must never be mutated")
				if expire {
					require.Empty(t, cs.sdcDownsampler.series)
				}
				for window := 0; window < 4; window++ {
					output, _ := cs.flush()
					require.Empty(t, output, "quiet endpoints must not be emitted twice")
				}
				require.EqualValues(t, 11, cs.sdcDownsampler.tlmBreakpoints.Get()-before)
			})
		}
	}
}

func TestSDC_PeriodicClosingDrainsBreakpointsEveryFlush(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all":                   true,
		"adaptive_downsampling.close_every_n_flushes": 4,
	})
	cs := newSDCTestSampler("periodic_breakpoints")
	for ts := 0.0; ts <= 10; ts++ {
		addSDCGauge(cs, "my.gauge", 42, ts, nil)
		sdcCommit(cs, ts)
	}
	first, _ := cs.flush()
	require.Len(t, first, 10)

	addSDCGauge(cs, "my.gauge", 1000, 11, nil)
	sdcCommit(cs, 11)
	second, _ := cs.flush()
	require.Equal(t, []metrics.Point{{Ts: 10, Value: 42}}, findSDCPoints(second, "my.gauge"))

	addSDCGauge(cs, "my.gauge", -1000, 12, nil)
	sdcCommit(cs, 12)
	third, _ := cs.flush()
	require.Equal(t, []metrics.Point{{Ts: 11, Value: 1000}}, findSDCPoints(third, "my.gauge"))
	fourth, _ := cs.flush()
	require.Equal(t, []metrics.Point{{Ts: 12, Value: -1000}}, findSDCPoints(fourth, "my.gauge"))
	require.Equal(t, []metrics.Point{{Ts: 11, Value: 1000}}, findSDCPoints(third, "my.gauge"),
		"the deferred endpoint must not be appended to an already-returned series")
}

func TestSDC_PeriodicDryRunMatchesDisabled(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all":                   false,
		"adaptive_downsampling.checks":                []string{},
		"adaptive_downsampling.close_every_n_flushes": 4,
	})
	disabled := newSDCTestSampler("periodic_disabled")
	setSDCTestConfig(t, map[string]interface{}{
		"adaptive_downsampling.all":     true,
		"adaptive_downsampling.dry_run": true,
	})
	dryRun := newSDCTestSampler("periodic_dry_run")
	setSDCTestConfig(t, map[string]interface{}{"adaptive_downsampling.dry_run": false})
	active := newSDCTestSampler("periodic_active")
	dryBreakpoints := dryRun.sdcDownsampler.tlmBreakpoints.Get()
	activeBreakpoints := active.sdcDownsampler.tlmBreakpoints.Get()
	for window := 0; window < 8; window++ {
		for _, sampler := range []*CheckSampler{disabled, dryRun, active} {
			for i := 0; i < 6; i++ {
				ts := float64(window*6 + i)
				addSDCGaugeWithTimestamp(sampler, "my.gauge", 42, ts, []string{"env:test"})
				if i%3 == 2 {
					sdcCommit(sampler, ts)
				}
			}
		}
		expected, _ := disabled.flush()
		actual, _ := dryRun.flush()
		active.flush()
		require.Len(t, actual, 2, "dry-run must preserve committed series boundaries")
		require.Equal(t, expected, actual)
		require.Equal(t, active.sdcDownsampler.tlmBreakpoints.Get()-activeBreakpoints,
			dryRun.sdcDownsampler.tlmBreakpoints.Get()-dryBreakpoints)
	}
}

func TestSDC_CloseIntervalBelowOneUsesEveryFlush(t *testing.T) {
	for _, interval := range []int{0, -1} {
		setSDCTestConfig(t, map[string]interface{}{
			"adaptive_downsampling.all":                   true,
			"adaptive_downsampling.close_every_n_flushes": interval,
		})
		cs := newSDCTestSampler("invalid_interval")
		require.Equal(t, 1, cs.sdcDownsampler.closeEveryNFlushes)
	}
}

func TestSDC_SerializedTimestampErrorBound(t *testing.T) {
	for _, timestamped := range []bool{false, true} {
		name := "gauge"
		if timestamped {
			name = "gauge_with_timestamp"
		}
		t.Run(name, func(t *testing.T) {
			setSDCTestConfig(t, map[string]interface{}{
				"adaptive_downsampling.all":                    true,
				"adaptive_downsampling.close_every_n_flushes":  1,
				"adaptive_downsampling.relative_error":         0.02,
				"adaptive_downsampling.scale_smoothing_factor": 0.3,
			})
			cs := newSDCTestSampler("wire_timestamps_" + name)
			submit := func(ts, value float64) {
				if timestamped {
					addSDCGaugeWithTimestamp(cs, "my.gauge", value, ts, nil)
				} else {
					addSDCGauge(cs, "my.gauge", value, ts, nil)
				}
				sdcCommit(cs, ts)
			}
			for i := 0; i < 9; i++ {
				submit(float64(i), 0)
			}
			// These points are collinear before serialization, but not after
			// truncation: (9,0) -> (10,10) -> (11,60). The middle must survive.
			submit(9.9, 0)
			submit(10.1, 10)
			submit(11.1, 60)
			series, _ := cs.flush()
			wire := sdcWirePoints(t, series, "my.gauge")
			require.Contains(t, wire, metrics.Point{Ts: 10, Value: 10})
			found := false
			for i := 1; i < len(wire); i++ {
				left, right := wire[i-1], wire[i]
				if left.Ts <= 10 && right.Ts >= 10 {
					reconstructed := left.Value + (right.Value-left.Value)*(10-left.Ts)/(right.Ts-left.Ts)
					// EWMA is zero after warmup, then 0.3*10=3 at this sample.
					require.InDelta(t, 10.0, reconstructed, 0.02*3)
					found = true
					break
				}
			}
			require.True(t, found)
		})
	}
}

func TestSDC_SerializedBoundAcrossPeriodicFlushes(t *testing.T) {
	for _, interval := range []int{1, 4} {
		setSDCTestConfig(t, map[string]interface{}{
			"adaptive_downsampling.all":                    true,
			"adaptive_downsampling.close_every_n_flushes":  interval,
			"adaptive_downsampling.relative_error":         0.02,
			"adaptive_downsampling.scale_smoothing_factor": 0.3,
		})
		cs := newSDCTestSampler("wire_periodic")
		var output metrics.Series
		var samples []metrics.Point
		var tolerances []float64
		var scale float64
		for i := 0; i < 240; i++ {
			// Nonuniform fractional timestamps become evenly spaced on wire.
			ts := float64(i) + 0.05 + 0.1*float64((i*3)%9)
			value := 100 + 40*math.Sin(2*math.Pi*float64(i)/120) + float64((i*7)%5-2)
			if i == 0 {
				scale = math.Abs(value)
			} else {
				scale = 0.3*math.Abs(value) + 0.7*scale
			}
			samples = append(samples, metrics.Point{Ts: float64(int64(ts)), Value: value})
			tolerances = append(tolerances, 0.02*scale)
			addSDCGauge(cs, "my.gauge", value, ts, nil)
			sdcCommit(cs, ts)
			if (i+1)%15 == 0 {
				series, _ := cs.flush()
				output = append(output, series...)
			}
		}
		wire := sdcWirePoints(t, output, "my.gauge")
		require.Less(t, len(wire), len(samples))
		endpoint := 1
		for i, sample := range samples {
			for endpoint < len(wire)-1 && wire[endpoint].Ts < sample.Ts {
				endpoint++
			}
			left, right := wire[endpoint-1], wire[endpoint]
			require.Greater(t, right.Ts, left.Ts)
			require.LessOrEqual(t, left.Ts, sample.Ts)
			require.GreaterOrEqual(t, right.Ts, sample.Ts)
			reconstructed := left.Value + (right.Value-left.Value)*(sample.Ts-left.Ts)/(right.Ts-left.Ts)
			require.InDelta(t, sample.Value, reconstructed, tolerances[i]+1e-9,
				"interval=%d sample=%d", interval, i)
		}
	}
}

func TestSDC_SameSecondSamplesPassThrough(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		setSDCTestConfig(t, map[string]interface{}{
			"adaptive_downsampling.all":     true,
			"adaptive_downsampling.dry_run": dryRun,
		})
		cs := newSDCTestSampler("same_second")
		for _, point := range []metrics.Point{{Ts: 100.1, Value: 1}, {Ts: 100.9, Value: 2}} {
			addSDCGaugeWithTimestamp(cs, "my.gauge", point.Value, point.Ts, nil)
			sdcCommit(cs, point.Ts)
		}
		series, _ := cs.flush()
		require.Equal(t, []metrics.Point{{Ts: 100, Value: 1}, {Ts: 100, Value: 2}}, sdcWirePoints(t, series, "my.gauge"))
		if dryRun {
			require.Equal(t, []metrics.Point{{Ts: 100.1, Value: 1}, {Ts: 100.9, Value: 2}}, findSDCPoints(series, "my.gauge"),
				"dry-run must retain the original timestamps and committed series")
		}
	}
}

func TestSDC_RetiringSamplerClosesEndpoint(t *testing.T) {
	for _, finalFlush := range []bool{false, true} {
		for _, dryRun := range []bool{false, true} {
			name := "deregister"
			if finalFlush {
				name = "final_flush"
			}
			if dryRun {
				name += "_dry_run"
			}
			t.Run(name, func(t *testing.T) {
				setSDCTestConfig(t, map[string]interface{}{
					"adaptive_downsampling.all":                   true,
					"adaptive_downsampling.dry_run":               dryRun,
					"adaptive_downsampling.close_every_n_flushes": 4,
				})
				cs := newSDCTestSampler("retiring_" + name)
				for ts := 0.0; ts < 13; ts++ {
					addSDCGauge(cs, "my.gauge", 42, ts, nil)
					sdcCommit(cs, ts)
				}
				first, _ := cs.flush()
				if dryRun {
					require.Len(t, findSDCPoints(first, "my.gauge"), 13)
				} else {
					require.Len(t, findSDCPoints(first, "my.gauge"), 10)
				}
				before := cs.sdcDownsampler.tlmBreakpoints.Get()
				agg := &BufferedAggregator{checkSamplers: map[checkid.ID]*CheckSampler{cs.id: cs}}
				if !finalFlush {
					agg.handleDeregisterSampler(cs.id)
				}
				var output metrics.Series
				var sketches metrics.SketchSeriesList
				agg.getSeriesAndSketches(time.Time{}, &output, &sketches, finalFlush)
				require.Empty(t, agg.checkSamplers)
				if dryRun {
					require.Empty(t, findSDCPoints(output, "my.gauge"))
				} else {
					require.Equal(t, []metrics.Point{{Ts: 12, Value: 42}}, findSDCPoints(output, "my.gauge"))
				}
				require.EqualValues(t, 1, cs.sdcDownsampler.tlmBreakpoints.Get()-before)
			})
		}
	}
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

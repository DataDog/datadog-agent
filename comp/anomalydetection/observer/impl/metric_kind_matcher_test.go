// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

func makeSyntheticPoints(startTs int64, stepSec int64, vals []float64) []observerdef.Point {
	pts := make([]observerdef.Point, len(vals))
	for i, v := range vals {
		pts[i] = observerdef.Point{
			Timestamp: startTs + int64(i)*stepSec,
			Value:     v,
		}
	}
	return pts
}

func TestMetricKindMatcher_TransitionSemantics(t *testing.T) {
	var scripted []observerdef.MetricKind
	idx := 0
	matcher := newMetricKindMatcherWithClassifier(
		observerdef.MetricKindMatcherConfig{
			MinPoints:               5,
			MaxPoints:               10,
			TransitionConfirmations: 3,
		},
		func(_ []observerdef.Point, _ *matcherWorkspace) observerdef.MetricKind {
			k := scripted[idx]
			idx++
			return k
		},
	)

	ref := observerdef.SeriesRef(1)
	// Insufficient history (< MinPoints=5) remains UNSET without invoking classifier.
	shortPts := makeSyntheticPoints(1, 1, []float64{10, 10, 10, 10})
	tr, changed := matcher.Observe(ref, observerdef.UNSET, shortPts, 4)
	require.False(t, changed)
	require.Equal(t, observerdef.KindTransition{}, tr)
	require.Equal(t, 0, idx)

	// First eligible window: immediate assignment UNSET -> STABLE.
	scripted = []observerdef.MetricKind{
		observerdef.STABLE,
		observerdef.LINEAR,
		observerdef.LINEAR,
		observerdef.STABLE, // resets pending candidate
		observerdef.LINEAR,
		observerdef.UNKNOWN, // switches candidate to UNKNOWN (count=1)
		observerdef.LINEAR,  // switches candidate back to LINEAR (count=1)
		observerdef.LINEAR,  // count=2
		observerdef.LINEAR,  // count=3 -> commits STABLE -> LINEAR
	}

	pts5 := makeSyntheticPoints(1, 1, []float64{10, 10, 10, 10, 10})
	tr, changed = matcher.Observe(ref, observerdef.UNSET, pts5, 5)
	require.True(t, changed)
	require.Equal(t, observerdef.KindTransition{Ref: ref, From: observerdef.UNSET, To: observerdef.STABLE}, tr)
	current := tr.To

	// Advance without new points (same last timestamp = 5) does not evaluate or increment confirmations.
	_, changed = matcher.Observe(ref, current, pts5, 6)
	require.False(t, changed)
	require.Equal(t, 1, idx, "classifier must not run when no new points are visible")

	// Fresh observation 1 (ts=6): predicts LINEAR (1/3)
	pts6 := append(pts5, observerdef.Point{Timestamp: 6, Value: 11})
	_, changed = matcher.Observe(ref, current, pts6, 6)
	require.False(t, changed)

	// Fresh observation 2 (ts=7): predicts LINEAR (2/3)
	pts7 := append(pts6, observerdef.Point{Timestamp: 7, Value: 12})
	_, changed = matcher.Observe(ref, current, pts7, 7)
	require.False(t, changed)

	// Fresh observation 3 (ts=8): predicts STABLE -> clears pending candidate
	pts8 := append(pts7, observerdef.Point{Timestamp: 8, Value: 10})
	_, changed = matcher.Observe(ref, current, pts8, 8)
	require.False(t, changed)

	// Fresh observation 4 (ts=9): predicts LINEAR (1/3)
	pts9 := append(pts8, observerdef.Point{Timestamp: 9, Value: 11})
	_, changed = matcher.Observe(ref, current, pts9, 9)
	require.False(t, changed)

	// Fresh observation 5 (ts=10): predicts UNKNOWN -> candidate becomes UNKNOWN (1/3)
	pts10 := append(pts9, observerdef.Point{Timestamp: 10, Value: 20})
	_, changed = matcher.Observe(ref, current, pts10, 10)
	require.False(t, changed)

	// Next 3 fresh observations (ts=11, 12, 13): all predict LINEAR -> commits on 3rd
	pts11 := append(pts10, observerdef.Point{Timestamp: 11, Value: 21})
	_, changed = matcher.Observe(ref, current, pts11, 11)
	require.False(t, changed)

	pts12 := append(pts11, observerdef.Point{Timestamp: 12, Value: 22})
	_, changed = matcher.Observe(ref, current, pts12, 12)
	require.False(t, changed)

	pts13 := append(pts12, observerdef.Point{Timestamp: 13, Value: 23})
	tr, changed = matcher.Observe(ref, current, pts13, 13)
	require.True(t, changed)
	require.Equal(t, observerdef.KindTransition{Ref: ref, From: observerdef.STABLE, To: observerdef.LINEAR}, tr)
}

func TestMetricKindMatcher_RemoveAndReset(t *testing.T) {
	matcher := newMetricKindMatcherWithClassifier(
		observerdef.MetricKindMatcherConfig{
			MinPoints:               3,
			MaxPoints:               10,
			TransitionConfirmations: 2,
		},
		func(_ []observerdef.Point, _ *matcherWorkspace) observerdef.MetricKind {
			return observerdef.LINEAR
		},
	)
	ref := observerdef.SeriesRef(7)
	pts := makeSyntheticPoints(1, 1, []float64{1, 2, 3})
	_, changed := matcher.Observe(ref, observerdef.STABLE, pts, 3)
	require.False(t, changed) // 1/2 confirmation

	matcher.RemoveSeries([]observerdef.SeriesRef{ref})
	pts4 := append(pts, observerdef.Point{Timestamp: 4, Value: 4})
	_, changed = matcher.Observe(ref, observerdef.STABLE, pts4, 4)
	require.False(t, changed, "RemoveSeries must clear partial candidate confirmations")

	matcher.Reset()
	pts5 := append(pts4, observerdef.Point{Timestamp: 5, Value: 5})
	_, changed = matcher.Observe(ref, observerdef.STABLE, pts5, 5)
	require.False(t, changed, "Reset must clear partial candidate confirmations")
}

func TestMetricKindMatcher_Config(t *testing.T) {
	defaults := observerdef.MetricKindMatcherConfig{MinPoints: 30, MaxPoints: 30, TransitionConfirmations: 3}
	require.Equal(t, defaults, DefaultMetricKindMatcherConfig())
	for _, tc := range []struct {
		name string
		cfg  observerdef.MetricKindMatcherConfig
		want observerdef.MetricKindMatcherConfig
	}{
		{"zero", observerdef.MetricKindMatcherConfig{}, defaults},
		{"negative", observerdef.MetricKindMatcherConfig{MinPoints: -1, MaxPoints: -2, TransitionConfirmations: -3}, defaults},
		{"explicit", observerdef.MetricKindMatcherConfig{MinPoints: 5, MaxPoints: 10, TransitionConfirmations: 1},
			observerdef.MetricKindMatcherConfig{MinPoints: 5, MaxPoints: 10, TransitionConfirmations: 1}},
		{"maximum below minimum", observerdef.MetricKindMatcherConfig{MinPoints: 130, MaxPoints: 10},
			observerdef.MetricKindMatcherConfig{MinPoints: 130, MaxPoints: 130, TransitionConfirmations: 3}},
		{"default maximum below explicit minimum", observerdef.MetricKindMatcherConfig{MinPoints: 60},
			observerdef.MetricKindMatcherConfig{MinPoints: 60, MaxPoints: 60, TransitionConfirmations: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMetricKindMatcher(tc.cfg).(*metricKindMatcher)
			require.Equal(t, tc.want, m.cfg)
			require.Equal(t, observerdef.DetectorPointWindow{MinPoints: tc.want.MinPoints, MaxPoints: tc.want.MaxPoints}, m.Window())
		})
	}
}

func TestMetricKindMatcher_InvalidWindowWarning(t *testing.T) {
	level, err := pkglog.GetLogLevelSpec()
	require.NoError(t, err)
	var output bytes.Buffer
	logger, err := pkglog.LoggerFromWriterWithMinLevelAndMsgFormat(&output, pkglog.WarnLvl)
	require.NoError(t, err)
	pkglog.SetupLogger(logger, "warn")
	t.Cleanup(func() { pkglog.SetupLogger(pkglog.Default(), level) })

	m := NewMetricKindMatcher(observerdef.MetricKindMatcherConfig{MinPoints: 30, MaxPoints: 10})
	pkglog.Flush()
	require.Contains(t, output.String(), "[anomalydetection] MetricKindMatcher max_points=10 is below min_points=30; using 30")
	require.Equal(t, observerdef.DetectorPointWindow{MinPoints: 30, MaxPoints: 30}, m.Window())

	output.Reset()
	NewMetricKindMatcher(DefaultMetricKindMatcherConfig())
	pkglog.Flush()
	require.Empty(t, output.String(), "valid windows should not warn")
}

func TestMetricKindMatcher_DefaultUsesLatestThirtyVisiblePoints(t *testing.T) {
	var received []observerdef.Point
	m := newMetricKindMatcherWithClassifier(observerdef.MetricKindMatcherConfig{},
		func(points []observerdef.Point, _ *matcherWorkspace) observerdef.MetricKind {
			received = append([]observerdef.Point(nil), points...)
			return observerdef.STABLE
		})
	points := make([]observerdef.Point, 125)
	for i := range points {
		points[i] = observerdef.Point{Timestamp: int64(i + 1), Value: float64(i)}
		if i >= 90 && i < 120 {
			points[i].Value = 100
		}
		if i >= 120 {
			points[i].Value = math.NaN()
		}
	}
	_, changed := m.Observe(1, observerdef.UNSET, points, 120)
	require.True(t, changed)
	require.Equal(t, points[90:120], received, "ignore old history and future replay observations")
	tr, changed := NewMetricKindMatcher(DefaultMetricKindMatcherConfig()).Observe(1, observerdef.UNSET, points, 120)
	require.True(t, changed)
	require.Equal(t, observerdef.STABLE, tr.To, "only the most recent visible shape matters")
}

func TestMetricKindMatcher_BoundedVisibleWindow(t *testing.T) {
	var windows [][]observerdef.Point
	m := newMetricKindMatcherWithClassifier(
		observerdef.MetricKindMatcherConfig{MinPoints: 3, MaxPoints: 4},
		func(points []observerdef.Point, _ *matcherWorkspace) observerdef.MetricKind {
			windows = append(windows, append([]observerdef.Point(nil), points...))
			return observerdef.STABLE
		},
	)
	points := makeSyntheticPoints(1, 1, []float64{10, 10, 10, 10, 10, 10, 10, 999})
	original := append([]observerdef.Point(nil), points...)
	for _, now := range []int64{0, 2} {
		_, changed := m.Observe(1, observerdef.UNSET, points, now)
		require.False(t, changed)
	}
	require.Empty(t, windows, "future points must not satisfy the minimum")
	require.Empty(t, m.series, "insufficient history must not allocate candidate state")
	tr, changed := m.Observe(1, observerdef.UNSET, points, 3)
	require.True(t, changed, "exact minimum is eligible")
	require.Equal(t, observerdef.STABLE, tr.To)
	require.Equal(t, points[:3], windows[0])

	_, changed = m.Observe(1, tr.To, points, 7)
	require.False(t, changed)
	require.Equal(t, points[3:7], windows[1], "trim the visible tail, not the preloaded future tail")
	require.Equal(t, original, points, "Observe must not mutate borrowed points")
	for _, now := range []int64{7, 6, 100} {
		// Even a later advance is not fresh when the input buffer hasn't advanced.
		m.Observe(1, tr.To, points[:7], now)
	}
	require.Len(t, windows, 2)
}

func TestMetricKindMatcher_AssignedKindTransitions(t *testing.T) {
	for _, current := range []observerdef.MetricKind{observerdef.UNSET, observerdef.UNKNOWN, observerdef.STABLE, observerdef.LINEAR} {
		for _, predicted := range []observerdef.MetricKind{observerdef.UNKNOWN, observerdef.STABLE, observerdef.LINEAR} {
			t.Run(current.String()+" to "+predicted.String(), func(t *testing.T) {
				m := newMetricKindMatcherWithClassifier(
					observerdef.MetricKindMatcherConfig{MinPoints: 3, MaxPoints: 4},
					func(_ []observerdef.Point, _ *matcherWorkspace) observerdef.MetricKind { return predicted },
				)
				for i := 0; i < 3; i++ {
					points := makeSyntheticPoints(int64(i+1), 1, []float64{10, 10, 10})
					tr, changed := m.Observe(1, current, points, int64(i+3))
					wantChange := current != predicted && (current == observerdef.UNSET || i == 2)
					require.Equal(t, wantChange, changed)
					if changed {
						require.Equal(t, observerdef.KindTransition{Ref: 1, From: current, To: predicted}, tr)
						require.Equal(t, 0, m.series[1].confirmations)
						break // The caller would synchronously commit this assignment.
					}
				}
			})
		}
	}
}

func TestMetricKindMatcher_FreshBatchAndSeriesIsolation(t *testing.T) {
	m := newMetricKindMatcherWithClassifier(
		observerdef.MetricKindMatcherConfig{MinPoints: 3, MaxPoints: 4},
		func(_ []observerdef.Point, _ *matcherWorkspace) observerdef.MetricKind { return observerdef.LINEAR },
	)
	points := makeSyntheticPoints(1, 1, []float64{1, 2, 3, 4, 5, 6, 7, 8})
	_, changed := m.Observe(1, observerdef.STABLE, points[:3], 3)
	require.False(t, changed)
	_, changed = m.Observe(1, observerdef.STABLE, points[:7], 7)
	require.False(t, changed, "a batch earns one confirmation, not one per new point")
	require.Equal(t, 2, m.series[1].confirmations)
	points[6].Value = 42
	_, changed = m.Observe(1, observerdef.STABLE, points[:7], 7)
	require.False(t, changed, "same-timestamp updates are not fresh evidence")
	require.Equal(t, 2, m.series[1].confirmations)
	m.Observe(2, observerdef.STABLE, points[:7], 7)
	require.Equal(t, 1, m.series[2].confirmations)
	m.RemoveSeries([]observerdef.SeriesRef{2, 999})
	require.Len(t, m.series, 1)
	tr, changed := m.Observe(1, observerdef.STABLE, points, 8)
	require.True(t, changed)
	require.Equal(t, observerdef.LINEAR, tr.To)
	m.Reset()
	require.Empty(t, m.series)
}

func TestMetricKindMatcher_StorageOwnsCommittedKind(t *testing.T) {
	s := newTimeSeriesStorage()
	ref := s.AddWithKeyAndHostComposite("check", "gauge", "", 10, 1, tagset.CompositeTags{}, 1, metrics.GaugeType).Ref
	generation := s.SeriesGeneration()
	m := NewMetricKindMatcher(observerdef.MetricKindMatcherConfig{MinPoints: 3, MaxPoints: 4, TransitionConfirmations: 1})
	points := makeSyntheticPoints(1, 1, []float64{10, 10, 10})
	tr, changed := m.Observe(ref, observerdef.UNSET, points, 3)
	require.True(t, changed)
	kind, ok := s.SeriesKind(ref)
	require.True(t, ok)
	require.Equal(t, observerdef.UNSET, kind)
	require.Equal(t, generation, s.SeriesGeneration())
	s.SetSeriesKind(ref, tr.To)
	require.Equal(t, observerdef.STABLE, s.GetSeriesMeta(ref).MetricKind)

	// Reuse the input buffer for another series, then supply fresh data against
	// the authoritative kind. The matcher must not retain borrowed buffers or
	// cache the committed kind.
	for i := range points {
		points[i] = observerdef.Point{Timestamp: int64(i + 1), Value: float64(i)}
	}
	m.Observe(2, observerdef.UNSET, points, 3)
	_, changed = m.Observe(ref, observerdef.STABLE,
		makeSyntheticPoints(2, 1, []float64{10, 10, 10}), 4)
	require.False(t, changed)
	s.SetSeriesKind(ref, observerdef.UNKNOWN)
	tr, changed = m.Observe(ref, observerdef.UNKNOWN,
		makeSyntheticPoints(3, 1, []float64{10, 10, 10}), 5)
	require.True(t, changed, "the supplied committed kind is authoritative")
	require.Equal(t, observerdef.UNKNOWN, tr.From)
}

func TestMetricKindMatcher_InsufficientHistoryKeepsPendingEvidence(t *testing.T) {
	m := newMetricKindMatcherWithClassifier(
		observerdef.MetricKindMatcherConfig{MinPoints: 3, MaxPoints: 4, TransitionConfirmations: 2},
		func(_ []observerdef.Point, _ *matcherWorkspace) observerdef.MetricKind { return observerdef.LINEAR },
	)
	m.Observe(1, observerdef.STABLE, makeSyntheticPoints(1, 1, []float64{1, 2, 3}), 3)
	_, changed := m.Observe(1, observerdef.STABLE, makeSyntheticPoints(4, 1, []float64{4, 5}), 5)
	require.False(t, changed)
	require.Equal(t, 1, m.series[1].confirmations)
	tr, changed := m.Observe(1, observerdef.STABLE, makeSyntheticPoints(4, 1, []float64{4, 5, 6}), 6)
	require.True(t, changed)
	require.Equal(t, observerdef.LINEAR, tr.To)
}

func TestClassifyMetricKind_InvalidWindows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		points []observerdef.Point
	}{
		{"empty", nil},
		{"too short", makeSyntheticPoints(1, 1, []float64{1, 2})},
		{"no time span", makeSyntheticPoints(1, 0, []float64{1, 2, 3})},
		{"nan", makeSyntheticPoints(1, 1, []float64{1, math.NaN(), 3})},
		{"positive infinity", makeSyntheticPoints(1, 1, []float64{1, math.Inf(1), 3})},
		{"negative infinity", makeSyntheticPoints(1, 1, []float64{1, math.Inf(-1), 3})},
		{"non-finite fit score", makeSyntheticPoints(1, 1, []float64{1e200, -1e200, 1e200})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ws matcherWorkspace
			require.Equal(t, observerdef.UNKNOWN, classifyMetricKind(tc.points, &ws))
		})
	}
}

// Lazy statistics must not consume scratch left behind by another shape/window.
func TestClassifyMetricKind_WorkspaceReuse(t *testing.T) {
	var reused matcherWorkspace
	for _, tc := range matcherEvalCorpus(101, 102) {
		var fresh matcherWorkspace
		want := classifyMetricKind(tc.points, &fresh)
		require.Equal(t, want, classifyMetricKind(tc.points, &reused), "%s, points=%d", tc.family, len(tc.points))
	}
}

func TestClassifyMetricKind_Shapes(t *testing.T) {
	var ws matcherWorkspace

	t.Run("exact constant and zero constant are STABLE", func(t *testing.T) {
		zeros := make([]float64, 40)
		ptsZero := makeSyntheticPoints(100, 10, zeros)
		assert.Equal(t, observerdef.STABLE, classifyMetricKind(ptsZero, &ws))

		flat := make([]float64, 40)
		for i := range flat {
			flat[i] = 512.0
		}
		ptsFlat := makeSyntheticPoints(100, 10, flat)
		assert.Equal(t, observerdef.STABLE, classifyMetricKind(ptsFlat, &ws))
	})

	t.Run("noisy plateau is STABLE", func(t *testing.T) {
		rng := rand.New(rand.NewSource(7))
		vals := make([]float64, 50)
		for i := range vals {
			vals[i] = 1000.0 + rng.NormFloat64()*5.0
		}
		pts := makeSyntheticPoints(100, 10, vals)
		assert.Equal(t, observerdef.STABLE, classifyMetricKind(pts, &ws))
	})

	t.Run("positive and negative linear ramps are LINEAR", func(t *testing.T) {
		rng := rand.New(rand.NewSource(11))
		pos := make([]float64, 50)
		neg := make([]float64, 50)
		for i := range pos {
			pos[i] = 100.0 + float64(i)*8.0 + rng.NormFloat64()*1.2
			neg[i] = 900.0 - float64(i)*6.5 + rng.NormFloat64()*1.0
		}
		posPts := makeSyntheticPoints(100, 15, pos)
		negPts := makeSyntheticPoints(100, 15, neg)
		assert.Equal(t, observerdef.LINEAR, classifyMetricKind(posPts, &ws))
		assert.Equal(t, observerdef.LINEAR, classifyMetricKind(negPts, &ws))
	})

	t.Run("irregular timestamps preserve true linear slope", func(t *testing.T) {
		pts := make([]observerdef.Point, 40)
		var ts int64 = 1000
		for i := range pts {
			gap := int64(5 + (i%4)*5)
			ts += gap
			pts[i] = observerdef.Point{
				Timestamp: ts,
				Value:     50.0 + 0.75*float64(ts-1000),
			}
		}
		assert.Equal(t, observerdef.LINEAR, classifyMetricKind(pts, &ws))
	})

	t.Run("step jump and sawtooth reset are UNKNOWN", func(t *testing.T) {
		step := make([]float64, 40)
		for i := range step {
			if i < 20 {
				step[i] = 100.0 + float64(i%2)*0.2
			} else {
				step[i] = 250.0 + float64(i%2)*0.2
			}
		}
		stepPts := makeSyntheticPoints(1, 10, step)
		assert.Equal(t, observerdef.UNKNOWN, classifyMetricKind(stepPts, &ws))

		saw := make([]float64, 40)
		for i := range saw {
			if i < 25 {
				saw[i] = 100.0 + float64(i)*10.0
			} else {
				saw[i] = 100.0 + float64(i-25)*10.0
			}
		}
		sawPts := makeSyntheticPoints(1, 10, saw)
		assert.Equal(t, observerdef.UNKNOWN, classifyMetricKind(sawPts, &ws))
	})

	t.Run("large oscillation is UNKNOWN", func(t *testing.T) {
		osc := make([]float64, 48)
		for i := range osc {
			osc[i] = 100.0 + 45.0*math.Sin(float64(i)*0.5)
		}
		oscPts := makeSyntheticPoints(1, 10, osc)
		assert.Equal(t, observerdef.UNKNOWN, classifyMetricKind(oscPts, &ws))
	})
}

func TestMetricKindMatcher_SpikesAndPlateaus(t *testing.T) {
	classify := func(t *testing.T, values []float64, want observerdef.MetricKind) {
		t.Helper()
		points := makeSyntheticPoints(1, 1, values)
		tr, changed := NewMetricKindMatcher(DefaultMetricKindMatcherConfig()).Observe(1, observerdef.UNSET, points, 30)
		require.True(t, changed)
		require.Equal(t, want, tr.To)
	}
	for _, shape := range []struct {
		name  string
		slope float64
		spike float64
		want  observerdef.MetricKind
	}{
		{"plateau", 0, 40, observerdef.STABLE},
		{"positive ramp", 8, 80, observerdef.LINEAR},
		{"negative ramp", -8, 80, observerdef.LINEAR},
	} {
		for _, positions := range [][]int{{0}, {15}, {29}, {2, 27}} {
			for _, sign := range []float64{-1, 1} {
				t.Run(fmt.Sprintf("%s/spikes=%v/sign=%g", shape.name, positions, sign), func(t *testing.T) {
					values := make([]float64, 30)
					for i := range values {
						values[i] = 100 + shape.slope*float64(i) + 0.5*float64(i%3-1)
					}
					for _, i := range positions {
						values[i] += sign * shape.spike
					}
					classify(t, values, shape.want)
				})
			}
		}
	}

	t.Run("zero-MAD plateau with isolated spikes", func(t *testing.T) {
		for _, positions := range [][]int{{0}, {15}, {29}, {3, 14, 26}} {
			for _, spike := range []float64{-100, 100} {
				t.Run(fmt.Sprintf("spikes=%v/size=%g", positions, spike), func(t *testing.T) {
					values := make([]float64, 30)
					for i := range values {
						values[i] = 100
					}
					for _, i := range positions {
						values[i] += spike
					}
					// Includes the exact 29 values at 100 followed by 200 regression.
					classify(t, values, observerdef.STABLE)
				})
			}
		}
	})
	t.Run("noisy ramps with isolated spikes", func(t *testing.T) {
		for _, slope := range []float64{-8, 8} {
			for _, positions := range [][]int{{0}, {15}, {29}, {3, 14, 26}} {
				for _, spike := range []float64{-80, 80} {
					t.Run(fmt.Sprintf("slope=%g/spikes=%v/size=%g", slope, positions, spike), func(t *testing.T) {
						rng := rand.New(rand.NewSource(59507))
						values := make([]float64, 30)
						for i := range values {
							values[i] = 400 + slope*float64(i) + 1.5*rng.NormFloat64()
						}
						for _, i := range positions {
							values[i] += spike
						}
						classify(t, values, observerdef.LINEAR)
					})
				}
			}
		}
	})
	t.Run("jagged plateau with frequent small peaks", func(t *testing.T) {
		offsets := []float64{0, 0, 1, 0, 3, 0, 1, 3, 0, 0, 2, 1, 0, 2, 2, 0, 0, 0, 1, 0, 2, 3, 0, 3, 0, 2, 1, 1, 2, 0}
		values := make([]float64, len(offsets))
		for i, offset := range offsets {
			values[i] = 100 + offset
		}
		classify(t, values, observerdef.STABLE)
	})
	t.Run("negative and zero-centered low-MAD plateaus", func(t *testing.T) {
		for _, level := range []float64{-100, 0} {
			values := make([]float64, 30)
			for i := range values {
				values[i] = level + 0.01*float64(i%3-1)
			}
			values[29] += 0.4
			classify(t, values, observerdef.STABLE)
		}
	})
	t.Run("irregularly sampled plateau with endpoint spikes", func(t *testing.T) {
		points := make([]observerdef.Point, 30)
		// Keep timestamps large to catch rounding from using absolute float times.
		ts := int64(1 << 60)
		for i := range points {
			ts += int64(1 + 10*(i%4))
			points[i] = observerdef.Point{Timestamp: ts, Value: 100 + 0.5*float64(i%3-1)}
		}
		points[0].Value += 40
		points[29].Value -= 40
		tr, changed := NewMetricKindMatcher(DefaultMetricKindMatcherConfig()).Observe(1, observerdef.UNSET, points, ts)
		require.True(t, changed)
		require.Equal(t, observerdef.STABLE, tr.To)
	})
	t.Run("low MAD does not hide a genuine trend", func(t *testing.T) {
		values := make([]float64, 30)
		for i := range values {
			values[i] = 100 + float64(i)
		}
		classify(t, values, observerdef.LINEAR)
	})
	t.Run("persistent level shifts are not plateaus", func(t *testing.T) {
		for _, start := range []int{2, 5, 15, 25, 28} {
			for _, shift := range []float64{-10, 10} {
				t.Run(fmt.Sprintf("start=%d/shift=%g", start, shift), func(t *testing.T) {
					values := make([]float64, 30)
					for i := range values {
						values[i] = 100
						if i >= start {
							values[i] += shift
						}
					}
					classify(t, values, observerdef.UNKNOWN)
				})
			}
		}
	})
	t.Run("high MAD oscillation remains unsupported", func(t *testing.T) {
		values := make([]float64, 30)
		for i := range values {
			values[i] = 80 + 40*float64(i%2)
		}
		classify(t, values, observerdef.UNKNOWN)
	})
}

type matcherEvalCase struct {
	family string
	want   observerdef.MetricKind
	points []observerdef.Point
}

// The supported families have settled shapes. Mixed-regime families test
// abstention, not how quickly an assigned kind should transition.
func matcherEvalCorpus(firstSeed, lastSeed int64) []matcherEvalCase {
	families := []struct {
		name string
		kind observerdef.MetricKind
	}{
		{"constant", observerdef.STABLE},
		{"jitter", observerdef.STABLE},
		{"spiky_plateau", observerdef.STABLE},
		{"positive_ramp", observerdef.LINEAR},
		{"negative_ramp", observerdef.LINEAR},
		{"irregular_ramp", observerdef.LINEAR},
		{"noisy_ramp", observerdef.LINEAR},
		{"step", observerdef.UNKNOWN},
		{"hinge", observerdef.UNKNOWN},
		{"sawtooth", observerdef.UNKNOWN},
		{"oscillation", observerdef.UNKNOWN},
		{"random_walk", observerdef.UNKNOWN},
	}
	var cases []matcherEvalCase
	for seed := firstSeed; seed <= lastSeed; seed++ {
		for _, n := range []int{30, 60, 120} {
			for familyID, family := range families {
				rng := rand.New(rand.NewSource(seed*1000 + int64(n*20+familyID)))
				points := make([]observerdef.Point, n)
				ts := int64(100)
				for i := range points {
					gap := int64(10)
					if family.name == "irregular_ramp" {
						gap = int64(1 + rng.Intn(30))
					}
					ts += gap
					points[i].Timestamp = ts
				}
				// Change scale and sign without changing the intended family.
				scale := math.Pow(10, float64(seed%5)-2)
				level := 100 * scale
				if seed%2 == 0 {
					level = -level
				}
				walk := 0.0
				for i := range points {
					x := float64(i) / float64(n-1)
					noise := rng.NormFloat64()
					v := level
					switch family.name {
					case "jitter":
						v += scale * noise
					case "spiky_plateau":
						v += scale * noise
						if i == n/4 || i == 3*n/4 {
							v += 15 * scale
						}
					case "positive_ramp":
						v += scale * (80*x + 0.5*noise)
					case "negative_ramp":
						v += scale * (-80*x + 0.5*noise)
					case "irregular_ramp":
						elapsed := float64(points[i].Timestamp - points[0].Timestamp)
						duration := float64(points[n-1].Timestamp - points[0].Timestamp)
						v += scale * (80*elapsed/duration + 0.5*noise)
					case "noisy_ramp":
						v += scale * (80*x + 5*noise)
					case "step":
						v += scale * noise
						if i >= n/2 {
							v += 40 * scale
						}
					case "hinge":
						v += scale * (5*x + 100*math.Max(x-0.5, 0) + 0.5*noise)
					case "sawtooth":
						v += scale * (40*math.Mod(4*x, 1) + 0.5*noise)
					case "oscillation":
						v += scale * (25*math.Sin(8*math.Pi*x) + noise)
					case "random_walk":
						walk += 3 * noise
						v += scale * walk
					}
					points[i].Value = v
				}
				cases = append(cases, matcherEvalCase{family.name, family.kind, points})
			}
		}
	}
	return cases
}

type matcherEvalMetrics struct {
	confusion [4][4]int
}

func (m *matcherEvalMetrics) add(want, got observerdef.MetricKind) {
	m.confusion[want][got]++
}

func (m matcherEvalMetrics) summary() string {
	total, correct, supported := 0, 0, 0
	macroF1 := 0.0
	for want := observerdef.UNKNOWN; want <= observerdef.LINEAR; want++ {
		tp, fp, fn := m.confusion[want][want], 0, 0
		for got := observerdef.UNKNOWN; got <= observerdef.LINEAR; got++ {
			total += m.confusion[want][got]
			if got == observerdef.STABLE || got == observerdef.LINEAR {
				supported += m.confusion[want][got]
			}
			if got != want {
				fn += m.confusion[want][got]
				fp += m.confusion[got][want]
			}
		}
		correct += tp
		if denominator := 2*tp + fp + fn; denominator > 0 {
			macroF1 += 2 * float64(tp) / float64(denominator)
		}
	}
	return fmt.Sprintf("n=%d accuracy=%.4f macro_f1=%.4f supported_coverage=%.4f confusion[UNKNOWN,STABLE,LINEAR]=%v",
		total, float64(correct)/float64(total), macroF1/3,
		float64(supported)/float64(total), m.confusion)
}

func TestMetricKindClassifierEvaluation(t *testing.T) {
	for _, split := range []struct {
		name        string
		first, last int64
	}{
		{"training", 1, 20},
		{"heldout", 101, 120},
	} {
		cases := matcherEvalCorpus(split.first, split.last)
		t.Run(split.name, func(t *testing.T) {
			var ws matcherWorkspace
			var aggregate matcherEvalMetrics
			perFamily := make(map[string]*matcherEvalMetrics)
			var familyOrder []string
			for _, tc := range cases {
				got := classifyMetricKind(tc.points, &ws)
				if got < observerdef.UNKNOWN || got > observerdef.LINEAR {
					t.Fatalf("%s: invalid classified kind %v", tc.family, got)
				}
				aggregate.add(tc.want, got)
				if perFamily[tc.family] == nil {
					perFamily[tc.family] = &matcherEvalMetrics{}
					familyOrder = append(familyOrder, tc.family)
				}
				perFamily[tc.family].add(tc.want, got)
			}
			t.Log(aggregate.summary())
			for _, family := range familyOrder {
				t.Logf("%s %s", family, perFamily[family].summary())
			}
		})
	}
}

// These windows intentionally mix regimes; report them separately from settled
// shape accuracy because the slope-change magnitude controls model suitability.
func TestMetricKindClassifierMixedWindows(t *testing.T) {
	var ws matcherWorkspace
	for _, position := range []float64{0.2, 0.5, 0.8} {
		for _, ratio := range []float64{1.5, 3, 10} {
			points := make([]observerdef.Point, 120)
			for i := range points {
				x := float64(i) / float64(len(points)-1)
				points[i] = observerdef.Point{
					Timestamp: int64(i + 1),
					Value:     100 + 20*x + 20*(ratio-1)*math.Max(x-position, 0),
				}
			}
			t.Logf("hinge position=%.1f slope_ratio=%.1f kind=%s",
				position, ratio, classifyMetricKind(points, &ws))
		}
	}
}

func BenchmarkMetricKindClassifier(b *testing.B) {
	cases := matcherEvalCorpus(101, 101)
	for _, n := range []int{30, 60, 120} {
		b.Run(fmt.Sprintf("points=%d", n), func(b *testing.B) {
			var selected []matcherEvalCase
			for _, tc := range cases {
				if len(tc.points) == n {
					selected = append(selected, tc)
				}
			}
			var ws matcherWorkspace
			for _, tc := range selected {
				classifyMetricKind(tc.points, &ws)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				classifyMetricKind(selected[i%len(selected)].points, &ws)
			}
		})
	}
}

// BenchmarkMetricKindMatcher measures a full advance across all series:
// window selection, freshness, classification, confirmation state and caller-side
// assignment. Input preparation and construction are untimed; storage/engine
// work is excluded. Cold assignment includes Reset and candidate allocations.
func BenchmarkMetricKindMatcher(b *testing.B) {
	for _, window := range []int{30, 120} {
		for _, numSeries := range []int{5, 200, 2000} {
			for _, input := range []string{"simple", "noisy"} {
				for _, mode := range []string{"initial_assignment", "fresh_data", "unchanged_data", "transitions"} {
					b.Run(fmt.Sprintf("points=%d/series=%d/input=%s/%s", window, numSeries, input, mode), func(b *testing.B) {
						kinds := []observerdef.MetricKind{observerdef.STABLE, observerdef.LINEAR, observerdef.UNKNOWN}
						windows := make([][]observerdef.Point, len(kinds))
						for shape := range windows {
							rng := rand.New(rand.NewSource(int64(window*10 + shape)))
							windows[shape] = make([]observerdef.Point, window+1)
							for j := 0; j < window; j++ {
								value := 1000.0 + float64(j%2)
								if shape == 1 {
									value = 100 + float64(j)
								} else if shape == 2 {
									value = 100
									if j >= window/2 {
										value = 250
									}
								}
								if input == "noisy" {
									value += rng.NormFloat64()
								}
								windows[shape][j] = observerdef.Point{Timestamp: int64(j + 1), Value: value}
							}
							// Preloaded future data must not affect classification.
							windows[shape][window] = observerdef.Point{Timestamp: math.MaxInt64, Value: math.NaN()}
						}
						cfg := DefaultMetricKindMatcherConfig()
						cfg.MaxPoints = window
						matcher := NewMetricKindMatcher(cfg)
						current := make([]observerdef.MetricKind, numSeries)
						for series := range current {
							tr, changed := matcher.Observe(observerdef.SeriesRef(series+1), observerdef.UNSET,
								windows[series%len(kinds)], int64(window))
							if !changed || tr.To != kinds[series%len(kinds)] {
								b.Fatalf("unexpected warmup assignment: %+v, changed=%v", tr, changed)
							}
							current[series] = tr.To
						}

						transitions := 0
						b.ReportAllocs()
						b.ResetTimer()
						for advance := 0; advance < b.N; advance++ {
							now := int64(window)
							if mode != "unchanged_data" {
								b.StopTimer()
								now += int64(advance + 1)
								for _, points := range windows {
									for j := 0; j < window; j++ {
										points[j].Timestamp = now - int64(window-j-1)
									}
								}
								b.StartTimer()
							}
							if mode == "initial_assignment" {
								matcher.Reset()
								clear(current)
							}
							for series := range current {
								shape := series % len(kinds)
								if mode == "transitions" {
									// Each new prediction persists for three fresh advances.
									shape = (shape + 1 + advance/cfg.TransitionConfirmations) % len(kinds)
								}
								tr, changed := matcher.Observe(observerdef.SeriesRef(series+1), current[series], windows[shape], now)
								if changed {
									current[series] = tr.To
									transitions++
								}
							}
						}
						b.StopTimer()
						wantTransitions := 0
						switch mode {
						case "initial_assignment":
							wantTransitions = b.N * numSeries
						case "transitions":
							wantTransitions = (b.N / cfg.TransitionConfirmations) * numSeries
						}
						if transitions != wantTransitions {
							b.Fatalf("got %d transitions, want %d", transitions, wantTransitions)
						}
						b.ReportMetric(float64(numSeries), "series/op")
						b.ReportMetric(float64(transitions)/float64(b.N), "transitions/op")
					})
				}
			}
		}
	}
}

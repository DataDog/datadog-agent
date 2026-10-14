// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testFrom          = time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	testWindow        = Window{From: testFrom, To: testFrom.Add(20 * time.Minute), Block: 5 * time.Minute}
	testSwappedWindow = Window{From: testFrom.Add(time.Hour), To: testFrom.Add(80 * time.Minute), Block: 5 * time.Minute}
	// 5 millicores, as a CPU usage in nanocores
	testThresholds = Thresholds{Relative: 0.05, Absolute: 5e6}
)

// testPoints returns a point per minute of window.
func testPoints(window Window, value func(minute int) float64) []Point {
	var points []Point
	for minute := 0; window.From.Add(time.Duration(minute) * time.Minute).Before(window.To); minute++ {
		points = append(points, Point{Timestamp: window.From.Add(time.Duration(minute) * time.Minute), Value: value(minute)})
	}
	return points
}

// usage is a CPU usage of mean millicores, with a deterministic noise depending on seed.
func usage(millicores, seed float64) func(int) float64 {
	return func(minute int) float64 {
		return millicores * 1e6 * (1 + 0.03*math.Sin(1.3*float64(minute)+seed))
	}
}

// testPhases returns the phases of the measurement of images using baseline and comparison
// millicores, the node pool of the comparison installation making any image use bias times
// as much CPU as the node pool of the baseline installation.
func testPhases(baseline, comparison, bias float64) (Phase, Phase) {
	first := Phase{
		Window:     testWindow,
		Baseline:   testPoints(testWindow, usage(baseline, 0)),
		Comparison: testPoints(testWindow, usage(comparison*bias, 1)),
	}
	swapped := Phase{
		Window:     testSwappedWindow,
		Baseline:   testPoints(testSwappedWindow, usage(baseline*bias, 2)),
		Comparison: testPoints(testSwappedWindow, usage(comparison, 3)),
	}
	return first, swapped
}

func TestComparePoolBias(t *testing.T) {
	for _, bias := range []float64{0.8, 1, 1.2, 1.5} {
		t.Run(fmt.Sprintf("node pool bias %g", bias), func(t *testing.T) {
			t.Run("no change", func(t *testing.T) {
				first, swapped := testPhases(100, 100, bias)
				c, err := Compare(first, swapped, testThresholds)
				require.NoError(t, err)
				assert.Equal(t, VerdictUnchanged, c.Verdict)
				assert.InDelta(t, 0, c.RelativeChange, 0.01)
				// Each image ran once on each node pool.
				assert.InDelta(t, (1+bias)/2*100e6, c.BaselineMean, 1e6)
				assert.InDelta(t, (1+bias)/2*100e6, c.ComparisonMean, 1e6)
				// The bias of the node pools shows in each phase, with opposite signs.
				require.Len(t, c.Phases, 2)
				assert.InDelta(t, math.Log(bias), c.Phases[0].LogRatio, 0.01)
				assert.InDelta(t, -math.Log(bias), c.Phases[1].LogRatio, 0.01)
			})

			t.Run("improvement", func(t *testing.T) {
				first, swapped := testPhases(100, 90, bias)
				c, err := Compare(first, swapped, testThresholds)
				require.NoError(t, err)
				assert.Equal(t, VerdictImprovement, c.Verdict)
				assert.InDelta(t, -0.1, c.RelativeChange, 0.01)
				assert.Less(t, c.PValue, 0.001)
			})

			t.Run("regression", func(t *testing.T) {
				first, swapped := testPhases(100, 120, bias)
				c, err := Compare(first, swapped, testThresholds)
				require.NoError(t, err)
				assert.Equal(t, VerdictRegression, c.Verdict)
				assert.InDelta(t, 0.2, c.RelativeChange, 0.01)
				assert.Less(t, c.PValue, 0.001)
			})
		})
	}
}

func TestCompare(t *testing.T) {
	alternating := func(millicores float64) func(int) float64 {
		// Block means alternating between half and 1.5 times millicores
		return func(minute int) float64 {
			if minute/5%2 == 0 {
				return 1.5 * millicores * 1e6
			}
			return 0.5 * millicores * 1e6
		}
	}

	tests := []struct {
		name        string
		phases      func() (Phase, Phase)
		verdict     Verdict
		change      float64
		significant bool
		err         string
	}{
		{
			name:        "significant change under the relative threshold",
			phases:      func() (Phase, Phase) { return testPhases(500, 515, 1.2) },
			verdict:     VerdictUnchanged,
			change:      0.03,
			significant: true,
		},
		{
			name:        "significant change under the absolute threshold",
			phases:      func() (Phase, Phase) { return testPhases(2, 3, 1.2) },
			verdict:     VerdictUnchanged,
			change:      0.5,
			significant: true,
		},
		{
			name: "change not significant",
			phases: func() (Phase, Phase) {
				first, swapped := testPhases(100, 106, 1)
				// Opposite fluctuations of the installations in the first phase
				first.Comparison = testPoints(testWindow, func(minute int) float64 { return 1.06 * alternating(100)(minute+5) })
				first.Baseline = testPoints(testWindow, alternating(100))
				return first, swapped
			},
			verdict: VerdictUnchanged,
			change:  0.06,
		},
		{
			name: "points outside the windows are ignored",
			phases: func() (Phase, Phase) {
				first, swapped := testPhases(100, 120, 1.2)
				first.Baseline = append(first.Baseline, Point{Timestamp: testWindow.From.Add(-time.Minute), Value: 1e12})
				swapped.Comparison = append(swapped.Comparison, Point{Timestamp: testSwappedWindow.To, Value: 1e12})
				return first, swapped
			},
			verdict:     VerdictRegression,
			change:      0.2,
			significant: true,
		},
		{
			name: "missing blocks in the swapped phase",
			phases: func() (Phase, Phase) {
				first, swapped := testPhases(100, 100, 1)
				swapped.Comparison = swapped.Comparison[:10]
				return first, swapped
			},
			err: "phase 2: comparison: no data between 2026-10-13T11:10:00Z and 2026-10-13T11:15:00Z\nno data between 2026-10-13T11:15:00Z and 2026-10-13T11:20:00Z",
		},
		{
			name: "no data in the first phase",
			phases: func() (Phase, Phase) {
				first, swapped := testPhases(100, 100, 1)
				first.Baseline = nil
				return first, swapped
			},
			err: "phase 1: baseline: no data between 2026-10-13T10:00:00Z",
		},
		{
			name: "window too short",
			phases: func() (Phase, Phase) {
				first, swapped := testPhases(100, 100, 1)
				swapped.Window.To = swapped.Window.From.Add(10 * time.Minute)
				return first, swapped
			},
			err: "phase 2: baseline: the window holds 2 block(s) of 5m0s, at least 3 are needed",
		},
		{
			name: "usage at zero",
			phases: func() (Phase, Phase) {
				first, swapped := testPhases(100, 100, 1)
				first.Comparison = testPoints(testWindow, func(int) float64 { return 0 })
				return first, swapped
			},
			err: "and the comparison (0) must be positive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, swapped := tt.phases()
			c, err := Compare(first, swapped, testThresholds)
			if tt.err != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.verdict, c.Verdict)
			assert.InDelta(t, tt.change, c.RelativeChange, 0.01)
			if tt.significant {
				assert.Less(t, c.PValue, SignificanceLevel)
			} else {
				assert.GreaterOrEqual(t, c.PValue, SignificanceLevel)
			}
		})
	}
}

func TestCompareShorterLastBlock(t *testing.T) {
	// 17 minutes: 3 full blocks and one of 2 minutes
	first, swapped := testPhases(100, 130, 1.1)
	first.Window.To = first.Window.From.Add(17 * time.Minute)

	c, err := Compare(first, swapped, testThresholds)
	require.NoError(t, err)
	assert.Equal(t, VerdictRegression, c.Verdict)
	assert.Equal(t, 4, c.Phases[0].Blocks)

	first.Comparison = first.Comparison[:15]
	_, err = Compare(first, swapped, testThresholds)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data between 2026-10-13T10:15:00Z and 2026-10-13T10:17:00Z")
}

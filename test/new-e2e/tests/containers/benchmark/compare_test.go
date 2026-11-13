// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmark

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testFrom = time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	// testTwoWindows are the windows of a measurement of 2 phases of 20 minutes, which the
	// benchmark made before it supported more phases.
	testTwoWindows    = testWindows(2, 20*time.Minute)
	testWindow        = testTwoWindows[0]
	testSwappedWindow = testTwoWindows[1]
	// 5 millicores, as a CPU usage in nanocores
	testThresholds = Thresholds{Relative: 0.05, Absolute: 5e6}
)

// testWindows returns the windows of phases measurement phases of duration, an hour apart,
// in blocks of 5 minutes.
func testWindows(phases int, duration time.Duration) []Window {
	windows := make([]Window, phases)
	for i := range windows {
		from := testFrom.Add(time.Duration(i) * time.Hour)
		windows[i] = Window{From: from, To: from.Add(duration), Block: 5 * time.Minute}
	}
	return windows
}

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

// testPhases returns the phases over windows of the measurement of images using baseline and
// comparison millicores, the node pool of installation b making any image use bias times as
// much CPU as the node pool of installation a, the installations swapping their images as in
// SwapInPhase.
func testPhases(windows []Window, baseline, comparison, bias float64) []Phase {
	return lo.Map(windows, func(window Window, i int) Phase {
		baselineBias, comparisonBias := SwapInPhase(i, 1.0, bias)
		return Phase{
			Window:     window,
			Baseline:   testPoints(window, usage(baseline*baselineBias, float64(2*i))),
			Comparison: testPoints(window, usage(comparison*comparisonBias, float64(2*i+1))),
		}
	})
}

// twoPhases returns the phases of testPhases over testTwoWindows.
func twoPhases(baseline, comparison, bias float64) []Phase {
	return testPhases(testTwoWindows, baseline, comparison, bias)
}

func TestSwapInPhase(t *testing.T) {
	for phase, expected := range [][2]string{{"a", "b"}, {"b", "a"}, {"a", "b"}, {"b", "a"}} {
		x, y := SwapInPhase(phase, "a", "b")
		assert.Equalf(t, expected, [2]string{x, y}, "phase %d", phase+1)
	}
}

func TestParsePhaseCount(t *testing.T) {
	for _, tt := range []struct {
		value  string
		phases int
		err    string
	}{
		{value: "2", phases: 2},
		{value: "4", phases: 4},
		{value: "6", phases: 6},
		{value: "0", err: "0 phase(s): a measurement needs an even number of phases, at least 2"},
		{value: "1", err: "1 phase(s): a measurement needs an even number of phases, at least 2"},
		{value: "3", err: "3 phase(s): a measurement needs an even number of phases, at least 2"},
		{value: "-2", err: "-2 phase(s): a measurement needs an even number of phases, at least 2"},
		{value: "four", err: `strconv.Atoi: parsing "four": invalid syntax`},
		{value: "", err: `strconv.Atoi: parsing "": invalid syntax`},
	} {
		t.Run(strconv.Quote(tt.value), func(t *testing.T) {
			phases, err := ParsePhaseCount(tt.value)
			if tt.err != "" {
				assert.EqualError(t, err, tt.err)
				assert.Zero(t, phases)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.phases, phases)
		})
	}
}

func TestCompareInvalidPhaseCount(t *testing.T) {
	for _, phases := range []int{0, 1, 3} {
		_, err := Compare(testPhases(testWindows(phases, 15*time.Minute), 100, 120, 1), testThresholds)
		assert.ErrorContainsf(t, err, fmt.Sprintf("%d phase(s)", phases), "%d phases", phases)
	}
}

func TestComparePoolBias(t *testing.T) {
	for _, design := range []struct {
		phases   int
		duration time.Duration
		blocks   int
	}{
		// The former default: 2 phases of 4 blocks
		{phases: 2, duration: 20 * time.Minute, blocks: 4},
		// The default: 4 phases of 3 blocks
		{phases: 4, duration: 15 * time.Minute, blocks: 3},
		{phases: 6, duration: 15 * time.Minute, blocks: 3},
	} {
		windows := testWindows(design.phases, design.duration)
		t.Run(fmt.Sprintf("%d phases of %s", design.phases, design.duration), func(t *testing.T) {
			for _, bias := range []float64{0.8, 1, 1.2, 1.5} {
				t.Run(fmt.Sprintf("node pool bias %g", bias), func(t *testing.T) {
					t.Run("no change", func(t *testing.T) {
						c, err := Compare(testPhases(windows, 100, 100, bias), testThresholds)
						require.NoError(t, err)
						assert.Equal(t, VerdictUnchanged, c.Verdict)
						assert.InDelta(t, 0, c.RelativeChange, 0.01)
						// Each image ran half of the phases on each node pool.
						assert.InDelta(t, (1+bias)/2*100e6, c.BaselineMean, 1e6)
						assert.InDelta(t, (1+bias)/2*100e6, c.ComparisonMean, 1e6)
						// The bias of the node pools shows in each phase, with alternating signs.
						require.Len(t, c.Phases, design.phases)
						for i, phase := range c.Phases {
							expected := math.Log(bias)
							if i%2 == 1 {
								expected = -expected
							}
							assert.InDeltaf(t, expected, phase.LogRatio, 0.01, "phase %d", i+1)
							assert.Equalf(t, design.blocks, phase.Blocks, "phase %d", i+1)
						}
					})

					for _, tt := range []struct {
						comparison float64
						verdict    Verdict
						change     float64
					}{
						{comparison: 90, verdict: VerdictImprovement, change: -0.1},
						{comparison: 120, verdict: VerdictRegression, change: 0.2},
					} {
						t.Run(string(tt.verdict), func(t *testing.T) {
							c, err := Compare(testPhases(windows, 100, tt.comparison, bias), testThresholds)
							require.NoError(t, err)
							assert.Equal(t, tt.verdict, c.Verdict)
							assert.InDelta(t, tt.change, c.RelativeChange, 0.01)
							assert.Less(t, c.PValue, 0.001)
						})
					}
				})
			}
		})
	}
}

// TestCompareTwoPhases verifies that the comparison over 2 phases is, to the bit, the one the
// benchmark made before it supported more phases: the averages of both phases, and Welch's
// t-test comparing the opposites of the block log ratios of the first phase to those of the
// swapped one.
func TestCompareTwoPhases(t *testing.T) {
	for _, tt := range []struct {
		comparison, bias float64
	}{
		{comparison: 100, bias: 1},
		{comparison: 101, bias: 1.15},
		{comparison: 103, bias: 0.9},
		{comparison: 120, bias: 1.2},
	} {
		t.Run(fmt.Sprintf("comparison %g, node pool bias %g", tt.comparison, tt.bias), func(t *testing.T) {
			phases := twoPhases(100, tt.comparison, tt.bias)
			c, err := Compare(phases, testThresholds)
			require.NoError(t, err)

			first, firstRatios, err := comparePhase(phases[0])
			require.NoError(t, err)
			swapped, swappedRatios, err := comparePhase(phases[1])
			require.NoError(t, err)
			assert.Equal(t, []PhaseComparison{first, swapped}, c.Phases)
			assert.Equal(t, (first.BaselineMean+swapped.BaselineMean)/2, c.BaselineMean)
			assert.Equal(t, (first.ComparisonMean+swapped.ComparisonMean)/2, c.ComparisonMean)
			assert.Equal(t, math.Expm1((first.LogRatio+swapped.LogRatio)/2), c.RelativeChange)
			assert.Equal(t, welchTTest(negated(firstRatios), swappedRatios), c.PValue)
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
	fourWindows := testWindows(4, 15*time.Minute)

	tests := []struct {
		name        string
		phases      func() []Phase
		verdict     Verdict
		change      float64
		significant bool
		err         string
	}{
		{
			name:        "significant change under the relative threshold",
			phases:      func() []Phase { return twoPhases(500, 515, 1.2) },
			verdict:     VerdictUnchanged,
			change:      0.03,
			significant: true,
		},
		{
			name:        "significant change under the absolute threshold",
			phases:      func() []Phase { return twoPhases(2, 3, 1.2) },
			verdict:     VerdictUnchanged,
			change:      0.5,
			significant: true,
		},
		{
			name: "change not significant",
			phases: func() []Phase {
				phases := twoPhases(100, 106, 1)
				// Opposite fluctuations of the installations in the first phase
				phases[0].Comparison = testPoints(testWindow, func(minute int) float64 { return 1.06 * alternating(100)(minute+5) })
				phases[0].Baseline = testPoints(testWindow, alternating(100))
				return phases
			},
			verdict: VerdictUnchanged,
			change:  0.06,
		},
		{
			name: "change not significant in 4 phases",
			phases: func() []Phase {
				phases := testPhases(fourWindows, 100, 106, 1)
				// Block log ratios of the third phase far apart from one another, of mean ln(1.06)
				factors := []float64{3, 1.0 / 3, 1}
				phases[2].Baseline = testPoints(fourWindows[2], func(int) float64 { return 100e6 })
				phases[2].Comparison = testPoints(fourWindows[2], func(minute int) float64 { return 106e6 * factors[minute/5] })
				return phases
			},
			verdict: VerdictUnchanged,
			change:  0.06,
		},
		{
			name:        "significant change in 4 phases",
			phases:      func() []Phase { return testPhases(fourWindows, 100, 108, 1.2) },
			verdict:     VerdictRegression,
			change:      0.08,
			significant: true,
		},
		{
			name: "points outside the windows are ignored",
			phases: func() []Phase {
				phases := twoPhases(100, 120, 1.2)
				phases[0].Baseline = append(phases[0].Baseline, Point{Timestamp: testWindow.From.Add(-time.Minute), Value: 1e12})
				phases[1].Comparison = append(phases[1].Comparison, Point{Timestamp: testSwappedWindow.To, Value: 1e12})
				return phases
			},
			verdict:     VerdictRegression,
			change:      0.2,
			significant: true,
		},
		{
			name: "missing blocks in the swapped phase",
			phases: func() []Phase {
				phases := twoPhases(100, 100, 1)
				phases[1].Comparison = phases[1].Comparison[:10]
				return phases
			},
			err: "phase 2: comparison: no data between 2026-10-13T11:10:00Z and 2026-10-13T11:15:00Z\nno data between 2026-10-13T11:15:00Z and 2026-10-13T11:20:00Z",
		},
		{
			name: "missing block in the fourth phase",
			phases: func() []Phase {
				phases := testPhases(fourWindows, 100, 100, 1)
				phases[3].Baseline = phases[3].Baseline[5:]
				return phases
			},
			err: "phase 4: baseline: no data between 2026-10-13T13:00:00Z and 2026-10-13T13:05:00Z",
		},
		{
			name: "no data in the first phase",
			phases: func() []Phase {
				phases := twoPhases(100, 100, 1)
				phases[0].Baseline = nil
				return phases
			},
			err: "phase 1: baseline: no data between 2026-10-13T10:00:00Z",
		},
		{
			name: "window too short",
			phases: func() []Phase {
				phases := twoPhases(100, 100, 1)
				phases[1].Window.To = phases[1].Window.From.Add(10 * time.Minute)
				return phases
			},
			err: "phase 2: baseline: the window holds 2 block(s) of 5m0s, at least 3 are needed",
		},
		{
			name: "usage at zero",
			phases: func() []Phase {
				phases := twoPhases(100, 100, 1)
				phases[0].Comparison = testPoints(testWindow, func(int) float64 { return 0 })
				return phases
			},
			err: "and the comparison (0) must be positive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Compare(tt.phases(), testThresholds)
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
	phases := twoPhases(100, 130, 1.1)
	phases[0].Window.To = phases[0].Window.From.Add(17 * time.Minute)

	c, err := Compare(phases, testThresholds)
	require.NoError(t, err)
	assert.Equal(t, VerdictRegression, c.Verdict)
	assert.Equal(t, 4, c.Phases[0].Blocks)

	phases[0].Comparison = phases[0].Comparison[:15]
	_, err = Compare(phases, testThresholds)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data between 2026-10-13T10:15:00Z and 2026-10-13T10:17:00Z")
}

func TestCompareNonFinite(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1)} {
		phases := twoPhases(100, 130, 1.1)
		phases[0].Comparison[0].Value = value
		_, err := Compare(phases, testThresholds)
		require.Errorf(t, err, "value %g", value)
		assert.Contains(t, err.Error(), "must be positive and finite")
	}
}

func TestResultCompareAfterFailure(t *testing.T) {
	result := Result{Thresholds: testThresholds}
	result.Fail(errors.New("no data"))

	// A successful retry replaces the error.
	result.Compare(twoPhases(100, 130, 1.1))
	assert.Equal(t, VerdictRegression, result.Verdict)
	assert.Empty(t, result.Error)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package vbr

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// feedAll runs every point through Update, then force-closes the trailing
// segment with a single FlushWindow, and returns every breakpoint emitted,
// in order.
func feedAll(c *Compressor, points []Point) []Point {
	var out []Point
	for _, p := range points {
		out = append(out, c.Update(p.Ts, p.Value)...)
	}
	out = append(out, c.FlushWindow(points[len(points)-1].Ts)...)
	return out
}

// expectedTolerances mirrors updateScaleAndTolerance so tests can compute,
// independently of the implementation under test, the tolerance that should
// have applied to each input sample.
func expectedTolerances(points []Point, cfg Config) []float64 {
	tols := make([]float64, len(points))
	var scale float64
	var hasScale bool
	for i, p := range points {
		abs := math.Abs(p.Value)
		if !hasScale {
			scale = abs
			hasScale = true
		} else {
			scale = cfg.Alpha*abs + (1-cfg.Alpha)*scale
		}
		tol := cfg.Epsilon * scale
		if tol < cfg.Floor {
			tol = cfg.Floor
		}
		tols[i] = tol
	}
	return tols
}

// reconstruct linearly interpolates the breakpoint series at t. Breakpoints
// must be sorted by Ts and t must fall within [breakpoints[0].Ts, last.Ts].
func reconstruct(t testing.TB, breakpoints []Point, at float64) float64 {
	require.NotEmpty(t, breakpoints)
	if len(breakpoints) == 1 {
		return breakpoints[0].Value
	}
	for i := 0; i < len(breakpoints)-1; i++ {
		a, b := breakpoints[i], breakpoints[i+1]
		if at >= a.Ts && at <= b.Ts {
			if b.Ts == a.Ts {
				return a.Value
			}
			frac := (at - a.Ts) / (b.Ts - a.Ts)
			return a.Value + frac*(b.Value-a.Value)
		}
	}
	t.Fatalf("query time %v outside breakpoint range [%v, %v]", at, breakpoints[0].Ts, breakpoints[len(breakpoints)-1].Ts)
	return 0
}

// assertBoundedError checks the defining property of the compressor: every
// original sample must be reconstructable, from the emitted breakpoints
// alone, to within the tolerance that was in effect for that sample.
func assertBoundedError(t *testing.T, cfg Config, samples []Point) []Point {
	t.Helper()
	c := New(cfg)
	breakpoints := feedAll(c, samples)
	require.NotEmpty(t, breakpoints)

	tols := expectedTolerances(samples, cfg)
	const slack = 1e-9 // floating point slack only
	for i, s := range samples {
		got := reconstruct(t, breakpoints, s.Ts)
		diff := math.Abs(got - s.Value)
		require.LessOrEqualf(t, diff, tols[i]+slack,
			"sample %d (t=%v, v=%v): reconstructed %v, error %v exceeds tolerance %v",
			i, s.Ts, s.Value, got, diff, tols[i])
	}
	return breakpoints
}

func testConfig() Config {
	return Config{Epsilon: 0.05, Alpha: 0.3, Floor: 0.5, Warmup: 2}
}

func TestBoundedError_FlatRun(t *testing.T) {
	var samples []Point
	for i := 0; i < 30; i++ {
		samples = append(samples, Point{Ts: float64(i), Value: 42})
	}
	breakpoints := assertBoundedError(t, testConfig(), samples)
	// Nothing ever moves: only the warmup points plus the final flush
	// should ship.
	require.LessOrEqual(t, len(breakpoints), testConfig().Warmup+1)
}

func TestBoundedError_SmoothRamp(t *testing.T) {
	var samples []Point
	for i := 0; i < 30; i++ {
		samples = append(samples, Point{Ts: float64(i), Value: 10 + 2*float64(i)})
	}
	breakpoints := assertBoundedError(t, testConfig(), samples)
	// A perfectly linear signal should collapse to essentially a straight
	// line: warmup points plus the final flush, nothing in between.
	require.LessOrEqual(t, len(breakpoints), testConfig().Warmup+1)
}

func TestBoundedError_SharpSpike(t *testing.T) {
	var samples []Point
	for i := 0; i < 30; i++ {
		v := 100.0
		if i == 15 {
			v = 500.0 // one sample spikes far past tolerance
		}
		samples = append(samples, Point{Ts: float64(i), Value: v})
	}
	breakpoints := assertBoundedError(t, testConfig(), samples)

	found := false
	for _, bp := range breakpoints {
		if bp.Ts == 15 {
			found = true
		}
	}
	require.True(t, found, "expected the spike sample itself to be shipped as a breakpoint, got %+v", breakpoints)
	// Flat-before + spike + flat-after should still compress heavily
	// relative to the 30 raw samples.
	require.Less(t, len(breakpoints), 10)
}

func TestBoundedError_Step(t *testing.T) {
	var samples []Point
	for i := 0; i < 30; i++ {
		v := 100.0
		if i >= 15 {
			v = 300.0
		}
		samples = append(samples, Point{Ts: float64(i), Value: v})
	}
	breakpoints := assertBoundedError(t, testConfig(), samples)
	require.Less(t, len(breakpoints), 10)
}

func TestBoundedError_NearZeroJitter(t *testing.T) {
	var samples []Point
	jitter := []float64{0, 0.05, -0.05, 0.02, -0.02, 0.01, 0, -0.01, 0.03, -0.03}
	for i := 0; i < 30; i++ {
		samples = append(samples, Point{Ts: float64(i), Value: jitter[i%len(jitter)]})
	}
	breakpoints := assertBoundedError(t, testConfig(), samples)
	// Jitter is far below Floor, so it should compress heavily rather than
	// shipping a breakpoint for every tiny wiggle.
	require.Less(t, len(breakpoints), 10)
}

func TestBoundedError_RandomWalk(t *testing.T) {
	// A deterministic pseudo-random walk exercises many arbitrary
	// slope-cone transitions without relying on math/rand (which the
	// workflow sandbox disallows as a source of non-determinism, and which
	// isn't needed here since the sequence itself is fixed).
	var samples []Point
	v := 1000.0
	seed := uint64(88172645463325252)
	for i := 0; i < 200; i++ {
		// xorshift64
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		step := float64(int64(seed)%21-10) * 3 // roughly [-30, 30]
		v += step
		samples = append(samples, Point{Ts: float64(i), Value: v})
	}
	assertBoundedError(t, testConfig(), samples)
}

// TestSwingDoorCandidateMustMatchItsOwnSlope reproduces the counterexample
// that shows why "cone stays non-empty" is not sufficient to accept a
// candidate point: (0,0) -> (10,1) -> (20,5) -> (21,10) with a constant
// tolerance of 1. The naive swing-door rule (accept whenever the intersected
// cone is still non-empty, and later close using the last accepted point's
// real value) accepts (20,5) into the same segment as (10,1), then closes
// there when (21,10) arrives — producing a reconstruction that misses (10,1)
// by 1.5, 50% over tolerance. The fix requires a candidate's own slope from
// the anchor to already lie within the cone accumulated from strictly prior
// points, which forces (10,1) to be shipped as its own breakpoint instead.
func TestSwingDoorCandidateMustMatchItsOwnSlope(t *testing.T) {
	cfg := Config{Epsilon: 0, Alpha: 0, Floor: 1, Warmup: 1}
	c := New(cfg)

	require.Equal(t, []Point{{Ts: 0, Value: 0}}, c.Update(0, 0)) // warmup emits the anchor
	require.Empty(t, c.Update(10, 1))                            // opens the segment, no violation yet

	// (20,5) is inconsistent with (10,1)'s own trajectory from the anchor
	// even though the naive intersected cone would still be non-empty; it
	// must force (10,1) to close out as its own breakpoint.
	closed := c.Update(20, 5)
	require.Equal(t, []Point{{Ts: 10, Value: 1}}, closed)

	// And the reconstruction from here on (new anchor (10,1), pending
	// (20,5)) must itself respect tolerance for every fed sample.
	final := c.FlushWindow(20)
	require.Equal(t, []Point{{Ts: 20, Value: 5}}, final)
}

func TestFlushWindow_NoPendingEmitsNothing(t *testing.T) {
	c := New(testConfig()) // Warmup: 2
	require.Equal(t, []Point{{Ts: 0, Value: 1}}, c.Update(0, 1))
	require.Equal(t, []Point{{Ts: 1, Value: 1}}, c.Update(1, 1))
	// Both calls above were still warmup; nothing pending yet, so flush
	// must not synthesize or re-emit anything.
	require.Empty(t, c.FlushWindow(1))
}

func TestUpdate_SingleSampleWindow(t *testing.T) {
	c := New(Config{Epsilon: 0.05, Alpha: 0.3, Floor: 0.5, Warmup: 1})
	got := c.Update(0, 7)
	require.Equal(t, []Point{{Ts: 0, Value: 7}}, got)
	// Nothing else happened this window: flushing must not re-emit the
	// already-shipped anchor.
	require.Empty(t, c.FlushWindow(0))
}

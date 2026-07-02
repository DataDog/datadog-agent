// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package vbr

import (
	"fmt"
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

// xorshift64 is the same hand-rolled, fixed-seed PRNG used elsewhere in this
// file (see TestBoundedError_RandomWalk): deterministic across Go versions
// and architectures, unlike math/rand's algorithm choice, so a failure is
// always reproducible from the seed alone.
type xorshift64 struct{ state uint64 }

func newXorshift64(seed uint64) *xorshift64 {
	if seed == 0 {
		seed = 1 // xorshift64 is fixed at all-zero state; avoid it.
	}
	return &xorshift64{state: seed}
}

func (x *xorshift64) next() uint64 {
	x.state ^= x.state << 13
	x.state ^= x.state >> 7
	x.state ^= x.state << 17
	return x.state
}

// float01 returns a pseudo-random float64 in [0, 1).
func (x *xorshift64) float01() float64 {
	return float64(x.next()%1_000_000_000) / 1_000_000_000
}

// floatRange returns a pseudo-random float64 in [lo, hi).
func (x *xorshift64) floatRange(lo, hi float64) float64 {
	return lo + x.float01()*(hi-lo)
}

// intRange returns a pseudo-random int in [lo, hi] (inclusive).
func (x *xorshift64) intRange(lo, hi int) int {
	return lo + int(x.next()%uint64(hi-lo+1))
}

// The genXxxSignal functions each produce n raw values following a distinct
// shape, exercising different regions of the compressor's behavior (flat
// runs collapse hard, spikes force closes, random walks exercise arbitrary
// slope-cone transitions, ...). genMixedSignal stitches several of them
// together back-to-back to also exercise the transitions between shapes.

func genFlatSignal(r *xorshift64, n int) []float64 {
	v := r.floatRange(-1000, 1000)
	out := make([]float64, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func genRampSignal(r *xorshift64, n int) []float64 {
	v := r.floatRange(-1000, 1000)
	step := r.floatRange(-50, 50)
	out := make([]float64, n)
	for i := range out {
		out[i] = v
		v += step
	}
	return out
}

func genStepSignal(r *xorshift64, n int) []float64 {
	v := r.floatRange(-1000, 1000)
	out := make([]float64, n)
	if n < 2 {
		// No room for a step; fall back to flat.
		for i := range out {
			out[i] = v
		}
		return out
	}
	stepAt := r.intRange(1, n-1)
	jump := r.floatRange(-2000, 2000)
	for i := range out {
		if i == stepAt {
			v += jump
		}
		out[i] = v
	}
	return out
}

func genSpikeSignal(r *xorshift64, n int) []float64 {
	base := r.floatRange(-1000, 1000)
	out := make([]float64, n)
	for i := range out {
		out[i] = base
	}
	spikes := r.intRange(0, n/5+1)
	for k := 0; k < spikes; k++ {
		out[r.intRange(0, n-1)] = base + r.floatRange(-5000, 5000)
	}
	return out
}

func genJitterSignal(r *xorshift64, n int) []float64 {
	base := r.floatRange(-1000, 1000)
	amplitude := r.floatRange(0, 5)
	out := make([]float64, n)
	for i := range out {
		out[i] = base + r.floatRange(-amplitude, amplitude)
	}
	return out
}

func genRandomWalkSignal(r *xorshift64, n int) []float64 {
	v := r.floatRange(-1000, 1000)
	stepSize := r.floatRange(0, 50)
	out := make([]float64, n)
	for i := range out {
		out[i] = v
		v += r.floatRange(-stepSize, stepSize)
	}
	return out
}

var signalShapes = []struct {
	name string
	gen  func(*xorshift64, int) []float64
}{
	{"flat", genFlatSignal},
	{"ramp", genRampSignal},
	{"step", genStepSignal},
	{"spike", genSpikeSignal},
	{"jitter", genJitterSignal},
	{"randomWalk", genRandomWalkSignal},
}

func genMixedSignal(r *xorshift64, n int) []float64 {
	var out []float64
	for remaining := n; remaining > 0; {
		segLen := r.intRange(1, remaining)
		out = append(out, signalShapes[r.intRange(0, len(signalShapes)-1)].gen(r, segLen)...)
		remaining -= segLen
	}
	return out
}

// toPoints assigns strictly increasing, non-uniformly-spaced timestamps to
// values — non-uniform spacing exercises openSegment/evalCandidate's dt
// handling more thoroughly than the fixed 1-unit spacing every other test in
// this file uses. Strictly increasing (never equal) sidesteps the dt<=0
// re-anchor branch in openSegment/evalCandidate, a degenerate case with its
// own, separately-tested semantics (see TestFlushWindow_NoPendingEmitsNothing
// and friends) that isn't part of this property.
func toPoints(r *xorshift64, values []float64) []Point {
	pts := make([]Point, len(values))
	ts := 0.0
	for i, v := range values {
		if i > 0 {
			ts += r.floatRange(0.1, 3.0)
		}
		pts[i] = Point{Ts: ts, Value: v}
	}
	return pts
}

// TestBoundedError_PropertyFuzz is the generic guard for the whole class of
// bug TestSwingDoorCandidateMustMatchItsOwnSlope found by hand: instead of
// relying on a human to construct a counterexample sequence, generate many
// random signals across a wide range of configs and shapes, and assert the
// compressor's core guarantee (assertBoundedError) holds for every one of
// them. A fixed seed keeps failures reproducible; bumping the iteration
// count only ever adds coverage, it never changes which cases already ran.
func TestBoundedError_PropertyFuzz(t *testing.T) {
	const iterations = 300
	r := newXorshift64(0x9e3779b97f4a7c15)

	for iter := 0; iter < iterations; iter++ {
		cfg := Config{
			Epsilon: r.floatRange(0, 0.3),
			Alpha:   r.floatRange(0, 1),
			Floor:   r.floatRange(0, 2),
			Warmup:  r.intRange(1, 5),
		}
		n := r.intRange(5, 150)
		shapeName, gen := "mixed", genMixedSignal
		if useNamed := r.intRange(0, 1); useNamed == 1 {
			shape := signalShapes[r.intRange(0, len(signalShapes)-1)]
			shapeName, gen = shape.name, shape.gen
		}
		samples := toPoints(r, gen(r, n))

		t.Run(fmt.Sprintf("iter%d_%s_n%d", iter, shapeName, n), func(t *testing.T) {
			assertBoundedError(t, cfg, samples)
		})
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sdc implements a streaming, bounded-error Swinging Door
// Trending/Compression (SDC) compressor for per-context metric point
// streams, with an EWMA-smoothed adaptive tolerance: full granularity where
// the signal moves, a handful of points where it doesn't. It also holds
// (eligibility.go) the single source of truth for whether a check should
// get SDC applied and for the checks.sdc_compression_* tuning/behavior
// knobs, used by pkg/aggregator's CheckSampler-level compression hook.
//
// This is the Swinging Door Trending algorithm, originally described by
// Bristol in U.S. Patent 4,669,097 ("Data Compression for Display and
// Storage", filed 1985, issued May 26, 1987; expired since 2004 under the
// 17-year post-grant term that applied to patents of that era). The rest
// of this file refers to it as "the algorithm" rather than "the patent" —
// it's long since become a standard, widely-implemented technique, not a
// live IP concern. It follows the algorithm's own terminology: each open
// segment pivots two "doors" (upperDoorSlope, lowerDoorSlope — the
// algorithm's SU(MAX)/SL(MIN)) from the segment's first point, admitting
// successive points until the doors would cross, at which point the last
// inbounds point is issued as the segment's end and a new segment begins
// from there.
//
// One deliberate deviation from the algorithm: a closed segment's end
// point is the real, observed lastInBounds point (one of the algorithm's
// own alternatives, "choose the last inbounds point"), not a computed
// point along the crossing-out-point-plus-E/2 construction it uses by
// default. That choice has a correctness consequence, which is why
// Update's accept test compares a candidate's raw (un-widened) slope
// against the accumulated door cone, rather than the algorithm's literal
// test of comparing the candidate's own ±errorBound-widened SU(i)/SL(i)
// against the running SU(MAX)/SL(MIN) — see the comment above that
// comparison in Update for why the literal test is provably insufficient
// once the segment end point is a real, un-adjusted point.
package sdc

import "math"

// Point is a single (timestamp, value) sample or breakpoint.
type Point struct {
	Ts    float64
	Value float64
}

// Config holds the global (not per-series) compressor parameters.
type Config struct {
	// Epsilon is the relative precision: tolerance = max(Epsilon*scale, Floor).
	Epsilon float64
	// Alpha is the EWMA smoothing factor used to track the signal's scale.
	Alpha float64
	// Floor is a tiny absolute tolerance floor so an idle signal never gets
	// a zero-width tolerance.
	Floor float64
	// Warmup is the number of leading samples emitted verbatim (and used to
	// seed the scale estimate) before compression engages. Defaults to 1.
	Warmup int
}

func (c Config) warmup() int {
	if c.Warmup <= 0 {
		return 1
	}
	return c.Warmup
}

// Compressor is a single-pass, O(1)-per-sample streaming compressor for one
// metric context. It keeps only the current EWMA scale estimate and the
// state of the currently-open segment.
type Compressor struct {
	cfg Config

	scale    float64
	hasScale bool

	warmupRemaining int

	// first is the current segment's first point (the algorithm's "first
	// corridor end point" C(i)), from which the two doors pivot.
	hasFirst bool
	first    Point

	// lastInBounds is the most recent point admitted into the current
	// segment (the algorithm's "last inbounds point") — the candidate for
	// the segment's end point once a later point forces the doors to cross.
	hasLastInBounds bool
	lastInBounds    Point
	// upperDoorSlope and lowerDoorSlope are the algorithm's SU(MAX)/SL(MIN):
	// the most extreme slope each door has had to swing to since first,
	// narrowing the admissible slope range as more points are folded in.
	upperDoorSlope float64
	lowerDoorSlope float64
}

// New returns a Compressor for one metric context, using cfg for all series.
func New(cfg Config) *Compressor {
	return &Compressor{cfg: cfg, warmupRemaining: cfg.warmup()}
}

// Update feeds one already-committed (timestamp, value) sample and returns
// any breakpoints the update closes. In steady state this is empty; it is
// non-empty exactly when the signal moved enough to require shipping a new
// point. Samples must be fed in non-decreasing Ts order.
func (c *Compressor) Update(ts, value float64) []Point {
	errorBound := c.updateScaleAndTolerance(value)

	if c.warmupRemaining > 0 {
		c.warmupRemaining--
		c.first = Point{Ts: ts, Value: value}
		c.hasFirst = true
		c.hasLastInBounds = false
		return []Point{{Ts: ts, Value: value}}
	}

	if !c.hasFirst {
		// Defensive: warmup with Warmup>=1 always sets the first point on
		// its last iteration, so this should not happen in practice.
		c.first = Point{Ts: ts, Value: value}
		c.hasFirst = true
		return nil
	}

	if !c.hasLastInBounds {
		c.establishPivots(ts, value, errorBound)
		return nil
	}

	cand := doorSlopes(c.first, ts, value, errorBound)

	// Why this compares cand.slope (raw, un-widened) rather than the
	// algorithm's literal test of cand.upperDoorSlope/cand.lowerDoorSlope
	// (SU(i)/SL(i)) against the running door bounds:
	//
	// The algorithm's literal test only proves that SOME slope within the
	// surviving [upperDoorSlope, lowerDoorSlope] cone would keep every
	// admitted point within errorBound — it doesn't constrain WHICH slope
	// gets used for reconstruction. The algorithm's own default
	// reconstruction (a computed crossing-out point, nudged by
	// errorBound/2 toward the center line) is free to land anywhere in
	// that cone, so the literal test is sufficient for it. This
	// implementation instead ships the real, observed lastInBounds point
	// as the segment end (one of the algorithm's own alternatives,
	// "choose the last inbounds point" — faster, and it preserves an
	// actual data point) — which means the reconstruction line's slope is
	// fixed to be exactly lastInBounds's own raw slope from first, not a
	// free choice from the cone.
	//
	// That makes the algorithm's literal test provably insufficient here: a
	// candidate can have its ±errorBound-widened range merely graze the
	// existing cone (technically keeping SU(MAX) <= SL(MIN)) while its own
	// raw, un-widened slope already sits outside it. If that candidate is
	// then accepted and later becomes lastInBounds, the straight line from
	// first to it can misrepresent an earlier point by well over
	// errorBound — see TestSwingDoorCandidateMustMatchItsOwnSlope for a
	// concrete, worked counterexample (misses an intermediate point by 50%
	// over tolerance). Requiring the candidate's raw slope to already lie
	// in the pre-update cone is what actually guarantees that whichever
	// point ends up shipped as lastInBounds has a slope from first
	// consistent with every other point swallowed by the segment, at the
	// full errorBound — not just with the two doors.
	if cand.feasible && cand.slope >= c.upperDoorSlope && cand.slope <= c.lowerDoorSlope {
		c.upperDoorSlope = math.Max(c.upperDoorSlope, cand.upperDoorSlope)
		c.lowerDoorSlope = math.Min(c.lowerDoorSlope, cand.lowerDoorSlope)
		c.lastInBounds = Point{Ts: ts, Value: value}
		return nil
	}

	// The doors would cross: close the segment at the last inbounds point
	// — the last point whose own trajectory from the first point was
	// consistent with the run — then open a fresh segment with this point
	// against that new first point.
	closed := c.lastInBounds
	c.first = closed
	c.hasLastInBounds = false
	c.establishPivots(ts, value, errorBound)
	return []Point{closed}
}

// FlushWindow force-closes the currently open segment, if any, and returns
// its closing point. The scale estimate and the first point (the returned
// point, or the previous first point if nothing was in bounds) carry
// forward into the next window unchanged.
func (c *Compressor) FlushWindow(_ float64) []Point {
	if !c.hasLastInBounds {
		return nil
	}
	closed := c.lastInBounds
	c.first = closed
	c.hasLastInBounds = false
	return []Point{closed}
}

// establishPivots starts a new segment from c.first: the algorithm's
// "establish pivot points" (Step 2) / "establish new offset points"
// (Step 8), computing the initial upper/lower door slopes toward (ts, value).
func (c *Compressor) establishPivots(ts, value, errorBound float64) {
	cand := doorSlopes(c.first, ts, value, errorBound)
	if !cand.feasible {
		// Same timestamp as the first point but outside tolerance: nothing
		// sensible to swing a door from; just restart from here.
		c.first = Point{Ts: ts, Value: value}
		c.hasLastInBounds = false
		return
	}
	c.upperDoorSlope, c.lowerDoorSlope = cand.upperDoorSlope, cand.lowerDoorSlope
	c.lastInBounds = Point{Ts: ts, Value: value}
	c.hasLastInBounds = true
}

// Scale returns the compressor's current EWMA estimate of the signal's
// magnitude (a smoothed |value|) — the basis for its tolerance (tolerance =
// max(Epsilon*Scale(), Floor)). Exposed for observability only (see
// pkg/aggregator's checksampler_sdc scale-deviation telemetry); the
// compressor's own correctness never depends on a caller reading this.
func (c *Compressor) Scale() float64 {
	return c.scale
}

// updateScaleAndTolerance returns the current errorBound (the algorithm's
// "error" / "error bound E") — here computed dynamically as
// max(Epsilon*EWMA-scale, Floor) rather than the algorithm's static,
// externally-supplied E.
func (c *Compressor) updateScaleAndTolerance(value float64) float64 {
	abs := math.Abs(value)
	if !c.hasScale {
		c.scale = abs
		c.hasScale = true
	} else {
		c.scale = c.cfg.Alpha*abs + (1-c.cfg.Alpha)*c.scale
	}
	errorBound := c.cfg.Epsilon * c.scale
	if errorBound < c.cfg.Floor {
		return c.cfg.Floor
	}
	return errorBound
}

// candidate holds a point's raw slope from the segment's first point, and
// the upper/lower door slopes (the algorithm's SU(i)/SL(i)) that pivoting
// each door from first to admit this point at ±errorBound would require.
type candidate struct {
	slope                          float64
	upperDoorSlope, lowerDoorSlope float64
	feasible                       bool
}

// doorSlopes computes the raw slope from first to (t, v), and SU(i)/SL(i):
// the interval of slopes from first that would keep (t, v) within
// errorBound. feasible is false only in the degenerate case where t ==
// first.Ts but v differs from first.Value by more than errorBound, which
// no line through first can satisfy.
func doorSlopes(first Point, t, v, errorBound float64) candidate {
	dt := t - first.Ts
	if dt <= 0 {
		if math.Abs(v-first.Value) <= errorBound {
			return candidate{slope: 0, upperDoorSlope: math.Inf(-1), lowerDoorSlope: math.Inf(1), feasible: true}
		}
		return candidate{feasible: false}
	}
	return candidate{
		slope:          (v - first.Value) / dt,
		upperDoorSlope: (v - errorBound - first.Value) / dt,
		lowerDoorSlope: (v + errorBound - first.Value) / dt,
		feasible:       true,
	}
}

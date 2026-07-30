// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sdc implements a streaming, bounded-error Swinging Door
// Trending/Compression (SDC) compressor for per-context metric point
// streams, with an EWMA-smoothed adaptive tolerance: full granularity where
// the signal moves, a handful of points where it doesn't.
//
// This is the Swinging Door Method of Bristol, U.S. Patent 4,669,097
// ("Data Compression for Display and Storage"), and follows its
// terminology: each open segment pivots two "doors" (upperDoorSlope,
// lowerDoorSlope — the patent's SU(MAX)/SL(MIN)) from the segment's first
// point, admitting successive points until the doors would cross, at
// which point the last inbounds point is issued as the segment's end and
// a new segment begins from there.
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

	// first is the current segment's first point (the patent's "first
	// corridor end point" C(i)), from which the two doors pivot.
	hasFirst bool
	first    Point

	// lastInBounds is the most recent point admitted into the current
	// segment (the patent's "last inbounds point") — the candidate for the
	// segment's end point once a later point forces the doors to cross.
	hasLastInBounds bool
	lastInBounds    Point
	// upperDoorSlope and lowerDoorSlope are the patent's SU(MAX)/SL(MIN):
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

	// A candidate is only safe to fold into the current segment if its own
	// real slope from the first point is already consistent with
	// everything swallowed so far. A merely non-empty intersected cone is
	// not enough: this point's own slope could sit outside the pre-update
	// cone while still leaving the (further-narrowed) cone non-empty,
	// which would let a later close silently misrepresent an earlier
	// point beyond tolerance. See the package tests for a concrete
	// counterexample.
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

// establishPivots starts a new segment from c.first: the patent's
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
// sdcsender's scale-deviation telemetry); the compressor's own correctness
// never depends on a caller reading this.
func (c *Compressor) Scale() float64 {
	return c.scale
}

// updateScaleAndTolerance returns the current errorBound (the patent's
// "error" / "error bound E") — here computed dynamically as
// max(Epsilon*EWMA-scale, Floor) rather than the patent's static,
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
// the upper/lower door slopes (the patent's SU(i)/SL(i)) that pivoting
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

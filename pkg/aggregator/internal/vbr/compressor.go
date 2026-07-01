// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package vbr implements a streaming, bounded-error piecewise-linear
// compressor for per-context metric point streams ("variable bit rate"
// storage: full granularity where the signal moves, a handful of points
// where it doesn't).
package vbr

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

	hasAnchor bool
	anchor    Point

	hasPending bool
	pending    Point
	slopeMin   float64
	slopeMax   float64
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
	tol := c.updateScaleAndTolerance(value)

	if c.warmupRemaining > 0 {
		c.warmupRemaining--
		c.anchor = Point{Ts: ts, Value: value}
		c.hasAnchor = true
		c.hasPending = false
		return []Point{{Ts: ts, Value: value}}
	}

	if !c.hasAnchor {
		// Defensive: warmup with Warmup>=1 always sets the anchor on its
		// last iteration, so this should not happen in practice.
		c.anchor = Point{Ts: ts, Value: value}
		c.hasAnchor = true
		return nil
	}

	if !c.hasPending {
		c.openSegment(ts, value, tol)
		return nil
	}

	cand := evalCandidate(c.anchor, ts, value, tol)

	// A candidate is only safe to fold into the current segment if its own
	// real slope from the anchor is already consistent with everything
	// swallowed so far. A merely non-empty intersected cone is not enough:
	// this point's own slope could sit outside the pre-update cone while
	// still leaving the (further-narrowed) cone non-empty, which would let
	// a later close silently misrepresent an earlier point beyond
	// tolerance. See the package tests for a concrete counterexample.
	if cand.feasible && cand.slope >= c.slopeMin && cand.slope <= c.slopeMax {
		c.slopeMin = math.Max(c.slopeMin, cand.lo)
		c.slopeMax = math.Min(c.slopeMax, cand.hi)
		c.pending = Point{Ts: ts, Value: value}
		return nil
	}

	// Close the segment at the pending point — the last point whose own
	// trajectory from the anchor was consistent with the run — then open a
	// fresh segment with this point against that new anchor.
	closed := c.pending
	c.anchor = closed
	c.hasPending = false
	c.openSegment(ts, value, tol)
	return []Point{closed}
}

// FlushWindow force-closes the currently open segment, if any, and returns
// its closing point. The scale estimate and the anchor (the returned point,
// or the previous anchor if nothing was pending) carry forward into the
// next window unchanged.
func (c *Compressor) FlushWindow(_ float64) []Point {
	if !c.hasPending {
		return nil
	}
	closed := c.pending
	c.anchor = closed
	c.hasPending = false
	return []Point{closed}
}

func (c *Compressor) openSegment(ts, value, tol float64) {
	cand := evalCandidate(c.anchor, ts, value, tol)
	if !cand.feasible {
		// Same timestamp as the anchor but outside tolerance: nothing
		// sensible to swing a door from; just re-anchor here.
		c.anchor = Point{Ts: ts, Value: value}
		c.hasPending = false
		return
	}
	c.slopeMin, c.slopeMax = cand.lo, cand.hi
	c.pending = Point{Ts: ts, Value: value}
	c.hasPending = true
}

func (c *Compressor) updateScaleAndTolerance(value float64) float64 {
	abs := math.Abs(value)
	if !c.hasScale {
		c.scale = abs
		c.hasScale = true
	} else {
		c.scale = c.cfg.Alpha*abs + (1-c.cfg.Alpha)*c.scale
	}
	tol := c.cfg.Epsilon * c.scale
	if tol < c.cfg.Floor {
		return c.cfg.Floor
	}
	return tol
}

type candidate struct {
	slope    float64
	lo, hi   float64
	feasible bool
}

// evalCandidate computes the slope from anchor to (t, v), and the interval
// of slopes from anchor that would keep (t, v) within tol. feasible is false
// only in the degenerate case where t == anchor.Ts but v differs from
// anchor.Value by more than tol, which no line through the anchor can
// satisfy.
func evalCandidate(anchor Point, t, v, tol float64) candidate {
	dt := t - anchor.Ts
	if dt <= 0 {
		if math.Abs(v-anchor.Value) <= tol {
			return candidate{slope: 0, lo: math.Inf(-1), hi: math.Inf(1), feasible: true}
		}
		return candidate{feasible: false}
	}
	return candidate{
		slope:    (v - anchor.Value) / dt,
		lo:       (v - tol - anchor.Value) / dt,
		hi:       (v + tol - anchor.Value) / dt,
		feasible: true,
	}
}

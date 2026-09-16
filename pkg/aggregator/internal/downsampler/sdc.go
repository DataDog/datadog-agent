// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package downsampler implements streaming metric downsampling algorithms.
//
// Its SDC implementation narrows a corridor of admissible slopes until a point
// falls outside it, then emits the previous in-bounds point as a breakpoint.
// The corridor width adapts to an EWMA of the signal's magnitude.
package downsampler

import "math"

// Point is a single (timestamp, value) sample or breakpoint.
type Point struct {
	Ts    float64
	Value float64
}

// SDCConfig holds the global (not per-series) downsampler parameters.
type SDCConfig struct {
	// RelativeError controls the tolerance relative to the signal's scale.
	RelativeError float64
	// ScaleSmoothingFactor controls the EWMA used to track the signal's scale.
	ScaleSmoothingFactor float64
}

// warmupSamples is long enough for the initial sample's contribution to the
// EWMA scale to fall below 5% with the default smoothing factor of 0.3.
const warmupSamples = 10

// SDC is a single-pass, constant-memory downsampler for one metric context.
type SDC struct {
	cfg SDCConfig

	scale    float64
	hasScale bool

	warmupRemaining int

	hasLatest bool
	latestTs  float64

	// first is the origin of the current segment.
	hasFirst bool
	first    Point

	// lastInBounds becomes the endpoint if a later point closes the segment.
	hasLastInBounds bool
	lastInBounds    Point
	// The admissible slope range narrows as points enter the segment.
	upperDoorSlope float64
	lowerDoorSlope float64
}

// NewSDC returns an SDC downsampler for one metric context.
func NewSDC(cfg SDCConfig) *SDC {
	return &SDC{cfg: cfg, warmupRemaining: warmupSamples}
}

// Update feeds one already-committed (timestamp, value) sample. It returns a
// breakpoint when the sample closes a segment; in steady state ok is false.
// The bounded-error guarantee requires timestamps to be strictly increasing.
// A non-increasing sample passes through verbatim without changing downsampler
// state, but reconstruction bounds are not guaranteed for streams that violate
// the timestamp-ordering contract.
func (c *SDC) Update(ts, value float64) (breakpoint Point, ok bool) {
	if c.hasLatest && ts <= c.latestTs {
		return Point{Ts: ts, Value: value}, true
	}
	c.hasLatest = true
	c.latestTs = ts

	errorBound := c.updateScaleAndTolerance(value)

	if c.warmupRemaining > 0 {
		c.warmupRemaining--
		c.first = Point{Ts: ts, Value: value}
		c.hasFirst = true
		c.hasLastInBounds = false
		return Point{Ts: ts, Value: value}, true
	}

	if !c.hasFirst {
		// Defensive: warmup always sets the first point on
		// its last iteration, so this should not happen in practice.
		c.first = Point{Ts: ts, Value: value}
		c.hasFirst = true
		return Point{}, false
	}

	if !c.hasLastInBounds {
		c.establishPivots(ts, value, errorBound)
		return Point{}, false
	}

	cand := doorSlopes(c.first, ts, value, errorBound)

	// We close segments with a real point, so its raw slope—not merely its
	// error-widened range—must fit the existing corridor. Otherwise the chosen
	// endpoint could violate the error bound for an earlier point. See
	// TestSwingDoorCandidateMustMatchItsOwnSlope.
	if cand.feasible && cand.slope >= c.upperDoorSlope && cand.slope <= c.lowerDoorSlope {
		c.upperDoorSlope = math.Max(c.upperDoorSlope, cand.upperDoorSlope)
		c.lowerDoorSlope = math.Min(c.lowerDoorSlope, cand.lowerDoorSlope)
		c.lastInBounds = Point{Ts: ts, Value: value}
		return Point{}, false
	}

	// Close at the last valid endpoint and start a new segment from it.
	closed := c.lastInBounds
	c.first = closed
	c.hasLastInBounds = false
	c.establishPivots(ts, value, errorBound)
	return closed, true
}

// FlushWindow closes the current segment while preserving adaptive state.
func (c *SDC) FlushWindow() (breakpoint Point, ok bool) {
	if !c.hasLastInBounds {
		return Point{}, false
	}
	closed := c.lastInBounds
	c.first = closed
	c.hasLastInBounds = false
	return closed, true
}

// establishPivots initializes the slope corridor toward the next point.
func (c *SDC) establishPivots(ts, value, errorBound float64) {
	cand := doorSlopes(c.first, ts, value, errorBound)
	if !cand.feasible {
		// No slope can represent both points; restart from the latest one.
		c.first = Point{Ts: ts, Value: value}
		c.hasLastInBounds = false
		return
	}
	c.upperDoorSlope, c.lowerDoorSlope = cand.upperDoorSlope, cand.lowerDoorSlope
	c.lastInBounds = Point{Ts: ts, Value: value}
	c.hasLastInBounds = true
}

// Scale returns the EWMA magnitude used to compute the error tolerance.
func (c *SDC) Scale() float64 {
	return c.scale
}

// updateScaleAndTolerance updates the EWMA and returns the current error bound.
func (c *SDC) updateScaleAndTolerance(value float64) float64 {
	abs := math.Abs(value)
	if !c.hasScale {
		c.scale = abs
		c.hasScale = true
	} else {
		c.scale = c.cfg.ScaleSmoothingFactor*abs + (1-c.cfg.ScaleSmoothingFactor)*c.scale
	}
	return c.cfg.RelativeError * c.scale
}

// candidate describes a point's raw and error-adjusted slopes.
type candidate struct {
	slope                          float64
	upperDoorSlope, lowerDoorSlope float64
	feasible                       bool
}

// doorSlopes computes the slope interval that represents the point within the
// error bound. It is infeasible when time does not advance and values differ.
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

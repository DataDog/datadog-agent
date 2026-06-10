// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package adaptivesampler exposes a shared rate-limit multiplier that the
// adaptive log sampler reads each time it decides whether to keep or drop a log.
// External components (e.g. the anomaly-scorer bridge) write to this controller
// to widen or narrow the sampler's effective rate/burst limits at runtime.
package adaptivesampler

import (
	"math"
	"sync/atomic"
)

// Controller holds an atomic rate-limit multiplier. A multiplier of 1.0 means
// no change to the sampler's configured limits; values > 1 relax sampling
// (keep more logs); values < 1 tighten it (keep fewer logs).
type Controller struct {
	bits atomic.Uint64 // IEEE 754 bits of a float64
}

// Multiplier returns the current multiplier. Returns 1.0 before any call to
// SetMultiplier.
func (c *Controller) Multiplier() float64 {
	return math.Float64frombits(c.bits.Load())
}

// SetMultiplier stores v as the current multiplier. Safe for concurrent use.
// Values ≤ 0 are silently ignored to prevent dividing credits to zero.
func (c *Controller) SetMultiplier(v float64) {
	if v <= 0 {
		return
	}
	c.bits.Store(math.Float64bits(v))
}

// shared is the process-wide singleton. Initialized to 1.0 (neutral).
var shared = func() *Controller {
	c := &Controller{}
	c.SetMultiplier(1.0)
	return c
}()

// Shared returns the process-wide Controller. All adaptive sampler instances
// and all scorer-bridge subscribers share this single object so that a severity
// transition immediately affects every log source.
func Shared() *Controller {
	return shared
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"math"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

// agentPool recycles *quantile.Agent structs to avoid per-insert heap allocations
// in the high-frequency distribution metric path.
var agentPool = sync.Pool{
	New: func() interface{} {
		return &quantile.Agent{}
	},
}

// sketchMap holds per-timestamp, per-context sketches for distribution metrics.
// It tracks the cardinality of the previous flush to hint inner-map allocation
// size, reducing rehashing pressure.
type sketchMap struct {
	m               map[int64]map[ckey.ContextKey]*quantile.Agent
	lastCardinality int // rolling hint for inner map make() capacity
}

// newSketchMap returns an initialised sketchMap.
func newSketchMap() sketchMap {
	return sketchMap{
		m: make(map[int64]map[ckey.ContextKey]*quantile.Agent),
	}
}

// Len returns the number of sketches stored
func (m *sketchMap) Len() int {
	l := 0
	for _, byCtx := range m.m {
		l += len(byCtx)
	}
	return l
}

// insert v into a sketch for the given (ts, contextKey)
// NOTE: ts is truncated to bucketSize
func (m *sketchMap) insert(ts int64, ck ckey.ContextKey, v float64, sampleRate float64) bool {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return false
	}

	m.getOrCreate(ts, ck).Insert(v, sampleRate)
	return true
}

func (m *sketchMap) insertInterp(ts int64, ck ckey.ContextKey, lower float64, upper float64, count uint) bool {
	if math.IsInf(lower, 0) || math.IsNaN(lower) {
		return false
	}

	if math.IsInf(upper, 0) || math.IsNaN(upper) {
		return false
	}

	// Use the error to indicate whether the insertion happened.
	err := m.getOrCreate(ts, ck).InsertInterpolate(lower, upper, count)
	return err == nil
}

func (m *sketchMap) getOrCreate(ts int64, ck ckey.ContextKey) *quantile.Agent {
	// level 1: ts -> ctx
	byCtx, ok := m.m[ts]
	if !ok {
		// Use lastCardinality as a capacity hint to pre-size the inner map and
		// avoid rehashing when filling a bucket with a similar number of contexts
		// as the previous flush.
		hint := m.lastCardinality
		if hint < 1 {
			hint = 1
		}
		byCtx = make(map[ckey.ContextKey]*quantile.Agent, hint)
		m.m[ts] = byCtx
	}

	// level 2: ctx -> sketch
	s, ok := byCtx[ck]
	if !ok {
		// Obtain a recycled agent from the pool rather than allocating a new struct.
		s = agentPool.Get().(*quantile.Agent)
		byCtx[ck] = s
	}

	return s
}

// countBucketsBefore returns the number of timestamp buckets with ts < beforeTs.
// Used to pre-size per-context SketchPoint slices in flushSketches.
func (m *sketchMap) countBucketsBefore(beforeTs int64) int {
	n := 0
	for ts := range m.m {
		if ts < beforeTs {
			n++
		}
	}
	return n
}

// flushBefore calls f for every sketch inserted before beforeTs, removing flushed sketches
// from the map. After flushing, each quantile.Agent is reset and returned to agentPool.
func (m *sketchMap) flushBefore(beforeTs int64, f func(ckey.ContextKey, metrics.SketchPoint)) {
	maxCardinality := 0
	for ts, byCtx := range m.m {
		if ts >= beforeTs {
			continue
		}

		if len(byCtx) > maxCardinality {
			maxCardinality = len(byCtx)
		}

		for ck, as := range byCtx {
			// FinishAndReset flushes pending values, extracts the sketch copy, resets
			// the agent in-place, and retains Buf/CountBuf backing arrays so the next
			// insert after pool reuse avoids reallocating those slices.
			sketch := as.FinishAndReset()
			agentPool.Put(as)
			f(ck, metrics.SketchPoint{
				Sketch: sketch,
				Ts:     ts,
			})
		}

		delete(m.m, ts)
	}
	// Update rolling cardinality hint. Use max across all flushed buckets so the
	// hint is never too small when a new bucket starts filling.
	if maxCardinality > 0 {
		if m.lastCardinality == 0 {
			m.lastCardinality = maxCardinality
		} else {
			// Simple EMA (α ≈ 0.5) biased toward the larger of old/new value.
			avg := (m.lastCardinality + maxCardinality) / 2
			if avg < maxCardinality {
				avg = maxCardinality
			}
			m.lastCardinality = avg
		}
	}
}

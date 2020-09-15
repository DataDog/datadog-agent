// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package aggregator

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/quantile"
)

type sketchMap map[int64]map[ckey.ContextKey]*quantile.Agent

// Len returns the number of sketches stored
func (m sketchMap) Len() int {
	l := 0
	for _, byCtx := range m {
		l += len(byCtx)
	}
	return l
}

// insert v into a sketch for the given (ts, contextKey)
// NOTE: ts is truncated to bucketSize
func (m sketchMap) insert(ts int64, ck ckey.ContextKey, v float64, sampleRate float64) bool {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return false
	}

	m.getOrCreate(ts, ck).Insert(v, sampleRate)
	return true
}

func (m sketchMap) insertInterp(ts int64, ck ckey.ContextKey, lower float64, upper float64, count uint) bool {
	if math.IsInf(lower, 0) || math.IsNaN(lower) {
		return false
	}

	if math.IsInf(upper, 0) || math.IsNaN(upper) {
		return false
	}

	m.getOrCreate(ts, ck).InsertInterpolate(lower, upper, count)
	return true
}

func (m sketchMap) getOrCreate(ts int64, ck ckey.ContextKey) *quantile.Agent {
	// level 1: ts -> ctx
	byCtx, ok := m[ts]
	if !ok {
		byCtx = make(map[ckey.ContextKey]*quantile.Agent)
		m[ts] = byCtx
	}

	// level 2: ctx -> sketch
	s, ok := byCtx[ck]
	if !ok {
		s = &quantile.Agent{}
		m[ts][ck] = s
	}

	return s
}

// flushBefore calls f for every sketch inserted before beforeTs, removing flushed sketches
// from the map.
func (m sketchMap) flushBefore(beforeTs int64, f func(ckey.ContextKey, metrics.SketchPoint)) {
	for ts, byCtx := range m {
		if ts >= beforeTs {
			continue
		}

		for ck, as := range byCtx {
			f(ck, metrics.SketchPoint{
				Sketch: as.Finish(),
				Ts:     ts,
			})
		}

		delete(m, ts)
	}
}

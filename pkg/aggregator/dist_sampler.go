// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// NOTE: This file contains a feature in development that is NOT supported.

package aggregator

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/quantile"
)

type distSampler struct {
	interval        int64
	defaultHostname string

	m           sketchMap
	ctxResolver *ContextResolver
}

func newDistSampler(interval int64, defaultHostname string) distSampler {
	if interval == 0 {
		interval = bucketSize
	}

	return distSampler{
		interval:        interval,
		defaultHostname: defaultHostname,
		m:               make(sketchMap),
		ctxResolver:     newContextResolver(),
	}
}

func (d *distSampler) truncateTs(ts int64) int64 {
	return ts - ts%d.interval
}

func (d *distSampler) addSample(ms *metrics.MetricSample, ts int64) {
	ts = d.truncateTs(ts)
	ck := d.ctxResolver.trackContext(ms, float64(ts))
	d.m.insert(ts, ck, ms.Value)
}

func (d *distSampler) flush(flushTs int64) metrics.SketchSeriesList {
	flushTs = d.truncateTs(flushTs)

	pointsByCtx := make(map[ckey.ContextKey][]metrics.SketchPoint)
	d.m.flushBefore(flushTs, func(ck ckey.ContextKey, p metrics.SketchPoint) {
		if p.Sketch == nil {
			return
		}
		pointsByCtx[ck] = append(pointsByCtx[ck], p)
	})

	out := make(metrics.SketchSeriesList, 0, len(pointsByCtx))
	for ck, points := range pointsByCtx {
		out = append(out, d.newSeries(ck, points))
	}
	d.ctxResolver.expireContexts(float64(flushTs) - defaultExpiry)
	return out
}

func (d *distSampler) newSeries(ck ckey.ContextKey, points []metrics.SketchPoint) metrics.SketchSeries {
	ctx := d.ctxResolver.contextsByKey[ck]
	ss := metrics.SketchSeries{
		Name:       ctx.Name,
		Tags:       ctx.Tags,
		Host:       ctx.Host,
		Interval:   d.interval,
		Points:     points,
		ContextKey: ck,
	}

	if ss.Host == "" {
		ss.Host = d.defaultHostname
	}

	return ss
}

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
func (m sketchMap) insert(ts int64, ck ckey.ContextKey, v float64) bool {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return false
	}

	m.getOrCreate(ts, ck).Insert(v)
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
		byCtx[ck] = s
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

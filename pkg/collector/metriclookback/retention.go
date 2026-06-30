// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metriclookback owns the shared point-retention substrate used by
// metric lookback producers. The first producers are selected DogStatsD
// no-aggregation samples, selected normal DogStatsD buckets, and later
// check/shadow-check samples.
package metriclookback

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// Retention owns the metric lookback point rings. It is intentionally outside
// pkg/aggregator so binaries that do not enable lookback do not link the
// concrete buffer code.
type Retention struct {
	bufferMu      sync.Mutex
	bufferOptions ringbuffer.Options
	buffer        *ringbuffer.Buffer

	sketchMu      sync.Mutex
	sketchOptions ringbuffer.Options
	sketchBuffer  *ringbuffer.SketchBuffer
}

// NewRetention creates a metric lookback retention backend with the provided
// ring buffer options. The backing rings are allocated lazily on first admitted
// point so enabled-but-unmatched DogStatsD lookback does not reserve retention
// memory.
func NewRetention(opts ringbuffer.Options) *Retention {
	return &Retention{bufferOptions: opts, sketchOptions: opts}
}

func (r *Retention) getBuffer(create bool) *ringbuffer.Buffer {
	if r == nil {
		return nil
	}
	r.bufferMu.Lock()
	defer r.bufferMu.Unlock()
	if r.buffer == nil && create {
		r.buffer = ringbuffer.New(r.bufferOptions)
	}
	return r.buffer
}

func (r *Retention) getSketchBuffer(create bool) *ringbuffer.SketchBuffer {
	if r == nil {
		return nil
	}
	r.sketchMu.Lock()
	defer r.sketchMu.Unlock()
	if r.sketchBuffer == nil && create {
		r.sketchBuffer = ringbuffer.NewSketchBuffer(r.sketchOptions)
	}
	return r.sketchBuffer
}

// AppendSamples stores scalar samples from the provided source in the shared
// retention ring.
func (r *Retention) AppendSamples(ctx context.Context, source ringbuffer.Source, samples []metrics.MetricSample) error {
	buffer := r.getBuffer(true)
	if buffer == nil {
		return nil
	}
	return buffer.AppendSamples(ctx, source, samples)
}

// AppendSerie stores an already-normalized series from the provided source in
// the shared retention ring. It is used for DogStatsD no-aggregation after the
// worker has built the same series it sends to the serializer.
func (r *Retention) AppendSerie(ctx context.Context, source ringbuffer.Source, serie *metrics.Serie) error {
	buffer := r.getBuffer(true)
	if buffer == nil {
		return nil
	}
	return buffer.AppendSerie(ctx, source, serie)
}

// AppendSketchSeries stores an already-finalized sketch series from the provided
// source in the shared retention ring.
func (r *Retention) AppendSketchSeries(ctx context.Context, source ringbuffer.Source, series *metrics.SketchSeries) error {
	if r == nil {
		return nil
	}
	sketchBuffer := r.getSketchBuffer(true)
	if sketchBuffer == nil {
		return nil
	}
	return sketchBuffer.AppendSketchSeries(ctx, source, series)
}

// Series returns every scalar sample currently retained in the metric lookback
// ring. It is primarily intended for tests and diagnostics; production egress
// should use ForwardRange.
func (r *Retention) Series() metrics.Series {
	buffer := r.getBuffer(false)
	if buffer == nil {
		return nil
	}
	return buffer.Series()
}

// SketchSeries returns every sketch point currently retained in the metric
// lookback ring. It is primarily intended for tests and diagnostics; production
// egress should use ForwardRange.
func (r *Retention) SketchSeries() metrics.SketchSeriesList {
	if r == nil {
		return nil
	}
	sketchBuffer := r.getSketchBuffer(false)
	if sketchBuffer == nil {
		return nil
	}
	return sketchBuffer.SketchSeriesBetween(time.Time{}, time.Time{})
}

// PointsBetween returns retained points for a metric/source in the inclusive
// [from, to] window. It is the local query surface used by monitors.
func (r *Retention) PointsBetween(source ringbuffer.Source, metricName string, from, to time.Time) []ringbuffer.Point {
	buffer := r.getBuffer(false)
	if buffer == nil {
		return nil
	}
	return buffer.PointsBetween(source, metricName, from, to)
}

// PointsBetweenSources returns retained points for a metric from any of the
// provided sources in the inclusive [from, to] window. It lets a monitor treat
// normal DogStatsD bucketed points and no-aggregation final points as one
// DogStatsD lookback view while retaining source provenance in the ring.
func (r *Retention) PointsBetweenSources(sources []ringbuffer.Source, metricName string, from, to time.Time) []ringbuffer.Point {
	buffer := r.getBuffer(false)
	if buffer == nil {
		return nil
	}
	return buffer.PointsBetweenSources(sources, metricName, from, to)
}

// ProjectedSketchPointsBetweenSources projects retained sketch points for a
// metric from any of the provided sources into scalar points in the inclusive
// [from, to] window. The projection is intentionally caller-provided so monitor
// semantics stay independent from retention and egress serialization.
func (r *Retention) ProjectedSketchPointsBetweenSources(sources []ringbuffer.Source, metricName string, from, to time.Time, projection SketchScalarProjection) []ringbuffer.Point {
	if r == nil || projection == nil {
		return nil
	}
	sketchBuffer := r.getSketchBuffer(false)
	if sketchBuffer == nil {
		return nil
	}
	return sketchBuffer.PointsBetweenSources(sources, metricName, from, to, projection.Project)
}

// Stats returns a point-in-time summary of the scalar retention ring.
func (r *Retention) Stats() ringbuffer.Stats {
	buffer := r.getBuffer(false)
	if buffer == nil {
		return ringbuffer.Stats{}
	}
	return buffer.Stats()
}

// SketchStats returns a point-in-time summary of the sketch retention ring.
func (r *Retention) SketchStats() ringbuffer.Stats {
	if r == nil {
		return ringbuffer.Stats{}
	}
	sketchBuffer := r.getSketchBuffer(false)
	if sketchBuffer == nil {
		return ringbuffer.Stats{}
	}
	return sketchBuffer.Stats()
}

// ForwardAll sends every sample currently retained in the metric lookback ring
// buffer through the provided serializer as iterable series/sketch payloads. It
// returns the number of series and sketch series sent. Forwarding is
// non-destructive.
func (r *Retention) ForwardAll(metricSerializer serializer.MetricSerializer) (int, error) {
	return r.ForwardRange(metricSerializer, time.Time{}, time.Time{})
}

// ForwardRange sends samples whose original timestamps fall in the half-open
// [from, to) window through the provided serializer as iterable series and
// sketch payloads. A zero from or to leaves that side of the window unbounded.
// It returns the number of series and sketch series sent. Forwarding is
// non-destructive.
func (r *Retention) ForwardRange(metricSerializer serializer.MetricSerializer, from, to time.Time) (int, error) {
	if r == nil {
		return 0, errors.New("metric lookback is disabled")
	}
	if metricSerializer == nil {
		return 0, errors.New("serializer is not available")
	}
	if !to.IsZero() && !from.IsZero() && !from.Before(to) {
		return 0, nil
	}

	inclusiveTo := exclusiveToInclusive(to)
	seriesCount := 0
	var seriesSource metrics.SerieSource
	if buffer := r.getBuffer(false); buffer != nil {
		seriesSource = buffer.SerieSourceBetween(from, inclusiveTo)
		seriesCount = int(seriesSource.Count())
	}

	var sketchSource metrics.SketchesSource
	sketchCount := 0
	if sketchBuffer := r.getSketchBuffer(false); sketchBuffer != nil {
		sketchSource = sketchBuffer.SketchSourceBetween(from, inclusiveTo)
		sketchCount = int(sketchSource.Count())
	}
	if seriesCount == 0 && sketchCount == 0 {
		return 0, nil
	}
	if seriesCount > 0 {
		if err := metricSerializer.SendIterableSeries(seriesSource); err != nil {
			return 0, err
		}
	}
	if sketchCount > 0 {
		if err := metricSerializer.SendSketch(sketchSource); err != nil {
			return 0, err
		}
	}
	return seriesCount + sketchCount, nil
}

func exclusiveToInclusive(to time.Time) time.Time {
	if to.IsZero() {
		return time.Time{}
	}
	return to.Add(-time.Microsecond)
}

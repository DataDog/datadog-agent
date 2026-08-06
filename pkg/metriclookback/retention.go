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

	"github.com/DataDog/datadog-agent/pkg/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/ringbuffer"
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

	monitorMu sync.RWMutex
	monitor   *monitor.Watcher
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

// SetMonitor installs the optional monitor notified by shadow-check sender
// writes. Retention remains storage-only for ordinary AppendSamples callers; this
// hook lets the sender integration use the same materialized-read monitor path
// as DogStatsD without adding a second hot-path tap.
func (r *Retention) SetMonitor(watcher *monitor.Watcher) {
	if r == nil {
		return
	}
	r.monitorMu.Lock()
	defer r.monitorMu.Unlock()
	r.monitor = watcher
}

func (r *Retention) getMonitor() *monitor.Watcher {
	if r == nil {
		return nil
	}
	r.monitorMu.RLock()
	defer r.monitorMu.RUnlock()
	return r.monitor
}

// ObserveSamples notifies the optional monitor that retained samples were
// admitted. Source-specific integrations call this after successful AppendSamples
// writes so retention itself can stay independent of producer APIs.
func (r *Retention) ObserveSamples(samples []metrics.MetricSample) {
	watcher := r.getMonitor()
	if watcher == nil {
		return
	}
	for i := range samples {
		watcher.Observe(samples[i].Name, sampleObservedAt(samples[i]))
	}
}

func sampleObservedAt(sample metrics.MetricSample) time.Time {
	if sample.Timestamp > 0 {
		return time.UnixMicro(int64(sample.Timestamp * 1e6))
	}
	return time.Now()
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
// non-destructive. Retrying callers that need to avoid duplicates after partial
// serializer failures should use ForwardSeriesRange and ForwardSketchRange so
// each pipeline can be marked forwarded independently.
func (r *Retention) ForwardRange(metricSerializer serializer.MetricSerializer, from, to time.Time) (int, error) {
	seriesCount, err := r.ForwardSeriesRange(metricSerializer, from, to)
	if err != nil {
		return 0, err
	}
	sketchCount, err := r.ForwardSketchRange(metricSerializer, from, to)
	if err != nil {
		return 0, err
	}
	return seriesCount + sketchCount, nil
}

// ForwardSeriesRange sends series samples whose original timestamps fall in the
// half-open [from, to) window. A zero from or to leaves that side of the window
// unbounded. Forwarding is non-destructive.
func (r *Retention) ForwardSeriesRange(metricSerializer serializer.MetricSerializer, from, to time.Time) (int, error) {
	inclusiveFrom, inclusiveTo, ok, err := r.forwardRangeBounds(metricSerializer, from, to)
	if err != nil || !ok {
		return 0, err
	}

	seriesCount := 0
	var seriesSource metrics.SerieSource
	if buffer := r.getBuffer(false); buffer != nil {
		seriesSource = buffer.SerieSourceBetween(inclusiveFrom, inclusiveTo)
		seriesCount = int(seriesSource.Count())
	}
	if seriesCount == 0 {
		return 0, nil
	}
	if err := metricSerializer.SendIterableSeries(seriesSource); err != nil {
		return 0, err
	}
	return seriesCount, nil
}

// ForwardSketchRange sends sketch samples whose original timestamps fall in the
// half-open [from, to) window. A zero from or to leaves that side of the window
// unbounded. Forwarding is non-destructive.
func (r *Retention) ForwardSketchRange(metricSerializer serializer.MetricSerializer, from, to time.Time) (int, error) {
	inclusiveFrom, inclusiveTo, ok, err := r.forwardRangeBounds(metricSerializer, from, to)
	if err != nil || !ok {
		return 0, err
	}

	sketchCount := 0
	var sketchSource metrics.SketchesSource
	if sketchBuffer := r.getSketchBuffer(false); sketchBuffer != nil {
		sketchSource = sketchBuffer.SketchSourceBetween(inclusiveFrom, inclusiveTo)
		sketchCount = int(sketchSource.Count())
	}
	if sketchCount == 0 {
		return 0, nil
	}
	if err := metricSerializer.SendSketch(sketchSource); err != nil {
		return 0, err
	}
	return sketchCount, nil
}

func (r *Retention) forwardRangeBounds(metricSerializer serializer.MetricSerializer, from, to time.Time) (time.Time, time.Time, bool, error) {
	if r == nil {
		return time.Time{}, time.Time{}, false, errors.New("metric lookback is disabled")
	}
	if metricSerializer == nil {
		return time.Time{}, time.Time{}, false, errors.New("serializer is not available")
	}
	if !to.IsZero() && !from.IsZero() && !from.Before(to) {
		return time.Time{}, time.Time{}, false, nil
	}

	inclusiveFrom, inclusiveTo, ok := halfOpenRangeToInclusiveMicroRange(from, to)
	if !ok {
		return time.Time{}, time.Time{}, false, nil
	}
	return inclusiveFrom, inclusiveTo, true, nil
}

// halfOpenRangeToInclusiveMicroRange converts the public [from, to) range to
// the inclusive microsecond bounds used by retention records.
func halfOpenRangeToInclusiveMicroRange(from, to time.Time) (time.Time, time.Time, bool) {
	inclusiveFrom := inclusiveMicroLowerBound(from)
	inclusiveTo := exclusiveMicroUpperBound(to)
	if !inclusiveFrom.IsZero() && !inclusiveTo.IsZero() && inclusiveFrom.After(inclusiveTo) {
		return time.Time{}, time.Time{}, false
	}
	return inclusiveFrom, inclusiveTo, true
}

func inclusiveMicroLowerBound(from time.Time) time.Time {
	if from.IsZero() {
		return time.Time{}
	}
	truncated := time.UnixMicro(from.UnixMicro())
	if from.Equal(truncated) || from.Before(truncated) {
		return truncated
	}
	return truncated.Add(time.Microsecond)
}

func exclusiveMicroUpperBound(to time.Time) time.Time {
	if to.IsZero() {
		return time.Time{}
	}
	truncated := time.UnixMicro(to.UnixMicro())
	if to.Equal(truncated) || to.Before(truncated) {
		return truncated.Add(-time.Microsecond)
	}
	return truncated
}

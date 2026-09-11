// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/sdc"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// tlmSDCSamples and tlmSDCBreakpoints count, per check, how many committed
// samples reached SDC and how many breakpoints it selected. Their ratio is
// the compression ratio for that check. Dry-run mode updates both counters
// from the compression decision while still shipping the original points.
var exportedSDCMetric = telemetry.Options{DefaultMetric: true}

var (
	tlmSDCSamples = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_sdc", "samples_total",
		[]string{"check_name"},
		"Number of committed samples fed into the SDC compressor, by check name",
		exportedSDCMetric)
	tlmSDCBreakpoints = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_sdc", "breakpoints_total",
		[]string{"check_name"},
		"Number of breakpoints selected by the SDC compressor, by check name",
		exportedSDCMetric)
)

// downsampledSerie owns one context's persistent SDC state and the resolved
// serie accumulated during the current CheckSampler flush window. A flush
// closes the current segment and clears serie, but keeps the compressor's
// EWMA, warmup progress, and new segment origin for the next window.
type downsampledSerie struct {
	serie      *metrics.Serie
	compressor *sdc.Compressor
	expired    bool
}

func newDownsampledSerie(serie *metrics.Serie, cfg sdc.Config) *downsampledSerie {
	return &downsampledSerie{serie: serie, compressor: sdc.New(cfg)}
}

// append adopts the first resolved serie in a flush window, then merges any
// later commits for the same context into it. The metadata is stable for a
// ContextKey; only the points differ between commits.
func (ds *downsampledSerie) append(serie *metrics.Serie) {
	if ds.serie == nil {
		ds.serie = serie
	} else {
		ds.serie.Points = append(ds.serie.Points, serie.Points...)
	}
	ds.expired = false
}

// compress applies SDC and force-closes the trailing segment so the final
// input point is represented at every CheckSampler flush boundary.
func (ds *downsampledSerie) compress(dryRun bool, samples, breakpoints telemetry.SimpleCounter) *metrics.Serie {
	original := ds.serie.Points
	kept := make([]metrics.Point, 0, len(original))

	for _, point := range original {
		selected := ds.compressor.Update(point.Ts, point.Value)
		samples.Inc()
		for _, breakpoint := range selected {
			breakpoints.Inc()
			if !dryRun {
				kept = append(kept, metrics.Point{Ts: breakpoint.Ts, Value: breakpoint.Value})
			}
		}
	}

	if len(original) > 0 {
		for _, breakpoint := range ds.compressor.FlushWindow(original[len(original)-1].Ts) {
			breakpoints.Inc()
			if !dryRun {
				kept = append(kept, metrics.Point{Ts: breakpoint.Ts, Value: breakpoint.Value})
			}
		}
	}

	output := ds.serie
	if !dryRun {
		output.Points = kept
	}
	ds.serie = nil
	return output
}

// checkSDCCompressor accumulates eligible gauge series for one CheckSampler.
// Each context's compressor state persists until the context expires or the
// sampler is released; pending serie data lasts only until the next flush.
type checkSDCCompressor struct {
	dryRun bool
	cfg    sdc.Config

	tlmSamples     telemetry.SimpleCounter
	tlmBreakpoints telemetry.SimpleCounter

	series map[ckey.ContextKey]*downsampledSerie
}

func newCheckSDCCompressor(id checkid.ID) *checkSDCCompressor {
	checkName := checkid.IDToCheckName(id)
	if !sdc.EnabledFor(checkName) {
		return nil
	}
	return &checkSDCCompressor{
		dryRun:         sdc.DryRun(),
		cfg:            sdc.CompressorConfig(),
		tlmSamples:     tlmSDCSamples.WithValues(checkName),
		tlmBreakpoints: tlmSDCBreakpoints.WithValues(checkName),
		series:         make(map[ckey.ContextKey]*downsampledSerie),
	}
}

// stashIfEligible takes ownership of an eligible gauge serie until the next
// CheckSampler flush. Metric type must come from the resolved Context because
// the API Serie type represents both Gauge and Rate as APIGaugeType.
func (sc *checkSDCCompressor) stashIfEligible(contextKey ckey.ContextKey, metricType metrics.MetricType, serie *metrics.Serie) bool {
	if metricType != metrics.GaugeType && metricType != metrics.GaugeWithTimestampType {
		return false
	}
	if existing := sc.series[contextKey]; existing != nil {
		existing.append(serie)
	} else {
		sc.series[contextKey] = newDownsampledSerie(serie, sc.cfg)
	}
	return true
}

// expire drops idle compressor state immediately. If the context still has
// points waiting for the next aggregator flush, it is deleted only after
// those points have been compressed and emitted.
func (sc *checkSDCCompressor) expire(contextKey ckey.ContextKey) {
	downsampled := sc.series[contextKey]
	if downsampled == nil {
		return
	}
	if downsampled.serie == nil {
		delete(sc.series, contextKey)
		return
	}
	downsampled.expired = true
}

// flush closes every segment with pending points. Compressor state remains
// available for the next window unless its context expired while points were
// pending, in which case it is removed after producing this final serie.
func (sc *checkSDCCompressor) flush() metrics.Series {
	if len(sc.series) == 0 {
		return nil
	}
	series := make(metrics.Series, 0, len(sc.series))
	for contextKey, downsampled := range sc.series {
		if downsampled.serie == nil {
			continue
		}
		series = append(series, downsampled.compress(sc.dryRun, sc.tlmSamples, sc.tlmBreakpoints))
		if downsampled.expired {
			delete(sc.series, contextKey)
		}
	}
	return series
}

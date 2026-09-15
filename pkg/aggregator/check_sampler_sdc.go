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
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// tlmSDCSamples and tlmSDCBreakpoints count, per check, how many committed
// samples reached SDC and how many breakpoints it selected. Their ratio is
// the downsampling ratio for that check. Dry-run mode updates both counters
// from the downsampling decision while still shipping the original points.
var exportedSDCMetric = telemetry.Options{DefaultMetric: true}

var (
	tlmSDCSamples = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_downsampling", "samples_total",
		[]string{"check_name"},
		"Number of committed samples fed into the SDC downsampler, by check name",
		exportedSDCMetric)
	tlmSDCBreakpoints = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_downsampling", "breakpoints_total",
		[]string{"check_name"},
		"Number of breakpoints selected by the SDC downsampler, by check name",
		exportedSDCMetric)
)

// downsampledSerie owns one context's persistent SDC state and the resolved
// serie accumulated during the current CheckSampler flush window. A flush
// closes the current segment and clears serie, but keeps the downsampler's
// EWMA, warmup progress, and new segment origin for the next window.
type downsampledSerie struct {
	serie       *metrics.Serie
	downsampler *sdc.Downsampler
	expired     bool
}

func newDownsampledSerie(serie *metrics.Serie, cfg sdc.Config) *downsampledSerie {
	return &downsampledSerie{serie: serie, downsampler: sdc.New(cfg)}
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

// downsample applies SDC and force-closes the trailing segment so the final
// input point is represented at every CheckSampler flush boundary.
func (ds *downsampledSerie) downsample(dryRun bool, samples, breakpoints telemetry.SimpleCounter) *metrics.Serie {
	original := ds.serie.Points
	kept := make([]metrics.Point, 0, len(original))

	for _, point := range original {
		selected := ds.downsampler.Update(point.Ts, point.Value)
		samples.Inc()
		for _, breakpoint := range selected {
			breakpoints.Inc()
			if !dryRun {
				kept = append(kept, metrics.Point{Ts: breakpoint.Ts, Value: breakpoint.Value})
			}
		}
	}

	if len(original) > 0 {
		for _, breakpoint := range ds.downsampler.FlushWindow(original[len(original)-1].Ts) {
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

// checkSDCDownsampler accumulates eligible gauge series for one CheckSampler.
// Each context's downsampler state persists until the context expires or the
// sampler is released; pending serie data lasts only until the next flush.
type checkSDCDownsampler struct {
	dryRun bool
	cfg    sdc.Config

	tlmSamples     telemetry.SimpleCounter
	tlmBreakpoints telemetry.SimpleCounter

	series map[ckey.ContextKey]*downsampledSerie
}

func newCheckSDCDownsampler(id checkid.ID) *checkSDCDownsampler {
	checkName := checkid.IDToCheckName(id)
	cfg := pkgconfigsetup.Datadog()
	if !cfg.GetBool("adaptive_downsampling.all") {
		enabled := false
		for _, name := range cfg.GetStringSlice("adaptive_downsampling.checks") {
			if name == checkName {
				enabled = true
				break
			}
		}
		if !enabled {
			return nil
		}
	}

	return &checkSDCDownsampler{
		dryRun: cfg.GetBool("adaptive_downsampling.dry_run"),
		cfg: sdc.Config{
			RelativeError:        cfg.GetFloat64("adaptive_downsampling.relative_error"),
			ScaleSmoothingFactor: cfg.GetFloat64("adaptive_downsampling.scale_smoothing_factor"),
		},
		tlmSamples:     tlmSDCSamples.WithValues(checkName),
		tlmBreakpoints: tlmSDCBreakpoints.WithValues(checkName),
		series:         make(map[ckey.ContextKey]*downsampledSerie),
	}
}

// stashIfEligible takes ownership of an eligible gauge serie until the next
// CheckSampler flush. Metric type must come from the resolved Context because
// the API Serie type represents both Gauge and Rate as APIGaugeType.
func (sc *checkSDCDownsampler) stashIfEligible(contextKey ckey.ContextKey, metricType metrics.MetricType, serie *metrics.Serie) bool {
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

// expire drops idle downsampler state immediately. If the context still has
// points waiting for the next aggregator flush, it is deleted only after
// those points have been downsampled and emitted.
func (sc *checkSDCDownsampler) expire(contextKey ckey.ContextKey) {
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

// flush closes every segment with pending points. Downsampler state remains
// available for the next window unless its context expired while points were
// pending, in which case it is removed after producing this final serie.
func (sc *checkSDCDownsampler) flush() metrics.Series {
	if len(sc.series) == 0 {
		return nil
	}
	series := make(metrics.Series, 0, len(sc.series))
	for contextKey, downsampled := range sc.series {
		if downsampled.serie == nil {
			continue
		}
		series = append(series, downsampled.downsample(sc.dryRun, sc.tlmSamples, sc.tlmBreakpoints))
		if downsampled.expired {
			delete(sc.series, contextKey)
		}
	}
	return series
}

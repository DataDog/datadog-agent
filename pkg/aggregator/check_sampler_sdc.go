// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"math"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/downsampler"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// tlmSDCSamples and tlmSDCBreakpoints count, per check, how many committed
// samples reached SDC and how many breakpoints it selected. Their ratio is
// the downsampling ratio for that check. Dry-run mode updates both counters
// from the downsampling decision while still shipping the original points.
var (
	tlmSDCSamples = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_downsampling", "samples_total",
		[]string{"check_name"},
		"Number of committed samples fed into the SDC downsampler, by check name",
		telemetry.Options{DefaultMetric: true})
	tlmSDCBreakpoints = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_downsampling", "breakpoints_total",
		[]string{"check_name"},
		"Number of breakpoints selected by the SDC downsampler, by check name",
		telemetry.Options{DefaultMetric: true})
)

// downsampledSerie owns one context's persistent SDC state and the output
// series pending for the current CheckSampler flush window.
type downsampledSerie struct {
	// template retains metadata, never points, for closing a segment after a
	// quiet window. Copy it before handing output to the serializer.
	template        metrics.Serie
	series          metrics.Series
	downsampler     *downsampler.SDC
	latestTimestamp float64
	expired         bool
}

func newDownsampledSerie(cfg downsampler.SDCConfig) *downsampledSerie {
	return &downsampledSerie{
		downsampler:     downsampler.NewSDC(cfg),
		latestTimestamp: math.Inf(-1),
	}
}

// process feeds one committed serie through the shared SDC state. Dry-run mode
// preserves the original serie; active mode creates a corresponding output.
func (ds *downsampledSerie) process(serie *metrics.Serie, dryRun bool, samples, breakpoints telemetry.SimpleCounter) {
	ds.template = *serie
	ds.template.Points = nil
	points := serie.Points
	if !dryRun {
		serie.Points = nil
	}
	ds.series = append(ds.series, serie)

	samples.Add(float64(len(points)))
	selectedCount := 0
	for _, point := range points {
		// All series serializers truncate timestamps to integer seconds. Fit
		// the corridor at that precision so serialization cannot change the
		// reconstructed line. Keep the original points untouched in dry-run.
		ts := float64(int64(point.Ts))
		if ts <= ds.latestTimestamp {
			log.Warnf(
				"Adaptive downsampling received non-increasing serialized timestamp %v after %v for metric %q (input timestamp %v); passing the point through without updating SDC state",
				ts, ds.latestTimestamp, serie.Name, point.Ts,
			)
		} else {
			ds.latestTimestamp = ts
		}

		breakpoint, selected := ds.downsampler.Update(ts, point.Value)
		if !selected {
			continue
		}
		selectedCount++
		if !dryRun {
			serie.Points = append(serie.Points, metrics.Point{Ts: breakpoint.Ts, Value: breakpoint.Value})
		}
	}
	if selectedCount > 0 {
		breakpoints.Add(float64(selectedCount))
	}
	ds.expired = false
}

// flush drains pending output every window, optionally closing the trailing
// segment. A segment can remain open even when there are no pending series.
func (ds *downsampledSerie) flush(forceClose, dryRun bool, breakpoints telemetry.SimpleCounter) metrics.Series {
	if forceClose {
		if breakpoint, selected := ds.downsampler.FlushWindow(); selected {
			breakpoints.Inc()
			if !dryRun {
				if len(ds.series) == 0 {
					output := ds.template
					ds.series = append(ds.series, &output)
				}
				output := ds.series[len(ds.series)-1]
				output.Points = append(output.Points, metrics.Point{Ts: breakpoint.Ts, Value: breakpoint.Value})
			}
		}
	}

	output := ds.series
	if !dryRun {
		nonEmpty := output[:0]
		for _, serie := range output {
			if len(serie.Points) > 0 {
				nonEmpty = append(nonEmpty, serie)
			}
		}
		output = nonEmpty
	}
	ds.series = nil
	return output
}

// checkSDCDownsampler accumulates eligible gauge series for one CheckSampler.
// Each context's downsampler state persists until the context expires or the
// sampler is released; pending series data lasts only until the next flush.
type checkSDCDownsampler struct {
	dryRun bool
	cfg    downsampler.SDCConfig

	maxGapFlushes     int
	flushesSinceClose int

	tlmSamples     telemetry.SimpleCounter
	tlmBreakpoints telemetry.SimpleCounter

	// series is nil when downsampling is disabled. An enabled downsampler
	// always has a non-nil map.
	series map[ckey.ContextKey]*downsampledSerie
}

func newCheckSDCDownsampler(id checkid.ID) checkSDCDownsampler {
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
			return checkSDCDownsampler{}
		}
	}

	return checkSDCDownsampler{
		dryRun:        cfg.GetBool("adaptive_downsampling.dry_run"),
		maxGapFlushes: max(1, cfg.GetInt("adaptive_downsampling.max_gap_flushes")),
		cfg: downsampler.SDCConfig{
			RelativeError:        cfg.GetFloat64("adaptive_downsampling.relative_error"),
			ScaleSmoothingFactor: cfg.GetFloat64("adaptive_downsampling.scale_smoothing_factor"),
		},
		tlmSamples:     tlmSDCSamples.WithValues(checkName),
		tlmBreakpoints: tlmSDCBreakpoints.WithValues(checkName),
		series:         make(map[ckey.ContextKey]*downsampledSerie),
	}
}

// isMetricTypeEligible reports whether the enabled downsampler accepts the
// resolved metric type. Metric type must come from the resolved Context because
// the API Serie type represents both Gauge and Rate as APIGaugeType.
func (sc *checkSDCDownsampler) isMetricTypeEligible(metricType metrics.MetricType) bool {
	return sc.series != nil && (metricType == metrics.GaugeType || metricType == metrics.GaugeWithTimestampType)
}

// take assumes ownership of serie and routes it through the context's SDC
// state. Output remains pending until the next CheckSampler flush.
func (sc *checkSDCDownsampler) take(contextKey ckey.ContextKey, serie *metrics.Serie) {
	downsampled := sc.series[contextKey]
	if downsampled == nil {
		downsampled = newDownsampledSerie(sc.cfg)
		sc.series[contextKey] = downsampled
	}
	downsampled.process(serie, sc.dryRun, sc.tlmSamples, sc.tlmBreakpoints)
}

// expire drops idle downsampler state immediately unless it still owns queued
// output or a deferred endpoint. Those contexts are force-closed and removed
// on the next aggregator flush, regardless of the periodic close schedule.
func (sc *checkSDCDownsampler) expire(contextKey ckey.ContextKey) {
	downsampled := sc.series[contextKey]
	if downsampled == nil {
		return
	}
	if len(downsampled.series) == 0 && !downsampled.downsampler.HasPendingEndpoint() {
		delete(sc.series, contextKey)
		return
	}
	downsampled.expired = true
}

// flush drains selected breakpoints every window and closes segments on a
// shared per-check schedule, including quiet windows. Expiration forces an
// early close without changing that schedule for the remaining contexts.
// Retiring a sampler always closes its remaining endpoints before removal.
func (sc *checkSDCDownsampler) flush(retiring bool) metrics.Series {
	if sc.series == nil {
		return nil
	}
	sc.flushesSinceClose++
	forceClose := retiring || sc.flushesSinceClose >= sc.maxGapFlushes
	if forceClose {
		sc.flushesSinceClose = 0
	}
	series := make(metrics.Series, 0, len(sc.series))
	for contextKey, downsampled := range sc.series {
		series = append(series, downsampled.flush(forceClose || downsampled.expired, sc.dryRun, sc.tlmBreakpoints)...)
		if downsampled.expired {
			delete(sc.series, contextKey)
		}
	}
	return series
}

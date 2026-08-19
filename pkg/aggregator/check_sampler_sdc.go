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
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/sdc"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// tlmSDCSamples and tlmSDCBreakpoints count, per check and metric name, how
// many real commits-with-a-sample reached the compressor and how many
// breakpoints it shipped. Unlike the earlier sender-level sdcsender
// implementation this replaces, one increment here corresponds to one real
// per-check flush cycle with a sample for that context — already
// deduplicated by Gauge.addSample's keep-last-value semantics upstream, not
// one increment per raw sender call. Their ratio is the compression ratio
// for that metric; computed at query time rather than stored directly,
// since a ratio gauge can't be usefully aggregated across time or hosts the
// way two counters can.
//
// tlmSDCFloorBoundSamples/tlmSDCFloorBoundBreakpoints are the same pair,
// narrowed to samples processed while Floor (rather than Epsilon*scale) set
// the tolerance — the regime a near-zero-scale signal (e.g. a small offset
// metric) can spend all its time in, since Floor is a single global
// constant shared by every check/metric regardless of its typical
// magnitude.
//
// tlmSDCContexts tracks how many distinct contexts (metric+tags
// combinations) are being compressed for a check. Contexts are expired the
// same way any other CheckSampler-tracked context is (see
// checkSDCCompressor.expire), unlike the old sender-level implementation's
// map, which never expired entries.
//
// tlmSDCScaleDeviationSum/tlmSDCScaleDeviationCount together track, per
// sample, |value - compressor.Scale()| — how far the raw value strays from
// the compressor's current EWMA estimate of the signal's magnitude, the
// basis for its tolerance (see sdc.Compressor.Scale). Their ratio
// (sum/count) is the average deviation; a chronically large one signals the
// EWMA is mistracking the signal, otherwise invisible from
// samples_total/breakpoints_total alone.
var exportedSDCMetric = telemetry.Options{DefaultMetric: true}

var (
	tlmSDCSamples = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_sdc", "samples_total",
		[]string{"check_name", "metric_name"},
		"Number of real per-check commits with a sample fed into the SDC compressor, by check and metric name",
		exportedSDCMetric)
	tlmSDCBreakpoints = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_sdc", "breakpoints_total",
		[]string{"check_name", "metric_name"},
		"Number of breakpoints shipped by the SDC compressor, by check and metric name",
		exportedSDCMetric)
	tlmSDCFloorBoundSamples = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_sdc", "floor_bound_samples_total",
		[]string{"check_name", "metric_name"},
		"Number of samples processed while Floor (not Epsilon*scale) set the tolerance, by check and metric name",
		exportedSDCMetric)
	tlmSDCFloorBoundBreakpoints = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_sdc", "floor_bound_breakpoints_total",
		[]string{"check_name", "metric_name"},
		"Number of breakpoints shipped while Floor set the tolerance, by check and metric name — floor_bound_samples_total minus this is how many points were swallowed specifically because of Floor",
		exportedSDCMetric)
	tlmSDCContexts = telemetryimpl.GetCompatComponent().NewGaugeWithOpts(
		"checksampler_sdc", "contexts",
		[]string{"check_name"},
		"Number of distinct metric contexts being SDC-compressed, by check name",
		exportedSDCMetric)
	tlmSDCScaleDeviationSum = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_sdc", "scale_deviation_sum",
		[]string{"check_name", "metric_name"},
		"Running sum of |value - EWMA scale| across all samples, by check and metric name — divide by checksampler_sdc_scale_deviation_count for the average",
		exportedSDCMetric)
	tlmSDCScaleDeviationCount = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_sdc", "scale_deviation_count",
		[]string{"check_name", "metric_name"},
		"Number of samples observed for checksampler_sdc_scale_deviation_sum, by check and metric name",
		exportedSDCMetric)
)

// noopSDCCounter/noopSDCGauge implement telemetry.SimpleCounter/
// telemetry.SimpleGauge as pure no-ops. Used in place of a real
// WithValues(...)-derived child series when
// checks.sdc_compression_telemetry_enabled is false: unlike merely skipping
// Inc()/Add() calls, never calling WithValues() at all means no child
// series is ever materialized in the underlying Prometheus vec, so there is
// nothing for the built-in "telemetry" corecheck to scrape and ship as
// datadog.agent.checksampler_sdc_* — with SDC compression enabled for many
// checks/metrics, that check_name/metric_name-tagged telemetry can itself
// become a sizable share of the very payload SDC is meant to shrink.
type noopSDCCounter struct{}

func (noopSDCCounter) Inc()         {}
func (noopSDCCounter) Add(float64)  {}
func (noopSDCCounter) Get() float64 { return 0 }

type noopSDCGauge struct{}

func (noopSDCGauge) Inc()         {}
func (noopSDCGauge) Dec()         {}
func (noopSDCGauge) Add(float64)  {}
func (noopSDCGauge) Sub(float64)  {}
func (noopSDCGauge) Set(float64)  {}
func (noopSDCGauge) Get() float64 { return 0 }

var (
	noopSDCSimpleCounter telemetry.SimpleCounter = noopSDCCounter{}
	noopSDCSimpleGauge   telemetry.SimpleGauge   = noopSDCGauge{}
)

// sdcContextState holds one context's SDC compressor plus its keep-alive
// bookkeeping and per-context telemetry handles.
type sdcContextState struct {
	compressor *sdc.Compressor
	// cfg is the compressor's own config, kept alongside it so floorBound
	// can recompute whether Floor is currently the binding tolerance term
	// without re-reading (possibly-changed-since) live config.
	cfg sdc.Config

	// commitsSinceShip counts consecutive commits with a sample but no
	// natural breakpoint; see checkSDCCompressor.keepAliveCommits.
	commitsSinceShip int

	tlmSamples               telemetry.SimpleCounter
	tlmBreakpoints           telemetry.SimpleCounter
	tlmFloorBoundSamples     telemetry.SimpleCounter
	tlmFloorBoundBreakpoints telemetry.SimpleCounter
	tlmScaleDeviationSum     telemetry.SimpleCounter
	tlmScaleDeviationCount   telemetry.SimpleCounter
}

// floorBound reports whether Floor, rather than Epsilon*scale, currently
// sets this context's tolerance — mirroring sdc.Compressor's own internal
// updateScaleAndTolerance formula exactly, using the compressor's current
// Scale() (its EWMA estimate after the most recently processed sample).
func (st *sdcContextState) floorBound() bool {
	return st.cfg.Epsilon*st.compressor.Scale() < st.cfg.Floor
}

// checkSDCCompressor holds all SDC compression state for one CheckSampler
// (one check instance). Only ever non-nil on a CheckSampler whose check is
// eligible per sdc.EnabledFor — see newCheckSDCCompressor.
type checkSDCCompressor struct {
	checkName        string
	dryRun           bool
	cfg              sdc.Config
	keepAliveCommits int
	telemetryEnabled bool

	tlmContexts telemetry.SimpleGauge
	contexts    map[ckey.ContextKey]*sdcContextState
}

// newCheckSDCCompressor returns nil if checkName is not eligible for SDC
// compression, mirroring the once-per-check-creation eligibility decision
// the earlier sender-level sdcsender implementation made in GetSender.
func newCheckSDCCompressor(id checkid.ID) *checkSDCCompressor {
	checkName := checkid.IDToCheckName(id)
	if !sdc.EnabledFor(checkName) {
		return nil
	}
	telemetryEnabled := sdc.TelemetryEnabled()
	tlmCtx := noopSDCSimpleGauge
	if telemetryEnabled {
		tlmCtx = tlmSDCContexts.WithValues(checkName)
	}
	return &checkSDCCompressor{
		checkName:        checkName,
		dryRun:           sdc.DryRun(),
		cfg:              sdc.CompressorConfig(),
		keepAliveCommits: sdc.KeepAliveCommits(),
		telemetryEnabled: telemetryEnabled,
		tlmContexts:      tlmCtx,
		contexts:         make(map[ckey.ContextKey]*sdcContextState),
	}
}

// noteSample lazily creates per-context compressor state for contextKey,
// keyed by the check-level metric kind (metricSample.Mtype), which is
// unambiguous here — unlike the outgoing Serie.MType seen later in apply,
// which conflates Gauge and Rate (both wire as APIGaugeType). Called from
// addSample for every Gauge/GaugeWithTimestamp sample on an eligible check.
func (sc *checkSDCCompressor) noteSample(contextKey ckey.ContextKey, metricName string) {
	if _, ok := sc.contexts[contextKey]; ok {
		return
	}
	st := &sdcContextState{compressor: sdc.New(sc.cfg), cfg: sc.cfg}
	if sc.telemetryEnabled {
		st.tlmSamples = tlmSDCSamples.WithValues(sc.checkName, metricName)
		st.tlmBreakpoints = tlmSDCBreakpoints.WithValues(sc.checkName, metricName)
		st.tlmFloorBoundSamples = tlmSDCFloorBoundSamples.WithValues(sc.checkName, metricName)
		st.tlmFloorBoundBreakpoints = tlmSDCFloorBoundBreakpoints.WithValues(sc.checkName, metricName)
		st.tlmScaleDeviationSum = tlmSDCScaleDeviationSum.WithValues(sc.checkName, metricName)
		st.tlmScaleDeviationCount = tlmSDCScaleDeviationCount.WithValues(sc.checkName, metricName)
	} else {
		st.tlmSamples = noopSDCSimpleCounter
		st.tlmBreakpoints = noopSDCSimpleCounter
		st.tlmFloorBoundSamples = noopSDCSimpleCounter
		st.tlmFloorBoundBreakpoints = noopSDCSimpleCounter
		st.tlmScaleDeviationSum = noopSDCSimpleCounter
		st.tlmScaleDeviationCount = noopSDCSimpleCounter
	}
	sc.contexts[contextKey] = st
	sc.tlmContexts.Inc()
}

// apply runs the compression decision for one already-flushed serie
// belonging to a tracked (Gauge-family) context. serie.Points has exactly
// one point for a plain Gauge (Gauge.flush always produces one), but can
// have more than one for GaugeWithTimestamp (metrics.MetricWithTimestamp's
// addSample appends rather than overwrites, so multiple calls within one
// commit all survive to flush time). Mutates serie.Points in place to the
// kept subset; returns false when nothing should ship this commit, in
// which case the caller must drop the whole Serie rather than append it.
func (sc *checkSDCCompressor) apply(contextKey ckey.ContextKey, serie *metrics.Serie) bool {
	st, ok := sc.contexts[contextKey]
	if !ok {
		// Not a Gauge-family context (see noteSample) — nothing to do.
		return true
	}

	kept := serie.Points[:0]
	for _, pt := range serie.Points {
		bps := st.compressor.Update(pt.Ts, pt.Value)
		st.tlmSamples.Inc()
		st.tlmScaleDeviationSum.Add(math.Abs(pt.Value - st.compressor.Scale()))
		st.tlmScaleDeviationCount.Inc()
		floorBound := st.floorBound()
		if floorBound {
			st.tlmFloorBoundSamples.Inc()
		}

		if sc.dryRun {
			// Telemetry above already measured what compression would do;
			// every point still ships unmodified in dry-run mode.
			kept = append(kept, pt)
			continue
		}
		if len(bps) > 0 {
			st.commitsSinceShip = 0
		}
		for _, bp := range bps {
			kept = append(kept, metrics.Point{Ts: bp.Ts, Value: bp.Value})
			st.tlmBreakpoints.Inc()
			if floorBound {
				st.tlmFloorBoundBreakpoints.Inc()
			}
		}
	}

	if !sc.dryRun && len(kept) == 0 && len(serie.Points) > 0 {
		// Nothing shipped naturally this commit: consider the keep-alive
		// once per commit (not once per point), using the last point's
		// timestamp as the force-close time.
		st.commitsSinceShip++
		if sc.keepAliveCommits > 0 && st.commitsSinceShip >= sc.keepAliveCommits {
			st.commitsSinceShip = 0
			lastPt := serie.Points[len(serie.Points)-1]
			floorBound := st.floorBound()
			if fw := st.compressor.FlushWindow(lastPt.Ts); len(fw) > 0 {
				kept = append(kept, metrics.Point{Ts: fw[0].Ts, Value: fw[0].Value})
				st.tlmBreakpoints.Inc()
				if floorBound {
					st.tlmFloorBoundBreakpoints.Inc()
				}
			}
		}
	}

	if len(kept) == 0 {
		return false
	}
	serie.Points = kept
	return true
}

// expire forgets a context's compressor state once CheckSampler's own
// context resolver expires it (see CheckSampler.commit).
func (sc *checkSDCCompressor) expire(contextKey ckey.ContextKey) {
	if _, ok := sc.contexts[contextKey]; ok {
		delete(sc.contexts, contextKey)
		sc.tlmContexts.Dec()
	}
}

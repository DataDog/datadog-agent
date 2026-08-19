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

// tlmSDCSamples and tlmSDCBreakpoints count, per check, how many real
// commits-with-a-sample reached the compressor and how many breakpoints it
// shipped. Unlike the earlier sender-level sdcsender implementation this
// replaces, one increment here corresponds to one real per-check flush
// cycle with a sample for that context — already deduplicated by
// Gauge.addSample's keep-last-value semantics upstream, not one increment
// per raw sender call. Their ratio is the compression ratio for that check;
// computed at query time rather than stored directly, since a ratio gauge
// can't be usefully aggregated across time or hosts the way two counters
// can. Deliberately tagged by check_name only, not metric_name: per-metric
// breakdown drove a much larger series count for marginal day-to-day value,
// and this telemetry's own cardinality can otherwise become a sizable
// share of the payload SDC is meant to shrink.
//
// tlmSDCContexts tracks how many distinct contexts (metric+tags
// combinations) are being compressed for a check. Contexts are expired the
// same way any other CheckSampler-tracked context is (see
// checkSDCCompressor.expire), unlike the old sender-level implementation's
// map, which never expired entries.
var exportedSDCMetric = telemetry.Options{DefaultMetric: true}

var (
	tlmSDCSamples = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_sdc", "samples_total",
		[]string{"check_name"},
		"Number of real per-check commits with a sample fed into the SDC compressor, by check name",
		exportedSDCMetric)
	tlmSDCBreakpoints = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
		"checksampler_sdc", "breakpoints_total",
		[]string{"check_name"},
		"Number of breakpoints shipped by the SDC compressor, by check name",
		exportedSDCMetric)
	tlmSDCContexts = telemetryimpl.GetCompatComponent().NewGaugeWithOpts(
		"checksampler_sdc", "contexts",
		[]string{"check_name"},
		"Number of distinct metric contexts being SDC-compressed, by check name",
		exportedSDCMetric)
)

// sdcContextState holds one context's SDC compressor plus its keep-alive
// bookkeeping. Telemetry handles are shared across all contexts for a check
// (see checkSDCCompressor.tlmSamples/tlmBreakpoints), since they're tagged
// by check_name only.
type sdcContextState struct {
	compressor *sdc.Compressor

	// commitsSinceShip counts consecutive commits with a sample but no
	// natural breakpoint; see checkSDCCompressor.keepAliveCommits.
	commitsSinceShip int
}

// checkSDCCompressor holds all SDC compression state for one CheckSampler
// (one check instance). Only ever non-nil on a CheckSampler whose check is
// eligible per sdc.EnabledFor — see newCheckSDCCompressor.
type checkSDCCompressor struct {
	dryRun           bool
	cfg              sdc.Config
	keepAliveCommits int

	tlmSamples     telemetry.SimpleCounter
	tlmBreakpoints telemetry.SimpleCounter
	tlmContexts    telemetry.SimpleGauge

	contexts map[ckey.ContextKey]*sdcContextState
}

// newCheckSDCCompressor returns nil if checkName is not eligible for SDC
// compression, mirroring the once-per-check-creation eligibility decision
// the earlier sender-level sdcsender implementation made in GetSender.
func newCheckSDCCompressor(id checkid.ID) *checkSDCCompressor {
	checkName := checkid.IDToCheckName(id)
	if !sdc.EnabledFor(checkName) {
		return nil
	}
	return &checkSDCCompressor{
		dryRun:           sdc.DryRun(),
		cfg:              sdc.CompressorConfig(),
		keepAliveCommits: sdc.KeepAliveCommits(),
		tlmSamples:       tlmSDCSamples.WithValues(checkName),
		tlmBreakpoints:   tlmSDCBreakpoints.WithValues(checkName),
		tlmContexts:      tlmSDCContexts.WithValues(checkName),
		contexts:         make(map[ckey.ContextKey]*sdcContextState),
	}
}

// noteSample lazily creates per-context compressor state for contextKey,
// keyed by the check-level metric kind (metricSample.Mtype), which is
// unambiguous here — unlike the outgoing Serie.MType seen later in apply,
// which conflates Gauge and Rate (both wire as APIGaugeType). Called from
// addSample for every Gauge/GaugeWithTimestamp sample on an eligible check.
func (sc *checkSDCCompressor) noteSample(contextKey ckey.ContextKey) {
	if _, ok := sc.contexts[contextKey]; ok {
		return
	}
	sc.contexts[contextKey] = &sdcContextState{compressor: sdc.New(sc.cfg)}
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
		sc.tlmSamples.Inc()

		if sc.dryRun {
			// Every point still ships unmodified in dry-run mode.
			kept = append(kept, pt)
			continue
		}
		if len(bps) > 0 {
			st.commitsSinceShip = 0
		}
		for _, bp := range bps {
			kept = append(kept, metrics.Point{Ts: bp.Ts, Value: bp.Value})
			sc.tlmBreakpoints.Inc()
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
			if fw := st.compressor.FlushWindow(lastPt.Ts); len(fw) > 0 {
				kept = append(kept, metrics.Point{Ts: fw[0].Ts, Value: fw[0].Value})
				sc.tlmBreakpoints.Inc()
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

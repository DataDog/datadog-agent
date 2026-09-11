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
// samples reached the compressor and how many breakpoints it shipped.
// Unlike the earlier sender-level sdcsender implementation this replaces,
// one increment here corresponds to one real sample already deduplicated
// by Gauge.addSample's keep-last-value semantics upstream, not one
// increment per raw sender call — true whether that sample arrived via a
// single commit or was batched with others into one flush-time apply()
// call (see checkSDCCompressor.flushPending). Their ratio is the
// compression ratio for that check; computed at query time rather than
// stored directly, since a ratio gauge can't be usefully aggregated across
// time or hosts the way two counters can. Deliberately tagged by check_name
// only, not metric_name: per-metric breakdown drove a much larger series
// count for marginal day-to-day value, and this telemetry's own cardinality
// can otherwise become a sizable share of the payload SDC is meant to
// shrink.
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
		"Number of real samples fed into the SDC compressor, by check name",
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

// sdcContextState holds one context's SDC compressor plus its heartbeat
// bookkeeping. Telemetry handles are shared across all contexts for a check
// (see checkSDCCompressor.tlmSamples/tlmBreakpoints), since they're tagged
// by check_name only.
type sdcContextState struct {
	compressor *sdc.Compressor

	// silentFlushes counts consecutive flush cycles with a pending sample
	// but no natural breakpoint; see checkSDCCompressor.maxSilentFlushes.
	silentFlushes int
}

// checkSDCCompressor holds all SDC compression state for one CheckSampler
// (one check instance). Only ever non-nil on a CheckSampler whose check is
// eligible per sdc.EnabledFor — see newCheckSDCCompressor.
type checkSDCCompressor struct {
	dryRun           bool
	cfg              sdc.Config
	maxSilentFlushes int

	tlmSamples     telemetry.SimpleCounter
	tlmBreakpoints telemetry.SimpleCounter
	tlmContexts    telemetry.SimpleGauge

	contexts map[ckey.ContextKey]*sdcContextState

	// pendingSeries holds fully-resolved (Name/Tags/Host/etc. already set)
	// but not-yet-compressed series accumulated since the last flush, one
	// per tracked context that got at least one sample this window. Drained
	// and compressed once per flush by flushPending — see
	// CheckSampler.flush and CheckSampler.commit's context-expiry loop
	// (which drains a single context early, via expire, if it's about to
	// be forgotten before the next flush arrives).
	pendingSeries map[ckey.ContextKey]*metrics.Serie
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
		maxSilentFlushes: sdc.MaxSilentFlushes(),
		tlmSamples:       tlmSDCSamples.WithValues(checkName),
		tlmBreakpoints:   tlmSDCBreakpoints.WithValues(checkName),
		tlmContexts:      tlmSDCContexts.WithValues(checkName),
		contexts:         make(map[ckey.ContextKey]*sdcContextState),
		pendingSeries:    make(map[ckey.ContextKey]*metrics.Serie),
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

// stashIfTracked accumulates a fully-resolved serie's points into this
// window's pending buffer for its context — merging into an existing
// accumulator if this isn't the first commit to sample that context since
// the last flush — and reports whether it did so. Returns false (does
// nothing) for a context with no compressor state, i.e. not a Gauge-family
// context on this eligible check (see noteSample); the caller (CheckSampler.
// commitSeries) ships those immediately instead of deferring them.
func (sc *checkSDCCompressor) stashIfTracked(serie *metrics.Serie) bool {
	if _, ok := sc.contexts[serie.ContextKey]; !ok {
		return false
	}
	if existing, ok := sc.pendingSeries[serie.ContextKey]; ok {
		existing.Points = append(existing.Points, serie.Points...)
	} else {
		sc.pendingSeries[serie.ContextKey] = serie
	}
	return true
}

// flushPending drains every context's pending points accumulated since the
// last flush, running each through apply() exactly once, and returns the
// series that survive compression. Called once per aggregator flush cycle
// from CheckSampler.flush — this is where checks.sdc_compression_max_silent_flushes
// is actually evaluated, at the same globally-shared cadence for every
// check regardless of each check's own min_collection_interval.
func (sc *checkSDCCompressor) flushPending() []*metrics.Serie {
	if len(sc.pendingSeries) == 0 {
		return nil
	}
	survivors := make([]*metrics.Serie, 0, len(sc.pendingSeries))
	for contextKey, serie := range sc.pendingSeries {
		if sc.apply(contextKey, serie) {
			survivors = append(survivors, serie)
		}
		delete(sc.pendingSeries, contextKey)
	}
	return survivors
}

// apply runs the compression decision for one accumulated serie belonging
// to a tracked (Gauge-family) context, covering every point sampled for it
// since the last flush — exactly one point for a context sampled by a
// single commit, more if either GaugeWithTimestamp produced several points
// in one commit or several commits contributed before the next flush ran
// (see stash). Mutates serie.Points in place to the kept subset; returns
// false when nothing should ship this flush, in which case the caller must
// drop the whole Serie rather than append it.
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

		// Telemetry always reflects what the compressor actually decided,
		// even in dry-run mode — that's the point of dry-run: previewing
		// the real compression ratio without applying it. Only what ships
		// in kept differs below.
		if len(bps) > 0 {
			st.silentFlushes = 0
		}
		for range bps {
			sc.tlmBreakpoints.Inc()
		}

		if sc.dryRun {
			// Every point still ships unmodified in dry-run mode.
			kept = append(kept, pt)
			continue
		}
		for _, bp := range bps {
			kept = append(kept, metrics.Point{Ts: bp.Ts, Value: bp.Value})
		}
	}

	if !sc.dryRun && len(kept) == 0 && len(serie.Points) > 0 {
		// Nothing shipped naturally this flush: consider the heartbeat
		// once per flush (not once per point), using the last point's
		// timestamp as the force-close time.
		st.silentFlushes++
		if sc.maxSilentFlushes > 0 && st.silentFlushes >= sc.maxSilentFlushes {
			st.silentFlushes = 0
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
// context resolver expires it (see CheckSampler.commit), and drains
// whatever serie is still pending for it, running it through the same
// apply() a normal flush would have used. Returns that serie if it
// survives compression, or nil otherwise.
//
// This isn't just cleanup: without draining pending here, a context that
// goes idle and gets expired before the next flush arrives would have its
// still-pending, already-sampled points silently discarded along with the
// rest of its state — real data loss, not intended compression. Running
// them through apply() first means an expiring context's final pending
// points get exactly the natural-breakpoint/heartbeat treatment they'd
// have gotten from a normal flush; nothing new is fabricated, and nothing
// real is dropped.
func (sc *checkSDCCompressor) expire(contextKey ckey.ContextKey) *metrics.Serie {
	var final *metrics.Serie
	if pending, ok := sc.pendingSeries[contextKey]; ok {
		if sc.apply(contextKey, pending) {
			final = pending
		}
		delete(sc.pendingSeries, contextKey)
	}
	if _, ok := sc.contexts[contextKey]; ok {
		delete(sc.contexts, contextKey)
		sc.tlmContexts.Dec()
	}
	return final
}

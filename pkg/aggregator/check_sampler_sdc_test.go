// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/strings"
)

// setSDCTestConfig sets each checks.sdc_compression_* override for the
// duration of the test, restoring whatever value was live beforehand
// (rather than assuming a specific prior value), since
// pkgconfigsetup.Datadog() is a global shared across the whole test binary.
func setSDCTestConfig(t *testing.T, overrides map[string]interface{}) {
	cfg := pkgconfigsetup.Datadog()
	for k, v := range overrides {
		key, val := k, cfg.Get(k)
		cfg.SetInTest(k, v)
		t.Cleanup(func() { cfg.SetInTest(key, val) })
	}
}

// newSDCTestSampler returns a CheckSampler for checkName with a fresh,
// isolated tags.Store and a short context-expiration window (2 commits),
// matching the pattern in check_sampler_test.go's testCheckGaugeSampling.
func newSDCTestSampler(checkName string) *CheckSampler {
	taggerComponent := nooptagger.NewComponent()
	store := tags.NewStore(true, "test")
	return newCheckSampler(2, true, true, time.Second, true, store, checkid.ID(checkName+":1"), taggerComponent)
}

func sdcTagMatcher() filterlist.TagMatcher {
	return filterlistimpl.NewNoopTagMatcher()
}

func addSDCGauge(cs *CheckSampler, name string, value, ts float64, tagList []string) {
	cs.addSample(&metrics.MetricSample{
		Name: name, Value: value, Mtype: metrics.GaugeType,
		Tags: tagList, SampleRate: 1, Timestamp: ts,
	}, sdcTagMatcher())
}

func addSDCGaugeWithTimestamp(cs *CheckSampler, name string, value, ts float64, tagList []string) {
	cs.addSample(&metrics.MetricSample{
		Name: name, Value: value, Mtype: metrics.GaugeWithTimestampType,
		Tags: tagList, SampleRate: 1, Timestamp: ts,
	}, sdcTagMatcher())
}

func sdcCommitAndFlush(cs *CheckSampler, ts float64) metrics.Series {
	matcher := strings.NewMatcher([]string{}, false)
	cs.commit(ts, &matcher)
	series, _ := cs.flush()
	return series
}

func findSDCSerie(series metrics.Series, name string) *metrics.Serie {
	for _, s := range series {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func TestSDC_NotEligibleCheckHasNilCompressor(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":    false,
		"checks.sdc_compression_checks": []string{},
	})
	cs := newSDCTestSampler("not_eligible")
	require.Nil(t, cs.sdcCompressor, "a check not covered by sdc_compression_all/_checks must get no compressor at all")

	addSDCGauge(cs, "my.gauge", 42, 0, nil)
	series := sdcCommitAndFlush(cs, 0)
	require.NotNil(t, findSDCSerie(series, "my.gauge"), "an ineligible check's gauge must ship completely unaffected")
}

func TestSDC_FlatSignalCompressesAcrossCommits(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":                true,
		"checks.sdc_compression_max_silent_flushes": 0, // disable the keep-alive so this test observes pure compression
	})
	cs := newSDCTestSampler("flat_signal")
	require.NotNil(t, cs.sdcCompressor)

	// Warmup (2) ships verbatim.
	addSDCGauge(cs, "my.gauge", 42, 0, nil)
	s := findSDCSerie(sdcCommitAndFlush(cs, 0), "my.gauge")
	require.NotNil(t, s, "warmup sample 1 must ship")
	require.Equal(t, []metrics.Point{{Ts: 0, Value: 42}}, s.Points)

	addSDCGauge(cs, "my.gauge", 42, 1, nil)
	s = findSDCSerie(sdcCommitAndFlush(cs, 1), "my.gauge")
	require.NotNil(t, s, "warmup sample 2 must ship")
	require.Equal(t, []metrics.Point{{Ts: 1, Value: 42}}, s.Points)

	// Post-warmup, flat: no keep-alive configured, so nothing ships.
	addSDCGauge(cs, "my.gauge", 42, 2, nil)
	require.Nil(t, findSDCSerie(sdcCommitAndFlush(cs, 2), "my.gauge"), "a flat post-warmup commit with no keep-alive must swallow its point")

	addSDCGauge(cs, "my.gauge", 42, 3, nil)
	require.Nil(t, findSDCSerie(sdcCommitAndFlush(cs, 3), "my.gauge"), "still flat: still swallowed")

	// A real spike must ship the delayed breakpoint — the LAST point whose
	// trajectory was still consistent with the run (Ts=3, Value=42), not
	// the spike's own value, matching the compressor's own "closed segment
	// ships lastInBounds" design (see sdc.Compressor.Update).
	addSDCGauge(cs, "my.gauge", 5000, 4, nil)
	s = findSDCSerie(sdcCommitAndFlush(cs, 4), "my.gauge")
	require.NotNil(t, s, "a spike must force a breakpoint to ship")
	require.Equal(t, []metrics.Point{{Ts: 3, Value: 42}}, s.Points)
}

func TestSDC_KeepAliveFiresAfterNCommits(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":                true,
		"checks.sdc_compression_max_silent_flushes": 3,
	})
	cs := newSDCTestSampler("keepalive")

	addSDCGauge(cs, "my.gauge", 1, 0, nil) // warmup 1
	sdcCommitAndFlush(cs, 0)
	addSDCGauge(cs, "my.gauge", 1, 1, nil) // warmup 2
	sdcCommitAndFlush(cs, 1)

	addSDCGauge(cs, "my.gauge", 1, 2, nil) // flat commit 1/3
	require.Nil(t, findSDCSerie(sdcCommitAndFlush(cs, 2), "my.gauge"))
	addSDCGauge(cs, "my.gauge", 1, 3, nil) // flat commit 2/3
	require.Nil(t, findSDCSerie(sdcCommitAndFlush(cs, 3), "my.gauge"))
	addSDCGauge(cs, "my.gauge", 1, 4, nil) // flat commit 3/3: keep-alive fires
	s := findSDCSerie(sdcCommitAndFlush(cs, 4), "my.gauge")
	require.NotNil(t, s, "the 3rd consecutive flat commit must force a keep-alive ship")
	require.Equal(t, []metrics.Point{{Ts: 4, Value: 1}}, s.Points)

	// Counter must have reset: the next 2 flat commits stay silent again.
	addSDCGauge(cs, "my.gauge", 1, 5, nil)
	require.Nil(t, findSDCSerie(sdcCommitAndFlush(cs, 5), "my.gauge"), "keep-alive counter must reset after firing")
	addSDCGauge(cs, "my.gauge", 1, 6, nil)
	require.Nil(t, findSDCSerie(sdcCommitAndFlush(cs, 6), "my.gauge"))
}

func TestSDC_KeepAliveDisabledMeansPureCompression(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":                true,
		"checks.sdc_compression_max_silent_flushes": 0,
	})
	cs := newSDCTestSampler("no_keepalive")

	addSDCGauge(cs, "my.gauge", 1, 0, nil)
	sdcCommitAndFlush(cs, 0)
	addSDCGauge(cs, "my.gauge", 1, 1, nil)
	sdcCommitAndFlush(cs, 1)

	for ts := 2.0; ts < 50; ts++ {
		addSDCGauge(cs, "my.gauge", 1, ts, nil)
		require.Nil(t, findSDCSerie(sdcCommitAndFlush(cs, ts), "my.gauge"),
			"with the keep-alive disabled, a flat signal must never force a ship, however many commits pass")
	}
}

func TestSDC_WarmupShipsVerbatim(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":    true,
		"checks.sdc_compression_warmup": 3,
	})
	cs := newSDCTestSampler("warmup")

	for i, v := range []float64{10, 999, 3} { // wildly different values, still all warmup
		addSDCGauge(cs, "my.gauge", v, float64(i), nil)
		s := findSDCSerie(sdcCommitAndFlush(cs, float64(i)), "my.gauge")
		require.NotNil(t, s, "warmup sample %d must ship regardless of magnitude", i+1)
		require.Equal(t, []metrics.Point{{Ts: float64(i), Value: v}}, s.Points)
	}
}

func TestSDC_DryRunShipsUnmodifiedButTelemetryReflectsCompression(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":                true,
		"checks.sdc_compression_dry_run":            true,
		"checks.sdc_compression_max_silent_flushes": 0,
	})
	cs := newSDCTestSampler("dryrun")

	addSDCGauge(cs, "my.gauge", 1, 0, nil)
	sdcCommitAndFlush(cs, 0)
	addSDCGauge(cs, "my.gauge", 1, 1, nil)
	sdcCommitAndFlush(cs, 1)

	for ts := 2.0; ts < 6; ts++ {
		addSDCGauge(cs, "my.gauge", 1, ts, nil)
		s := findSDCSerie(sdcCommitAndFlush(cs, ts), "my.gauge")
		require.NotNil(t, s, "dry-run must ship every point unmodified, even ones real compression would swallow")
		require.Equal(t, []metrics.Point{{Ts: ts, Value: 1}}, s.Points)
	}
	// Only the 2 verbatim warmup ships count as breakpoints so far (same as
	// in normal, non-dry-run mode); the flat run added none.
	require.EqualValues(t, 2, cs.sdcCompressor.tlmBreakpoints.Get())

	// A real spike must still register as one more breakpoint in telemetry,
	// even in dry-run — that's the whole point of dry-run: previewing the
	// real compression ratio without applying it — even though the shipped
	// point is still the raw sample, not the compressed breakpoint's own
	// (different) value.
	addSDCGauge(cs, "my.gauge", 9000, 6, nil)
	s := findSDCSerie(sdcCommitAndFlush(cs, 6), "my.gauge")
	require.NotNil(t, s)
	require.Equal(t, []metrics.Point{{Ts: 6, Value: 9000}}, s.Points, "dry-run ships the raw sample, not the compressed breakpoint's own value")
	require.EqualValues(t, 3, cs.sdcCompressor.tlmBreakpoints.Get(), "the spike must register as one more real breakpoint even though dry-run didn't actually swallow anything")
}

func TestSDC_ContextsTelemetryTracksCreationAndExpiry(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all": true,
	})
	cs := newSDCTestSampler("contexts_lifecycle")
	require.NotNil(t, cs.sdcCompressor)

	addSDCGauge(cs, "my.gauge", 1, 0, nil)
	sdcCommitAndFlush(cs, 0)
	require.Len(t, cs.sdcCompressor.contexts, 1, "a new context must be tracked after its first sample")

	// expirationCount=2 (see newSDCTestSampler): the context expires two
	// commits after the last one that sampled it.
	sdcCommitAndFlush(cs, 1)
	require.Len(t, cs.sdcCompressor.contexts, 1, "must not expire before the configured window elapses")
	sdcCommitAndFlush(cs, 2)
	require.Empty(t, cs.sdcCompressor.contexts, "must expire once the configured window elapses with no new sample")
}

func TestSDC_GaugeWithTimestampMultiPointPerCommit(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":                true,
		"checks.sdc_compression_max_silent_flushes": 0,
	})
	cs := newSDCTestSampler("gauge_with_ts")

	// Two calls with distinct caller-supplied timestamps, both within the
	// SAME commit — unlike plain Gauge, MetricWithTimestamp.addSample
	// appends rather than overwrites, so both survive to flush time as one
	// multi-point Serie, which the SDC hook must filter correctly.
	addSDCGaugeWithTimestamp(cs, "my.gauge_ts", 10, 100, nil) // warmup 1
	addSDCGaugeWithTimestamp(cs, "my.gauge_ts", 10, 101, nil) // warmup 2, same commit
	s := findSDCSerie(sdcCommitAndFlush(cs, 999 /* commit ts irrelevant for GaugeWithTimestamp */), "my.gauge_ts")
	require.NotNil(t, s)
	require.Equal(t, []metrics.Point{{Ts: 100, Value: 10}, {Ts: 101, Value: 10}}, s.Points,
		"both warmup points must ship, preserving their own caller-supplied timestamps")

	// Next commit: a flat point (no keep-alive) followed by a real spike,
	// both within the same commit — only the spike's forced breakpoint
	// (the closed segment's lastInBounds, i.e. the flat point) should ship.
	addSDCGaugeWithTimestamp(cs, "my.gauge_ts", 10, 102, nil)
	addSDCGaugeWithTimestamp(cs, "my.gauge_ts", 9000, 103, nil)
	s = findSDCSerie(sdcCommitAndFlush(cs, 999), "my.gauge_ts")
	require.NotNil(t, s)
	require.Equal(t, []metrics.Point{{Ts: 102, Value: 10}}, s.Points)
}

func TestSDC_NonGaugeFamilySeriesPassThroughUnmodified(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all": true,
	})
	cs := newSDCTestSampler("non_gauge")
	require.NotNil(t, cs.sdcCompressor)

	cs.addSample(&metrics.MetricSample{
		Name: "my.rate", Value: 1, Mtype: metrics.RateType, SampleRate: 1, Timestamp: 0,
	}, sdcTagMatcher())
	cs.addSample(&metrics.MetricSample{
		Name: "my.count", Value: 1, Mtype: metrics.CountType, SampleRate: 1, Timestamp: 0,
	}, sdcTagMatcher())

	require.Empty(t, cs.sdcCompressor.contexts, "Rate/Count samples must never create SDC compressor state")

	series := sdcCommitAndFlush(cs, 1)
	require.NotNil(t, findSDCSerie(series, "my.count"), "a Count series on an eligible check must pass through untouched")
	// Rate needs 2 samples before it produces a serie at all (its own
	// derivative logic, unrelated to SDC) — not asserted here, only that
	// nothing panicked and Count (which needs only 1) shipped normally.
}

func TestSDC_NoSampleThisCommitProducesNoSerieAndNoSpuriousKeepAlive(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":                true,
		"checks.sdc_compression_max_silent_flushes": 2,
	})
	cs := newSDCTestSampler("gap")

	addSDCGauge(cs, "my.gauge", 1, 0, nil) // warmup 1
	sdcCommitAndFlush(cs, 0)
	addSDCGauge(cs, "my.gauge", 1, 1, nil) // warmup 2
	sdcCommitAndFlush(cs, 1)

	// A real gap: no sample at all for this commit. If this incorrectly
	// counted toward silentFlushes, the keep-alive (threshold 2) would
	// fire one commit earlier than it should below.
	require.Nil(t, findSDCSerie(sdcCommitAndFlush(cs, 2), "my.gauge"), "a commit with no sample must produce no serie, not a synthetic keep-alive point")

	// First sampled commit after the gap: only 1 real flat commit so far
	// (the gap didn't count), so the threshold-2 keep-alive must not fire yet.
	addSDCGauge(cs, "my.gauge", 1, 3, nil)
	require.Nil(t, findSDCSerie(sdcCommitAndFlush(cs, 3), "my.gauge"),
		"resuming after a gap must not have silently pre-counted it toward the keep-alive")

	// Second real flat commit after the gap: now the threshold is met.
	addSDCGauge(cs, "my.gauge", 1, 4, nil)
	s := findSDCSerie(sdcCommitAndFlush(cs, 4), "my.gauge")
	require.NotNil(t, s, "the keep-alive must still fire correctly once 2 REAL sampled commits have elapsed")
	require.Equal(t, []metrics.Point{{Ts: 4, Value: 1}}, s.Points)
}

func TestSDC_MultipleCommitsBatchedIntoOneFlush_NaturalBreakpoint(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":                true,
		"checks.sdc_compression_max_silent_flushes": 0,
	})
	cs := newSDCTestSampler("batched_breakpoint")
	matcher := strings.NewMatcher([]string{}, false)

	addSDCGauge(cs, "my.gauge", 42, 0, nil) // warmup 1
	sdcCommitAndFlush(cs, 0)
	addSDCGauge(cs, "my.gauge", 42, 1, nil) // warmup 2
	sdcCommitAndFlush(cs, 1)

	// Two commits with no flush() in between: a flat sample, then a spike.
	// Both must reach the compressor in the SAME flush-time apply() call
	// (checkSDCCompressor.stash accumulates both commits' points), and
	// still produce exactly the breakpoint a per-commit evaluation would
	// have produced.
	addSDCGauge(cs, "my.gauge", 42, 2, nil)
	cs.commit(2, &matcher)
	addSDCGauge(cs, "my.gauge", 5000, 3, nil)
	cs.commit(3, &matcher)

	series, _ := cs.flush()
	s := findSDCSerie(series, "my.gauge")
	require.NotNil(t, s, "a spike batched into the same flush as the flat sample before it must still force a breakpoint")
	require.Equal(t, []metrics.Point{{Ts: 2, Value: 42}}, s.Points,
		"the shipped point must be the closed segment's lastInBounds (Ts=2), exactly as it would be evaluated per-commit")
}

func TestSDC_MultipleCommitsBatchedIntoOneFlush_SilentFlushCountsOnce(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":                true,
		"checks.sdc_compression_max_silent_flushes": 1,
	})
	cs := newSDCTestSampler("batched_silent")
	matcher := strings.NewMatcher([]string{}, false)

	addSDCGauge(cs, "my.gauge", 1, 0, nil) // warmup 1
	sdcCommitAndFlush(cs, 0)
	addSDCGauge(cs, "my.gauge", 1, 1, nil) // warmup 2
	sdcCommitAndFlush(cs, 1)

	// Three flat commits with no flush() in between: with
	// max_silent_flushes=1, this must still force-ship exactly ONE point
	// for the whole flush — not one per underlying commit — since the
	// heartbeat is now evaluated once per flush, not once per commit.
	addSDCGauge(cs, "my.gauge", 1, 2, nil)
	cs.commit(2, &matcher)
	addSDCGauge(cs, "my.gauge", 1, 3, nil)
	cs.commit(3, &matcher)
	addSDCGauge(cs, "my.gauge", 1, 4, nil)
	cs.commit(4, &matcher)

	series, _ := cs.flush()
	s := findSDCSerie(series, "my.gauge")
	require.NotNil(t, s, "a flush with no natural breakpoint must still force a single heartbeat point")
	require.Len(t, s.Points, 1, "exactly one point must ship for the whole flush, not one per underlying commit")
	require.Equal(t, []metrics.Point{{Ts: 4, Value: 1}}, s.Points)
}

func TestSDC_ExpireDrainsPendingWithoutLoss(t *testing.T) {
	setSDCTestConfig(t, map[string]interface{}{
		"checks.sdc_compression_all":                true,
		"checks.sdc_compression_max_silent_flushes": 1,
	})
	cs := newSDCTestSampler("expire_drain")
	matcher := strings.NewMatcher([]string{}, false)

	addSDCGauge(cs, "my.gauge", 1, 0, nil) // warmup 1
	sdcCommitAndFlush(cs, 0)
	addSDCGauge(cs, "my.gauge", 1, 1, nil) // warmup 2
	sdcCommitAndFlush(cs, 1)

	// One more real, post-warmup sample, deliberately left unflushed.
	addSDCGauge(cs, "my.gauge", 1, 2, nil)
	cs.commit(2, &matcher)
	require.Len(t, cs.sdcCompressor.pendingSeries, 1, "the sample must be waiting, unflushed, when the idle countdown below starts")

	// One idle commit (ts=3): keeps the context alive (expirationCount=2,
	// see newSDCTestSampler), must not disturb the still-pending sample.
	cs.commit(3, &matcher)
	require.Len(t, cs.sdcCompressor.pendingSeries, 1, "an idle commit must not touch an unrelated, not-yet-expiring context's pending sample")

	// Second idle commit (ts=4): the context resolver now expires this
	// context, BEFORE any flush() call has ever run. Without expire()
	// draining pendingSeries first, the ts=2 sample would be silently
	// discarded right here instead of surfacing below.
	cs.commit(4, &matcher)
	require.Empty(t, cs.sdcCompressor.contexts, "the context's compressor state must be gone once truly expired")

	series, _ := cs.flush()
	s := findSDCSerie(series, "my.gauge")
	require.NotNil(t, s, "the pending sample from before expiry must still ship, not be silently dropped")
	require.Equal(t, []metrics.Point{{Ts: 2, Value: 1}}, s.Points)
}

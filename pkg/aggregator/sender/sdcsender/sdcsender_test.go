// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sdcsender

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/infratags"
)

// timestampedCall records one call to GaugeWithTimestamp or CountWithTimestamp.
type timestampedCall struct {
	metric    string
	value     float64
	hostname  string
	tags      []string
	timestamp float64
}

// rawCall records one call to a plain (non-timestamped) sender method.
type rawCall struct {
	metric   string
	value    float64
	hostname string
	tags     []string
}

// monotonicCountWithFlushCall records one call to
// MonotonicCountWithFlushFirstValue, including the flushFirstValue flag —
// which a plain rawCall has no field for.
type monotonicCountWithFlushCall struct {
	metric          string
	value           float64
	hostname        string
	tags            []string
	flushFirstValue bool
}

// fakeSender is a minimal sender.Sender: it records GaugeWithTimestamp/
// CountWithTimestamp calls (what sdcsender ships breakpoints through), and
// plain Gauge/Count/Rate/MonotonicCountWithFlushFirstValue calls (what
// dry-run mode forwards unmodified instead), and no-ops everything else.
type fakeSender struct {
	gauges []timestampedCall
	counts []timestampedCall

	rawGauges                   []rawCall
	rawCounts                   []rawCall
	rawRates                    []rawCall
	rawMonotonicCounts          []rawCall
	rawMonotonicCountsWithFlush []monotonicCountWithFlushCall
}

func (f *fakeSender) Commit() {}
func (f *fakeSender) Gauge(metric string, value float64, hostname string, tags []string) {
	f.rawGauges = append(f.rawGauges, rawCall{metric, value, hostname, tags})
}
func (f *fakeSender) GaugeNoIndex(string, float64, string, []string) {}
func (f *fakeSender) Rate(metric string, value float64, hostname string, tags []string) {
	f.rawRates = append(f.rawRates, rawCall{metric, value, hostname, tags})
}
func (f *fakeSender) Count(metric string, value float64, hostname string, tags []string) {
	f.rawCounts = append(f.rawCounts, rawCall{metric, value, hostname, tags})
}
func (f *fakeSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	f.rawMonotonicCounts = append(f.rawMonotonicCounts, rawCall{metric, value, hostname, tags})
}
func (f *fakeSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	f.rawMonotonicCountsWithFlush = append(f.rawMonotonicCountsWithFlush, monotonicCountWithFlushCall{metric, value, hostname, tags, flushFirstValue})
}
func (f *fakeSender) Counter(string, float64, string, []string)      {}
func (f *fakeSender) Histogram(string, float64, string, []string)    {}
func (f *fakeSender) Historate(string, float64, string, []string)    {}
func (f *fakeSender) Distribution(string, float64, string, []string) {}
func (f *fakeSender) ServiceCheck(string, servicecheck.ServiceCheckStatus, string, []string, string) {
}
func (f *fakeSender) OpenmetricsBucket(string, int64, float64, float64, bool, string, []string, bool) {
}
func (f *fakeSender) HistogramBucket(string, int64, float64, float64, bool, string, []string, bool) {
}

func (f *fakeSender) GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	f.gauges = append(f.gauges, timestampedCall{metric, value, hostname, tags, timestamp})
	return nil
}

func (f *fakeSender) CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	f.counts = append(f.counts, timestampedCall{metric, value, hostname, tags, timestamp})
	return nil
}

func (f *fakeSender) Event(event.Event)                                            {}
func (f *fakeSender) EventPlatformEvent([]byte, string)                            {}
func (f *fakeSender) GetSenderStats() stats.SenderStats                            { return stats.SenderStats{} }
func (f *fakeSender) DisableDefaultHostname(bool)                                  {}
func (f *fakeSender) SetCheckCustomTags([]string)                                  {}
func (f *fakeSender) SetCheckService(string)                                       {}
func (f *fakeSender) SetNoIndex(bool)                                              {}
func (f *fakeSender) FinalizeCheckServiceTag()                                     {}
func (f *fakeSender) SetInfraTagger(*infratags.Tagger)                             {}
func (f *fakeSender) OrchestratorMetadata([]types.ProcessMessageBody, string, int) {}
func (f *fakeSender) OrchestratorManifest([]types.ProcessMessageBody, string)      {}

func newTestSender() (*Sender, *fakeSender) {
	fake := &fakeSender{}
	return newSender(fake, false, "my_check", 15*time.Second), fake
}

func newTestSenderDryRun() (*Sender, *fakeSender) {
	fake := &fakeSender{}
	return newSender(fake, true, "my_check", 15*time.Second), fake
}

func TestGauge_FlatSignalCompressesUntilWindowFlush(t *testing.T) {
	s, fake := newTestSender()

	for i := 0; i < 10; i++ {
		s.compressAt(kindGauge, "my.gauge", 42, "host", nil, float64(i), false)
	}
	// Warmup (2) ships verbatim; nothing else changes, so no more
	// breakpoints until a window boundary.
	require.Len(t, fake.gauges, 2)

	// Cross the 15s window boundary: the flat signal's pending point (the
	// last warmup sample) must ship as the window's key point.
	s.compressAt(kindGauge, "my.gauge", 42, "host", nil, 16, false)
	require.Len(t, fake.gauges, 3)
	last := fake.gauges[2]
	require.Equal(t, "my.gauge", last.metric)
	require.Equal(t, 42.0, last.value)
	require.Equal(t, "host", last.hostname)
}

func TestGauge_SpikeShipsViaGaugeWithTimestamp(t *testing.T) {
	s, fake := newTestSender()

	for i := 0; i < 10; i++ {
		v := 100.0
		if i == 5 {
			v = 5000.0
		}
		s.compressAt(kindGauge, "my.gauge", v, "host", []string{"env:prod"}, float64(i), false)
	}

	found := false
	for _, c := range fake.gauges {
		if c.value == 5000.0 {
			found = true
			require.Equal(t, 5.0, c.timestamp)
			require.Equal(t, []string{"env:prod"}, c.tags)
		}
	}
	require.True(t, found, "expected the spike to be shipped as its own breakpoint, got %+v", fake.gauges)
	require.Empty(t, fake.counts, "gauge calls must never ship via CountWithTimestamp")
}

func TestCount_ShipsViaCountWithTimestampNotGauge(t *testing.T) {
	s, fake := newTestSender()

	total := 0.0
	for i := 0; i < 10; i++ {
		v := 1.0
		if i == 5 {
			v = 500.0
		}
		total += v
		s.compressAt(kindCount, "my.count", v, "host", nil, float64(i), false)
	}

	require.Empty(t, fake.gauges, "count calls must never ship via GaugeWithTimestamp")
	require.NotEmpty(t, fake.counts, "expected the spike to force at least one breakpoint to ship")

	shipped := 0.0
	for _, c := range fake.counts {
		shipped += c.value
	}
	ctx := s.contexts[contextKeyFor("my.count", "host", nil)]
	require.InDelta(t, total, shipped+ctx.pendingSum, 1e-9,
		"every received value must be shipped or still pending, never lost")
}

// TestCount_SumIsConservedAcrossWarmupCloseAndWindowFlush drives Count
// through warmup, a segment close (spike), and a window-flush boundary
// (windowDuration == 15s in the sample-timestamp domain) and asserts the
// core pendingSum invariant: nothing received is ever lost, only possibly
// still pending.
func TestCount_SumIsConservedAcrossWarmupCloseAndWindowFlush(t *testing.T) {
	s, fake := newTestSender()

	values := []float64{1, 1, 500, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	total := 0.0
	for i, v := range values {
		total += v
		s.compressAt(kindCount, "my.count", v, "host", nil, float64(i), false)
	}

	shipped := 0.0
	for _, c := range fake.counts {
		shipped += c.value
	}
	ctx := s.contexts[contextKeyFor("my.count", "host", nil)]
	require.InDelta(t, total, shipped+ctx.pendingSum, 1e-9)
}

// TestGaugeWithTimestamp_ShipsCompressedBreakpointsPreservingCallerTimestamp
// mirrors TestGauge_SpikeShipsViaGaugeWithTimestamp: GaugeWithTimestamp
// must compress exactly like Gauge (kindGaugeWithTimestamp is reduced
// identically to kindGauge, see reduce()), and must never fall back to the
// untimestamped Gauge method — only real checks (e.g. the GPU check, which
// stamps samples with their own eBPF collection time) call
// GaugeWithTimestamp, and losing that timestamp would misattribute when a
// compressed value actually occurred.
func TestGaugeWithTimestamp_ShipsCompressedBreakpointsPreservingCallerTimestamp(t *testing.T) {
	s, fake := newTestSender()

	for i := 0; i < 10; i++ {
		v := 100.0
		if i == 5 {
			v = 5000.0
		}
		s.compressAt(kindGaugeWithTimestamp, "my.gauge", v, "host", []string{"env:prod"}, float64(i), false)
	}

	found := false
	for _, c := range fake.gauges {
		if c.value == 5000.0 {
			found = true
			require.Equal(t, 5.0, c.timestamp)
			require.Equal(t, []string{"env:prod"}, c.tags)
		}
	}
	require.True(t, found, "expected the spike to be shipped as its own breakpoint, got %+v", fake.gauges)
	require.Empty(t, fake.counts, "gauge calls must never ship via CountWithTimestamp")
	require.Empty(t, fake.rawGauges, "GaugeWithTimestamp must never fall back to the untimestamped Gauge method")
}

// TestCountWithTimestamp_SumIsConservedAndShipsViaCountWithTimestamp mirrors
// TestCount_SumIsConservedAcrossWarmupCloseAndWindowFlush: CountWithTimestamp
// must accumulate pendingSum exactly like Count (kindCountWithTimestamp is
// included in every pendingSum gate alongside kindCount/kindMonotonicCount).
func TestCountWithTimestamp_SumIsConservedAndShipsViaCountWithTimestamp(t *testing.T) {
	s, fake := newTestSender()

	values := []float64{1, 1, 500, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	total := 0.0
	for i, v := range values {
		total += v
		s.compressAt(kindCountWithTimestamp, "my.count", v, "host", nil, float64(i), false)
	}

	require.Empty(t, fake.gauges, "CountWithTimestamp calls must never ship via GaugeWithTimestamp")
	require.NotEmpty(t, fake.counts)

	shipped := 0.0
	for _, c := range fake.counts {
		shipped += c.value
	}
	ctx := s.contexts[contextKeyFor("my.count", "host", nil)]
	require.InDelta(t, total, shipped+ctx.pendingSum, 1e-9,
		"every received value must be shipped or still pending, never lost, same as plain Count")
}

func TestGaugeWithTimestamp_InvalidTimestampIsRejectedWithoutRecordingSample(t *testing.T) {
	s, fake := newTestSender()

	require.Error(t, s.GaugeWithTimestamp("my.gauge", 42, "host", nil, 0))
	require.Error(t, s.GaugeWithTimestamp("my.gauge", 42, "host", nil, -5))
	require.Empty(t, fake.gauges)
	require.Empty(t, s.contexts, "an invalid-timestamp call must not create a context or touch the compressor")
}

func TestCountWithTimestamp_InvalidTimestampIsRejectedWithoutRecordingSample(t *testing.T) {
	s, fake := newTestSender()

	require.Error(t, s.CountWithTimestamp("my.count", 42, "host", nil, 0))
	require.Empty(t, fake.counts)
	require.Empty(t, s.contexts)
}

// TestDryRun_ForwardsGaugeAndCountWithTimestampPreservingCallerTimestamp is
// the key behavior this whole interception exists for: unlike plain
// Gauge/Count (which dry-run forwards via the untimestamped method, see
// TestDryRun_ForwardsRawGaugeUnmodified), GaugeWithTimestamp/
// CountWithTimestamp must forward via the SAME timestamped method with the
// caller's own timestamp intact — silently replacing it with nowSeconds()
// would misrepresent when a check's sample was actually collected.
func TestDryRun_ForwardsGaugeAndCountWithTimestampPreservingCallerTimestamp(t *testing.T) {
	s, fake := newTestSenderDryRun()

	require.NoError(t, s.GaugeWithTimestamp("my.gauge", 42, "host", []string{"env:prod"}, 12345))
	require.NoError(t, s.CountWithTimestamp("my.count", 7, "host", nil, 67890))

	require.Len(t, fake.gauges, 1, "GaugeWithTimestamp must forward via the same timestamped method in dry-run mode, unlike plain Gauge")
	require.Equal(t, 42.0, fake.gauges[0].value)
	require.Equal(t, 12345.0, fake.gauges[0].timestamp, "the caller's own timestamp must be preserved, not replaced by nowSeconds()")
	require.Equal(t, []string{"env:prod"}, fake.gauges[0].tags)

	require.Len(t, fake.counts, 1)
	require.Equal(t, 7.0, fake.counts[0].value)
	require.Equal(t, 67890.0, fake.counts[0].timestamp)

	require.Empty(t, fake.rawGauges, "must never fall back to the untimestamped Gauge method")
	require.Empty(t, fake.rawCounts, "must never fall back to the untimestamped Count method")
}

func TestRate_FirstSampleProducesNoValue(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindRate, "my.rate", 100, "host", nil, 0, false)
	require.Empty(t, fake.gauges, "a lone first Rate sample has no previous sample to derive a rate from")
}

func TestRate_ComputesDerivativeLocally(t *testing.T) {
	s, fake := newTestSender()

	// raw counter goes 100 -> 200 over 10s => rate of 10/s. Warmup(2) ships
	// both computed rate points verbatim regardless of magnitude.
	s.compressAt(kindRate, "my.rate", 100, "host", nil, 0, false)
	s.compressAt(kindRate, "my.rate", 200, "host", nil, 10, false)

	require.Len(t, fake.gauges, 1)
	require.InDelta(t, 10.0, fake.gauges[0].value, 1e-9)
	require.Equal(t, 10.0, fake.gauges[0].timestamp)
}

func TestRate_NegativeRateIsTreatedAsReset(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindRate, "my.rate", 200, "host", nil, 0, false)
	// counter went down: underlying raw counter must have reset.
	s.compressAt(kindRate, "my.rate", 100, "host", nil, 10, false)

	require.Empty(t, fake.gauges, "a negative derivative must be dropped, not shipped as a negative rate")
}

func TestMonotonicCount_ComputesDiffLocally(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindMonotonicCount, "my.mc", 10, "host", nil, 0, false)
	require.Empty(t, fake.counts, "first sample has no previous value to diff against")

	s.compressAt(kindMonotonicCount, "my.mc", 16, "host", nil, 1, false)
	require.Len(t, fake.counts, 1)
	require.InDelta(t, 6.0, fake.counts[0].value, 1e-9)
}

func TestMonotonicCount_ResetIsDropped(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindMonotonicCount, "my.mc", 100, "host", nil, 0, false)
	// raw counter reset back to a lower value.
	s.compressAt(kindMonotonicCount, "my.mc", 5, "host", nil, 1, false)

	require.Empty(t, fake.counts, "a reset (decreasing raw value) must be dropped, not shipped as a negative diff")
}

// TestMonotonicCount_SumIsConservedAcrossWarmupCloseAndWindowFlush mirrors
// TestCount_SumIsConservedAcrossWarmupCloseAndWindowFlush for
// MonotonicCount's locally-diffed values.
func TestMonotonicCount_SumIsConservedAcrossWarmupCloseAndWindowFlush(t *testing.T) {
	s, fake := newTestSender()

	raw := []float64{10, 16, 1000, 1001, 1002, 1003, 1004, 1005, 1006, 1007, 1008, 1009, 1010, 1011, 1012, 1013, 1014, 1015, 1016, 1017}
	totalDiff := 0.0
	for i := 1; i < len(raw); i++ {
		if d := raw[i] - raw[i-1]; d >= 0 {
			totalDiff += d
		}
	}
	for i, v := range raw {
		s.compressAt(kindMonotonicCount, "my.mc", v, "host", nil, float64(i), false)
	}

	shipped := 0.0
	for _, c := range fake.counts {
		shipped += c.value
	}
	ctx := s.contexts[contextKeyFor("my.mc", "host", nil)]
	require.InDelta(t, totalDiff, shipped+ctx.pendingSum, 1e-9)
}

func TestMonotonicCount_FlushFirstValueShipsFirstSampleImmediately(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindMonotonicCount, "my.mc", 42, "host", nil, 0, true)

	require.Len(t, fake.counts, 1, "flushFirstValue must ship the very first sample instead of waiting for a second to diff against")
	require.Equal(t, 42.0, fake.counts[0].value)
}

func TestMonotonicCount_FlushFirstValueShipsResetBaseline(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindMonotonicCount, "my.mc", 100, "host", nil, 0, false)
	require.Empty(t, fake.counts, "first sample has no previous value to diff against")

	// raw counter reset back to a lower value, with flushFirstValue set: the
	// new value must ship as the reset baseline, not be dropped.
	s.compressAt(kindMonotonicCount, "my.mc", 5, "host", nil, 1, true)

	require.Len(t, fake.counts, 1)
	require.Equal(t, 5.0, fake.counts[0].value)
}

// TestMonotonicCount_ResetReplacesRatherThanAddsToPendingSum is a
// regression test for a real bug flagged in PR review: on a
// flushFirstValue-triggered counter reset, reduce() returns the reset
// baseline as an absolute "count since reset" value, not a delta — so
// compressAt must REPLACE pendingSum with it rather than adding it to
// whatever accumulated earlier in the same window. Mirrors the reported
// counter sequence 100 -> 110 -> 3 (a delta of 10 left pending when the
// reset to baseline 3 arrives): the correct value that ends up shipped is
// 3, not 10+3=13.
func TestMonotonicCount_ResetReplacesRatherThanAddsToPendingSum(t *testing.T) {
	s, fake := newTestSender()

	// Establish the context with a previous raw value of 110, then force
	// a known pending delta of 10 — simulating an earlier, not-yet-shipped
	// sample accumulated in the same window.
	s.compressAt(kindMonotonicCount, "my.mc", 100, "host", nil, 0, false)
	s.compressAt(kindMonotonicCount, "my.mc", 110, "host", nil, 1, false)
	ctx := s.contexts[contextKeyFor("my.mc", "host", nil)]
	ctx.pendingSum = 10

	// Reset to 3 with flushFirstValue: must replace pendingSum with the
	// reset baseline (3), not add to the pre-existing pending delta. This
	// call happens to land on the compressor's own warmup ship (Warmup
	// defaults to 2), so it ships immediately — asserting on the shipped
	// value is what actually exercises the fix, since pendingSum is
	// correctly drained back to 0 right after being set.
	s.compressAt(kindMonotonicCount, "my.mc", 3, "host", nil, 2, true)

	require.Len(t, fake.counts, 2, "the 100->110 delta ships first (its own warmup breakpoint), then the reset")
	require.Equal(t, 3.0, fake.counts[1].value,
		"a counter reset must replace pendingSum with the reset baseline, not add to it (would otherwise ship 13 instead of 3)")
	require.Zero(t, ctx.pendingSum, "pendingSum must be drained after shipping")
}

func TestWindowFlush_DrivenBySampleTimestampsNotWallClock(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 0, false)
	require.Len(t, fake.gauges, 1, "warmup ships the first sample verbatim")

	// Still well inside the window: no extra ship for a flat signal.
	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 1, false)
	require.Len(t, fake.gauges, 2, "warmup(2) ships the second sample verbatim too")

	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 5, false)
	require.Len(t, fake.gauges, 2, "flat signal after warmup: no new breakpoint before the window boundary")

	// Cross 15 sample-seconds since the last flush (t=0): must force-close.
	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 16, false)
	require.Len(t, fake.gauges, 3, "window boundary crossed: the pending point must ship")
}

// TestWindowFlush_DurationIsConfigurable is a regression test for
// checks.sdc_compression_window_duration: the force-flush boundary must
// track whatever duration the Sender was constructed with, not the
// previous hardcoded 15s.
func TestWindowFlush_DurationIsConfigurable(t *testing.T) {
	fake := &fakeSender{}
	s := newSender(fake, false, "my_check", 5*time.Second)

	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 0, false)
	require.Len(t, fake.gauges, 1, "warmup ships the first sample verbatim")
	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 1, false)
	require.Len(t, fake.gauges, 2, "warmup(2) ships the second sample verbatim too")

	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 4, false)
	require.Len(t, fake.gauges, 2, "still inside the configured 5s window: no new breakpoint yet")

	// Cross the configured 5 sample-seconds (not the old 15s default):
	// must force-close.
	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 6, false)
	require.Len(t, fake.gauges, 3, "the shorter, configured window boundary crossed: the pending point must ship")
}

func TestWrap_ReadsWindowDurationFromConfig(t *testing.T) {
	cfg := pkgconfigsetup.Datadog()
	cfg.SetInTest("checks.sdc_compression_window_duration", 5)
	t.Cleanup(func() { cfg.SetInTest("checks.sdc_compression_window_duration", 15) })

	m := Wrap(nil, false)
	require.Equal(t, 5*time.Second, m.windowDuration)
}

func TestDifferentTagsAreIndependentContexts(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindGauge, "my.gauge", 1, "host", []string{"env:prod"}, 0, false)
	s.compressAt(kindGauge, "my.gauge", 999, "host", []string{"env:staging"}, 0, false)

	require.Len(t, fake.gauges, 2, "different tag sets must not share compressor state")
}

func TestTagOrderDoesNotCreateNewContext(t *testing.T) {
	s, _ := newTestSender()

	s.compressAt(kindGauge, "my.gauge", 1, "host", []string{"a:1", "b:2"}, 0, false)
	s.compressAt(kindGauge, "my.gauge", 1, "host", []string{"b:2", "a:1"}, 1, false)
	// Warmup(2) ships both verbatim regardless; the point of this test is
	// that both calls hit the SAME context (single entry), not two.
	require.Len(t, s.contexts, 1)
}

func TestOtherSenderMethodsPassThroughUnmodified(t *testing.T) {
	s, fake := newTestSender()

	s.Commit()
	s.SetNoIndex(true)
	s.Histogram("my.histogram", 1, "host", nil)

	require.Empty(t, fake.gauges)
	require.Empty(t, fake.counts)
}

func TestDryRun_ForwardsRawGaugeUnmodified(t *testing.T) {
	s, fake := newTestSenderDryRun()

	for i := 0; i < 10; i++ {
		s.compressAt(kindGauge, "my.gauge", 42, "host", []string{"env:prod"}, float64(i), false)
	}

	require.Len(t, fake.rawGauges, 10, "every raw call must be forwarded unmodified in dry-run mode")
	for _, c := range fake.rawGauges {
		require.Equal(t, "my.gauge", c.metric)
		require.Equal(t, 42.0, c.value)
		require.Equal(t, []string{"env:prod"}, c.tags)
	}
	require.Empty(t, fake.gauges, "dry-run must never actually ship a compressed breakpoint")
	require.Empty(t, fake.counts, "dry-run must never actually ship a compressed breakpoint")
}

func TestDryRun_ForwardsRawCountRateMonotonicCountToTheirOwnMethods(t *testing.T) {
	s, fake := newTestSenderDryRun()

	s.compressAt(kindCount, "my.count", 5, "host", nil, 0, false)
	s.compressAt(kindRate, "my.rate", 100, "host", nil, 0, false)
	s.compressAt(kindMonotonicCount, "my.mc", 10, "host", nil, 0, false)

	require.Len(t, fake.rawCounts, 1)
	require.Equal(t, 5.0, fake.rawCounts[0].value)
	require.Len(t, fake.rawRates, 1)
	require.Equal(t, 100.0, fake.rawRates[0].value, "Rate forwards the RAW value, not sdcsender's locally-reduced derivative — the real sender does its own diffing")

	// MonotonicCount always forwards via MonotonicCountWithFlushFirstValue
	// (never the plain MonotonicCount method), since that form is
	// behaviorally identical when flushFirstValue is false.
	require.Empty(t, fake.rawMonotonicCounts)
	require.Len(t, fake.rawMonotonicCountsWithFlush, 1)
	require.Equal(t, 10.0, fake.rawMonotonicCountsWithFlush[0].value, "MonotonicCount forwards the RAW cumulative value, not sdcsender's locally-reduced diff")
	require.False(t, fake.rawMonotonicCountsWithFlush[0].flushFirstValue)

	require.Empty(t, fake.gauges)
	require.Empty(t, fake.counts)
}

func TestDryRun_ForwardsFlushFirstValueFlag(t *testing.T) {
	s, fake := newTestSenderDryRun()

	s.compressAt(kindMonotonicCount, "my.mc", 10, "host", nil, 0, true)

	require.Len(t, fake.rawMonotonicCountsWithFlush, 1)
	require.True(t, fake.rawMonotonicCountsWithFlush[0].flushFirstValue, "the flushFirstValue flag must be forwarded unmodified in dry-run mode")
}

func TestDryRun_StillMeasuresCompressionViaTelemetryOnly(t *testing.T) {
	s, fake := newTestSenderDryRun()

	for i := 0; i < 10; i++ {
		s.compressAt(kindGauge, "my.gauge", 42, "host", nil, float64(i), false)
	}

	// The underlying compressor still ran (warmup(2) would have produced its
	// own breakpoints in live mode); confirm state advanced normally, just
	// without shipping anything itself.
	ctx := s.contexts[contextKeyFor("my.gauge", "host", nil)]
	require.NotNil(t, ctx)
	require.NotNil(t, ctx.compressor)
	require.Empty(t, fake.gauges)
	require.Len(t, fake.rawGauges, 10)
}

func TestTlmContexts_TracksDistinctContextCountPerSender(t *testing.T) {
	// A dedicated check name, not shared with newTestSender()'s "my_check":
	// tlmContexts is a process-global telemetry gauge keyed by check name,
	// so reusing a name other tests already incremented would make this
	// test's absolute-value assertions flaky.
	fake := &fakeSender{}
	s := newSender(fake, false, "check_tlm_contexts_test", 15*time.Second)

	require.Equal(t, 0.0, s.tlmContexts.Get())

	s.compressAt(kindGauge, "my.gauge", 1, "host", []string{"env:prod"}, 0, false)
	require.Equal(t, 1.0, s.tlmContexts.Get())

	// Same metric, same tags (different order): must not count as a new
	// context.
	s.compressAt(kindGauge, "my.gauge", 1, "host", []string{"env:prod"}, 1, false)
	require.Equal(t, 1.0, s.tlmContexts.Get())

	// Different tags: a genuinely new context.
	s.compressAt(kindGauge, "my.gauge", 1, "host", []string{"env:staging"}, 0, false)
	require.Equal(t, 2.0, s.tlmContexts.Get())

	// A different metric entirely: another new context.
	s.compressAt(kindCount, "my.count", 1, "host", nil, 0, false)
	require.Equal(t, 3.0, s.tlmContexts.Get())

	// Contexts never expire: repeating earlier calls must not double-count.
	s.compressAt(kindGauge, "my.gauge", 2, "host", []string{"env:prod"}, 2, false)
	require.Equal(t, 3.0, s.tlmContexts.Get())
}

func TestTlmScaleDeviation_ObservesAbsoluteDiffFromScale(t *testing.T) {
	// A dedicated check name: tlmScaleDeviationSum/Count are process-global
	// counters keyed by (check_name, metric_name), so reusing a
	// (check_name, metric_name) pair another test already observed into
	// would make this test's exact Count/Sum assertions flaky.
	fake := &fakeSender{}
	s := newSender(fake, false, "check_scale_deviation_test", 15*time.Second)

	values := []float64{10, 20, 15, 100, 12}
	alpha := compressorConfig().Alpha
	var scale float64
	var hasScale bool
	expectedSum := 0.0
	for i, v := range values {
		s.compressAt(kindGauge, "my.gauge", v, "host", nil, float64(i), false)

		// Independently mirrors sdc.Compressor's own EWMA update (see
		// pkg/aggregator/internal/sdc's updateScaleAndTolerance), rather
		// than reading it back via Scale(), so this test actually exercises
		// the wiring instead of only restating whatever the compressor
		// already computed.
		abs := math.Abs(v)
		if !hasScale {
			scale, hasScale = abs, true
		} else {
			scale = alpha*abs + (1-alpha)*scale
		}
		expectedSum += math.Abs(v - scale)
	}

	ctx := s.contexts[contextKeyFor("my.gauge", "host", nil)]
	require.EqualValues(t, len(values), ctx.tlmScaleDeviationCount.Get(), "every Gauge sample must be observed exactly once")
	require.InDelta(t, expectedSum, ctx.tlmScaleDeviationSum.Get(), 1e-9)
}

func TestFloorBoundTelemetry_TracksSwallowedPointsWhenFloorDominates(t *testing.T) {
	// A dedicated check name: see TestTlmScaleDeviation_ObservesAbsoluteDiffFromScale.
	fake := &fakeSender{}
	s := newSender(fake, false, "check_floor_bound_dominates_test", 15*time.Second)

	// A near-zero-scale signal: with the default config (Epsilon=0.02,
	// Floor=1e-3), Epsilon*scale (~2e-8) is many orders of magnitude below
	// Floor, so every sample here is floor-bound.
	for i := 0; i < 20; i++ {
		s.compressAt(kindGauge, "my.tiny_gauge", 1e-6, "host", nil, float64(i), false)
	}

	ctx := s.contexts[contextKeyFor("my.tiny_gauge", "host", nil)]
	require.EqualValues(t, 20, ctx.tlmSamples.Get())
	require.EqualValues(t, 20, ctx.tlmFloorBoundSamples.Get(), "every sample should be floor-bound given the signal's tiny scale")
	require.NotZero(t, ctx.tlmBreakpoints.Get())
	require.Equal(t, ctx.tlmBreakpoints.Get(), ctx.tlmFloorBoundBreakpoints.Get(), "every breakpoint shipped for this signal must also be floor-bound")
}

func TestFloorBoundTelemetry_DisabledWhenFloorIsZero(t *testing.T) {
	pkgconfigsetup.Datadog().SetInTest("checks.sdc_compression_floor", 0.0)
	t.Cleanup(func() { pkgconfigsetup.Datadog().SetInTest("checks.sdc_compression_floor", 1e-3) })

	fake := &fakeSender{}
	s := newSender(fake, false, "check_floor_bound_disabled_test", 15*time.Second)

	for i := 0; i < 20; i++ {
		s.compressAt(kindGauge, "my.tiny_gauge", 1e-6, "host", nil, float64(i), false)
	}

	ctx := s.contexts[contextKeyFor("my.tiny_gauge", "host", nil)]
	require.EqualValues(t, 20, ctx.tlmSamples.Get())
	require.Zero(t, ctx.tlmFloorBoundSamples.Get(), "with Floor disabled, no sample should ever be floor-bound")
	require.Zero(t, ctx.tlmFloorBoundBreakpoints.Get())
}

func TestTwoSendersHaveIndependentContextCounts(t *testing.T) {
	fakeA := &fakeSender{}
	sA := newSender(fakeA, false, "check_a", 15*time.Second)
	fakeB := &fakeSender{}
	sB := newSender(fakeB, false, "check_b", 15*time.Second)

	sA.compressAt(kindGauge, "my.gauge", 1, "host", nil, 0, false)
	sA.compressAt(kindGauge, "my.gauge2", 1, "host", nil, 0, false)
	sB.compressAt(kindGauge, "my.gauge", 1, "host", nil, 0, false)

	require.Equal(t, 2.0, sA.tlmContexts.Get())
	require.Equal(t, 1.0, sB.tlmContexts.Get())
}

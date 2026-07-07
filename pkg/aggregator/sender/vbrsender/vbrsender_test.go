// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package vbrsender

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
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
// CountWithTimestamp calls (what vbrsender ships breakpoints through), and
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
func (f *fakeSender) OrchestratorMetadata([]types.ProcessMessageBody, string, int) {}
func (f *fakeSender) OrchestratorManifest([]types.ProcessMessageBody, string)      {}

func newTestSender() (*Sender, *fakeSender) {
	fake := &fakeSender{}
	return newSender(fake, false, "my_check", "", ""), fake
}

func newTestSenderDryRun() (*Sender, *fakeSender) {
	fake := &fakeSender{}
	return newSender(fake, true, "my_check", "", ""), fake
}

func newTestSenderShadow(shadowHostSuffix, defaultHostname string) (*Sender, *fakeSender) {
	fake := &fakeSender{}
	return newSender(fake, false, "my_check", shadowHostSuffix, defaultHostname), fake
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
	require.Equal(t, 100.0, fake.rawRates[0].value, "Rate forwards the RAW value, not vbrsender's locally-reduced derivative — the real sender does its own diffing")

	// MonotonicCount always forwards via MonotonicCountWithFlushFirstValue
	// (never the plain MonotonicCount method), since that form is
	// behaviorally identical when flushFirstValue is false.
	require.Empty(t, fake.rawMonotonicCounts)
	require.Len(t, fake.rawMonotonicCountsWithFlush, 1)
	require.Equal(t, 10.0, fake.rawMonotonicCountsWithFlush[0].value, "MonotonicCount forwards the RAW cumulative value, not vbrsender's locally-reduced diff")
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
	s := newSender(fake, false, "check_tlm_contexts_test", "", "")

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
	s := newSender(fake, false, "check_scale_deviation_test", "", "")

	values := []float64{10, 20, 15, 100, 12}
	var scale float64
	var hasScale bool
	expectedSum := 0.0
	for i, v := range values {
		s.compressAt(kindGauge, "my.gauge", v, "host", nil, float64(i), false)

		// Independently mirrors vbr.Compressor's own EWMA update (see
		// pkg/aggregator/internal/vbr's updateScaleAndTolerance), rather
		// than reading it back via Scale(), so this test actually exercises
		// the wiring instead of only restating whatever the compressor
		// already computed.
		abs := math.Abs(v)
		if !hasScale {
			scale, hasScale = abs, true
		} else {
			scale = defaultConfig.Alpha*abs + (1-defaultConfig.Alpha)*scale
		}
		expectedSum += math.Abs(v - scale)
	}

	ctx := s.contexts[contextKeyFor("my.gauge", "host", nil)]
	require.EqualValues(t, len(values), ctx.tlmScaleDeviationCount.Get(), "every Gauge sample must be observed exactly once")
	require.InDelta(t, expectedSum, ctx.tlmScaleDeviationSum.Get(), 1e-9)
}

func TestTwoSendersHaveIndependentContextCounts(t *testing.T) {
	fakeA := &fakeSender{}
	sA := newSender(fakeA, false, "check_a", "", "")
	fakeB := &fakeSender{}
	sB := newSender(fakeB, false, "check_b", "", "")

	sA.compressAt(kindGauge, "my.gauge", 1, "host", nil, 0, false)
	sA.compressAt(kindGauge, "my.gauge2", 1, "host", nil, 0, false)
	sB.compressAt(kindGauge, "my.gauge", 1, "host", nil, 0, false)

	require.Equal(t, 2.0, sA.tlmContexts.Get())
	require.Equal(t, 1.0, sB.tlmContexts.Get())
}

func TestShadow_ShipsBothRawAndCompressedUnderDifferentHostnames(t *testing.T) {
	s, fake := newTestSenderShadow("-vbr", "agent-host")

	for i := 0; i < 10; i++ {
		s.compressAt(kindGauge, "my.gauge", 42, "check-host", []string{"env:prod"}, float64(i), false)
	}

	require.Len(t, fake.rawGauges, 10, "shadow mode must still ship every raw call unmodified, like dry-run")
	for _, c := range fake.rawGauges {
		require.Equal(t, "check-host", c.hostname, "the raw series must ship under the check's real hostname")
	}

	require.NotEmpty(t, fake.gauges, "shadow mode must also ship the compressed breakpoints, unlike dry-run")
	for _, c := range fake.gauges {
		require.Equal(t, "check-host-vbr", c.hostname, "the compressed series must ship under hostname+suffix")
	}
}

func TestShadow_FallsBackToDefaultHostnameWhenCheckHostnameEmpty(t *testing.T) {
	s, fake := newTestSenderShadow("-vbr", "agent-host")

	s.compressAt(kindGauge, "my.gauge", 42, "", nil, 0, false)

	require.Len(t, fake.gauges, 1)
	require.Equal(t, "agent-host-vbr", fake.gauges[0].hostname,
		"an empty check hostname must fall back to the agent's resolved default before appending the suffix, matching what the real sender fills in downstream for the raw series")
}

func TestShadow_ShipsViaCountWithTimestampToo(t *testing.T) {
	s, fake := newTestSenderShadow("-vbr", "agent-host")

	s.compressAt(kindMonotonicCount, "my.mc", 10, "check-host", nil, 0, true)

	require.Len(t, fake.rawMonotonicCountsWithFlush, 1, "shadow mode ships the raw call through the same path dry-run uses")
	require.Len(t, fake.counts, 1, "shadow mode ships the compressed breakpoint too")
	require.Equal(t, "check-host-vbr", fake.counts[0].hostname)
}

func TestShadow_TakesPrecedenceOverDryRun(t *testing.T) {
	fake := &fakeSender{}
	s := newSender(fake, true /* dryRun */, "my_check", "-vbr", "agent-host")

	s.compressAt(kindGauge, "my.gauge", 42, "check-host", nil, 0, false)

	require.Len(t, fake.rawGauges, 1, "the raw call must ship exactly once, not duplicated by both dryRun and shadow forwarding it")
	require.Len(t, fake.gauges, 1, "shadow mode must ship the compressed breakpoint even though dryRun is also true")
	require.Equal(t, "check-host-vbr", fake.gauges[0].hostname)
}

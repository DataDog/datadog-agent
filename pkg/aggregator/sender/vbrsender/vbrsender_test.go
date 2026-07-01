// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package vbrsender

import (
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

// fakeSender is a minimal sender.Sender: it records GaugeWithTimestamp/
// CountWithTimestamp calls (what vbrsender ships breakpoints through) and
// no-ops everything else.
type fakeSender struct {
	gauges []timestampedCall
	counts []timestampedCall
}

func (f *fakeSender) Commit()                                                                   {}
func (f *fakeSender) Gauge(string, float64, string, []string)                                   {}
func (f *fakeSender) GaugeNoIndex(string, float64, string, []string)                            {}
func (f *fakeSender) Rate(string, float64, string, []string)                                    {}
func (f *fakeSender) Count(string, float64, string, []string)                                   {}
func (f *fakeSender) MonotonicCount(string, float64, string, []string)                          {}
func (f *fakeSender) MonotonicCountWithFlushFirstValue(string, float64, string, []string, bool) {}
func (f *fakeSender) Counter(string, float64, string, []string)                                 {}
func (f *fakeSender) Histogram(string, float64, string, []string)                               {}
func (f *fakeSender) Historate(string, float64, string, []string)                               {}
func (f *fakeSender) Distribution(string, float64, string, []string)                            {}
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
	return newSender(fake), fake
}

func TestGauge_FlatSignalCompressesUntilWindowFlush(t *testing.T) {
	s, fake := newTestSender()

	for i := 0; i < 10; i++ {
		s.compressAt(kindGauge, "my.gauge", 42, "host", nil, float64(i))
	}
	// Warmup (2) ships verbatim; nothing else changes, so no more
	// breakpoints until a window boundary.
	require.Len(t, fake.gauges, 2)

	// Cross the 15s window boundary: the flat signal's pending point (the
	// last warmup sample) must ship as the window's key point.
	s.compressAt(kindGauge, "my.gauge", 42, "host", nil, 16)
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
		s.compressAt(kindGauge, "my.gauge", v, "host", []string{"env:prod"}, float64(i))
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

	for i := 0; i < 10; i++ {
		v := 1.0
		if i == 5 {
			v = 500.0
		}
		s.compressAt(kindCount, "my.count", v, "host", nil, float64(i))
	}

	require.Empty(t, fake.gauges, "count calls must never ship via GaugeWithTimestamp")
	found := false
	for _, c := range fake.counts {
		if c.value == 500.0 {
			found = true
		}
	}
	require.True(t, found, "expected the count spike to be shipped, got %+v", fake.counts)
}

func TestRate_FirstSampleProducesNoValue(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindRate, "my.rate", 100, "host", nil, 0)
	require.Empty(t, fake.gauges, "a lone first Rate sample has no previous sample to derive a rate from")
}

func TestRate_ComputesDerivativeLocally(t *testing.T) {
	s, fake := newTestSender()

	// raw counter goes 100 -> 200 over 10s => rate of 10/s. Warmup(2) ships
	// both computed rate points verbatim regardless of magnitude.
	s.compressAt(kindRate, "my.rate", 100, "host", nil, 0)
	s.compressAt(kindRate, "my.rate", 200, "host", nil, 10)

	require.Len(t, fake.gauges, 1)
	require.InDelta(t, 10.0, fake.gauges[0].value, 1e-9)
	require.Equal(t, 10.0, fake.gauges[0].timestamp)
}

func TestRate_NegativeRateIsTreatedAsReset(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindRate, "my.rate", 200, "host", nil, 0)
	// counter went down: underlying raw counter must have reset.
	s.compressAt(kindRate, "my.rate", 100, "host", nil, 10)

	require.Empty(t, fake.gauges, "a negative derivative must be dropped, not shipped as a negative rate")
}

func TestMonotonicCount_ComputesDiffLocally(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindMonotonicCount, "my.mc", 10, "host", nil, 0)
	require.Empty(t, fake.counts, "first sample has no previous value to diff against")

	s.compressAt(kindMonotonicCount, "my.mc", 16, "host", nil, 1)
	require.Len(t, fake.counts, 1)
	require.InDelta(t, 6.0, fake.counts[0].value, 1e-9)
}

func TestMonotonicCount_ResetIsDropped(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindMonotonicCount, "my.mc", 100, "host", nil, 0)
	// raw counter reset back to a lower value.
	s.compressAt(kindMonotonicCount, "my.mc", 5, "host", nil, 1)

	require.Empty(t, fake.counts, "a reset (decreasing raw value) must be dropped, not shipped as a negative diff")
}

func TestWindowFlush_DrivenBySampleTimestampsNotWallClock(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 0)
	require.Len(t, fake.gauges, 1, "warmup ships the first sample verbatim")

	// Still well inside the window: no extra ship for a flat signal.
	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 1)
	require.Len(t, fake.gauges, 2, "warmup(2) ships the second sample verbatim too")

	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 5)
	require.Len(t, fake.gauges, 2, "flat signal after warmup: no new breakpoint before the window boundary")

	// Cross 15 sample-seconds since the last flush (t=0): must force-close.
	s.compressAt(kindGauge, "my.gauge", 1, "host", nil, 16)
	require.Len(t, fake.gauges, 3, "window boundary crossed: the pending point must ship")
}

func TestDifferentTagsAreIndependentContexts(t *testing.T) {
	s, fake := newTestSender()

	s.compressAt(kindGauge, "my.gauge", 1, "host", []string{"env:prod"}, 0)
	s.compressAt(kindGauge, "my.gauge", 999, "host", []string{"env:staging"}, 0)

	require.Len(t, fake.gauges, 2, "different tag sets must not share compressor state")
}

func TestTagOrderDoesNotCreateNewContext(t *testing.T) {
	s, _ := newTestSender()

	s.compressAt(kindGauge, "my.gauge", 1, "host", []string{"a:1", "b:2"}, 0)
	s.compressAt(kindGauge, "my.gauge", 1, "host", []string{"b:2", "a:1"}, 1)
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

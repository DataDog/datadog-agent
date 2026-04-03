// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package sender

import (
	"testing"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
)

// capturingHandle records every MetricView passed to ObserveMetric.
type capturingHandle struct {
	samples []observerdef.MetricView
}

func (h *capturingHandle) ObserveMetric(s observerdef.MetricView)         { h.samples = append(h.samples, s) }
func (h *capturingHandle) ObserveLog(_ observerdef.LogView)               {}
func (h *capturingHandle) ObserveTrace(_ observerdef.TraceView)           {}
func (h *capturingHandle) ObserveTraceStats(_ observerdef.TraceStatsView) {}
func (h *capturingHandle) ObserveProfile(_ observerdef.ProfileView)       {}
func (h *capturingHandle) ObserveLifecycle(_ observerdef.LifecycleView)   {}

// noopSender satisfies Sender and discards everything.
type noopSender struct{}

func (noopSender) Commit()                                                  {}
func (noopSender) Gauge(_ string, _ float64, _ string, _ []string)          {}
func (noopSender) GaugeNoIndex(_ string, _ float64, _ string, _ []string)   {}
func (noopSender) Rate(_ string, _ float64, _ string, _ []string)           {}
func (noopSender) Count(_ string, _ float64, _ string, _ []string)          {}
func (noopSender) MonotonicCount(_ string, _ float64, _ string, _ []string) {}
func (noopSender) MonotonicCountWithFlushFirstValue(_ string, _ float64, _ string, _ []string, _ bool) {
}
func (noopSender) Counter(_ string, _ float64, _ string, _ []string)      {}
func (noopSender) Histogram(_ string, _ float64, _ string, _ []string)    {}
func (noopSender) Historate(_ string, _ float64, _ string, _ []string)    {}
func (noopSender) Distribution(_ string, _ float64, _ string, _ []string) {}
func (noopSender) ServiceCheck(_ string, _ servicecheck.ServiceCheckStatus, _ string, _ []string, _ string) {
}
func (noopSender) HistogramBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}
func (noopSender) GaugeWithTimestamp(_ string, _ float64, _ string, _ []string, _ float64) error {
	return nil
}
func (noopSender) CountWithTimestamp(_ string, _ float64, _ string, _ []string, _ float64) error {
	return nil
}
func (noopSender) Event(_ event.Event)                                                {}
func (noopSender) EventPlatformEvent(_ []byte, _ string)                              {}
func (noopSender) GetSenderStats() stats.SenderStats                                  { return stats.NewSenderStats() }
func (noopSender) DisableDefaultHostname(_ bool)                                      {}
func (noopSender) SetCheckCustomTags(_ []string)                                      {}
func (noopSender) SetCheckService(_ string)                                           {}
func (noopSender) SetNoIndex(_ bool)                                                  {}
func (noopSender) FinalizeCheckServiceTag()                                           {}
func (noopSender) OrchestratorMetadata(_ []types.ProcessMessageBody, _ string, _ int) {}
func (noopSender) OrchestratorManifest(_ []types.ProcessMessageBody, _ string)        {}

// TestAggregatingSender_SourcePopulated verifies that every metric method stamps
// the correct MetricSource on the sample delivered to the observer handle.
// This regression test guards against the earlier bug where AggregatingSender
// constructed inline MetricSamples without a Source, making source-based
// filtering in systemFilteredHandle impossible.
func TestAggregatingSender_SourcePopulated(t *testing.T) {
	// "cpu" maps to MetricSourceCPU via CheckNameToMetricSource.
	id := checkid.ID("cpu")
	handle := &capturingHandle{}

	// Use a long flush interval so the backend sender is never called during the test.
	s := NewAggregatingSender(id, noopSender{}, handle, time.Hour)
	defer s.Stop()

	calls := []struct {
		name string
		call func()
	}{
		{"Gauge", func() { s.Gauge("m", 1, "", nil) }},
		{"GaugeNoIndex", func() { s.GaugeNoIndex("m", 1, "", nil) }},
		{"Rate", func() { s.Rate("m", 1, "", nil) }},
		{"Count", func() { s.Count("m", 1, "", nil) }},
		{"MonotonicCount", func() { s.MonotonicCount("m", 1, "", nil) }},
		{"MonotonicCountWithFlushFirstValue", func() { s.MonotonicCountWithFlushFirstValue("m", 1, "", nil, false) }},
		{"Histogram", func() { s.Histogram("m", 1, "", nil) }},
		{"Historate", func() { s.Historate("m", 1, "", nil) }},
		{"Distribution", func() { s.Distribution("m", 1, "", nil) }},
		{"GaugeWithTimestamp", func() { s.GaugeWithTimestamp("m", 1, "", nil, 0) }}, //nolint:errcheck
		{"CountWithTimestamp", func() { s.CountWithTimestamp("m", 1, "", nil, 0) }}, //nolint:errcheck
		{"HistogramBucket", func() { s.HistogramBucket("m", 1, 0, 1, false, "", nil, false) }},
	}

	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			before := len(handle.samples)
			c.call()
			if len(handle.samples) == before {
				t.Fatalf("%s: expected observer handle to receive a sample", c.name)
			}
			sample := handle.samples[len(handle.samples)-1]
			sp, ok := sample.(interface{ GetSource() metrics.MetricSource })
			if !ok {
				t.Fatalf("%s: sample does not implement sourceProvider (type %T)", c.name, sample)
			}
			if got := sp.GetSource(); got != metrics.MetricSourceCPU {
				t.Errorf("%s: got Source=%v, want MetricSourceCPU (%v)", c.name, got, metrics.MetricSourceCPU)
			}
		})
	}
}

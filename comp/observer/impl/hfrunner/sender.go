// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hfrunner provides a high-frequency system check runner for the observer.
// It runs system checks at 1-second intervals and routes their output directly
// into the observer pipeline, bypassing the normal aggregator/forwarder chain.
// Metrics collected here are never forwarded to Datadog intake.
package hfrunner

import (
	"sort"
	"strings"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	aggsender "github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
)

// observerSenderManager implements sender.SenderManager.
// All senders it produces route metrics to the observer handle.
type observerSenderManager struct {
	handle observerdef.Handle
}

func newObserverSenderManager(handle observerdef.Handle) *observerSenderManager {
	return &observerSenderManager{handle: handle}
}

func (m *observerSenderManager) GetSender(id checkid.ID) (aggsender.Sender, error) {
	return &observerSender{handle: m.handle, prev: make(map[string]prevSample)}, nil
}

func (m *observerSenderManager) SetSender(s aggsender.Sender, id checkid.ID) error {
	return nil
}

func (m *observerSenderManager) DestroySender(id checkid.ID) {}

func (m *observerSenderManager) GetDefaultSender() (aggsender.Sender, error) {
	return &observerSender{handle: m.handle, prev: make(map[string]prevSample)}, nil
}

// prevSample stores the previous value and timestamp for a metric series,
// used to compute deltas for Rate and MonotonicCount metrics.
type prevSample struct {
	value float64
	ts    int64
}

// observerSender implements sender.Sender. Metric calls route to the observer
// handle as MetricView observations. Everything else (events, service checks,
// orchestrator metadata) is silently dropped — the observer doesn't use them.
//
// Rate and MonotonicCount metrics receive cumulative counter values from checks.
// The sender computes deltas so the observer sees per-interval changes, matching
// what the normal aggregator pipeline produces.
type observerSender struct {
	handle observerdef.Handle
	prev   map[string]prevSample // delta tracking for Rate/MonotonicCount
}

// inlineMetric is a lightweight MetricView that holds a single sample.
// It is stack-allocated and passed synchronously to ObserveMetric, which
// copies the data before returning, so no heap escape is required.
type inlineMetric struct {
	name      string
	value     float64
	tags      []string
	timestamp int64
}

func (m *inlineMetric) GetName() string         { return m.name }
func (m *inlineMetric) GetValue() float64       { return m.value }
func (m *inlineMetric) GetRawTags() []string    { return m.tags }
func (m *inlineMetric) GetTimestampUnix() int64 { return m.timestamp }
func (m *inlineMetric) GetSampleRate() float64  { return 1.0 }

// metricKey builds a map key from metric name + sorted tags for delta tracking.
func metricKey(name string, tags []string) string {
	if len(tags) == 0 {
		return name
	}
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	return name + "|" + strings.Join(sorted, ",")
}

// observeDelta computes the delta from the previous sample for Rate and
// MonotonicCount metrics. Returns false on the first sample (no previous
// value to diff against) so the caller can skip emitting.
func (s *observerSender) observeDelta(name string, value float64, tags []string, isRate bool) {
	key := metricKey(name, tags)
	now := time.Now().Unix()

	prev, hasPrev := s.prev[key]
	s.prev[key] = prevSample{value: value, ts: now}

	if !hasPrev {
		return // first sample — no delta to emit
	}

	delta := value - prev.value
	if delta < 0 {
		// Counter wrapped or reset — emit the raw value as the delta.
		delta = value
	}

	if isRate {
		elapsed := float64(now - prev.ts)
		if elapsed > 0 {
			delta /= elapsed
		}
	}

	s.observe(name, delta, tags)
}

func (s *observerSender) observe(name string, value float64, tags []string) {
	m := &inlineMetric{
		name:      name,
		value:     value,
		tags:      tags,
		timestamp: time.Now().Unix(),
	}
	s.handle.ObserveMetric(m)
}

func (s *observerSender) Commit() {}

func (s *observerSender) Gauge(metric string, value float64, _ string, tags []string) {
	s.observe(metric, value, tags)
}

func (s *observerSender) GaugeNoIndex(metric string, value float64, _ string, tags []string) {
	s.observe(metric, value, tags)
}

func (s *observerSender) Rate(metric string, value float64, _ string, tags []string) {
	s.observeDelta(metric, value, tags, true) // cumulative → per-second rate
}

func (s *observerSender) Count(metric string, value float64, _ string, tags []string) {
	s.observe(metric, value, tags) // already a per-interval delta
}

func (s *observerSender) MonotonicCount(metric string, value float64, _ string, tags []string) {
	s.observeDelta(metric, value, tags, false) // cumulative → delta
}

func (s *observerSender) MonotonicCountWithFlushFirstValue(metric string, value float64, _ string, tags []string, _ bool) {
	s.observeDelta(metric, value, tags, false)
}

func (s *observerSender) Counter(metric string, value float64, _ string, tags []string) {
	s.observe(metric, value, tags)
}

func (s *observerSender) Histogram(metric string, value float64, _ string, tags []string) {
	s.observe(metric, value, tags)
}

func (s *observerSender) Historate(metric string, value float64, _ string, tags []string) {
	s.observe(metric, value, tags)
}

func (s *observerSender) Distribution(metric string, value float64, _ string, tags []string) {
	s.observe(metric, value, tags)
}

func (s *observerSender) GaugeWithTimestamp(metric string, value float64, _ string, tags []string, timestamp float64) error {
	m := &inlineMetric{
		name:      metric,
		value:     value,
		tags:      tags,
		timestamp: int64(timestamp),
	}
	s.handle.ObserveMetric(m)
	return nil
}

func (s *observerSender) CountWithTimestamp(metric string, value float64, _ string, tags []string, timestamp float64) error {
	m := &inlineMetric{
		name:      metric,
		value:     value,
		tags:      tags,
		timestamp: int64(timestamp),
	}
	s.handle.ObserveMetric(m)
	return nil
}

// The following methods are no-ops: the observer does not process events,
// service checks, histogram buckets, or orchestrator payloads.

func (s *observerSender) ServiceCheck(_ string, _ servicecheck.ServiceCheckStatus, _ string, _ []string, _ string) {
}

func (s *observerSender) HistogramBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}

func (s *observerSender) Event(_ event.Event) {}

func (s *observerSender) EventPlatformEvent(_ []byte, _ string) {}

func (s *observerSender) GetSenderStats() stats.SenderStats {
	return stats.NewSenderStats()
}

func (s *observerSender) DisableDefaultHostname(_ bool) {}
func (s *observerSender) SetCheckCustomTags(_ []string) {}
func (s *observerSender) SetCheckService(_ string)      {}
func (s *observerSender) SetNoIndex(_ bool)             {}
func (s *observerSender) FinalizeCheckServiceTag()      {}

func (s *observerSender) OrchestratorMetadata(_ []types.ProcessMessageBody, _ string, _ int) {}
func (s *observerSender) OrchestratorManifest(_ []types.ProcessMessageBody, _ string)        {}

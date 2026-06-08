// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"errors"
	"sync"

	lookbackdef "github.com/DataDog/datadog-agent/comp/lookback/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
)

// shadowSender implements sender.Sender.  Metric samples are forwarded to a
// lookbackdef.Component; everything else (service checks, events, orchestrator
// data) is silently dropped.  HistogramBucket / OpenmetricsBucket are no-ops.
type shadowSender struct {
	baseSender
	sink lookbackdef.Component
}

func newShadowSender(id checkid.ID, defaultHostname string, sink lookbackdef.Component) *shadowSender {
	return &shadowSender{
		baseSender: baseSender{
			id:               id,
			defaultHostname:  defaultHostname,
			metricStats:      stats.NewSenderStats(),
			priormetricStats: stats.NewSenderStats(),
		},
		sink: sink,
	}
}

func (s *shadowSender) sendMetricSample(metric string, value float64, hostname string, tags []string, mType metrics.MetricType, flushFirstValue bool, noIndex bool, timestamp float64) {
	sample := s.buildMetricSample(metric, value, hostname, tags, mType, flushFirstValue, noIndex, timestamp)
	s.sink.RecordSample(s.id, sample.Name, sample.Value, sample.Tags, sample.Host, sample.Timestamp, sample.Mtype)

	s.statsLock.Lock()
	s.metricStats.MetricSamples++
	s.statsLock.Unlock()
}

// Commit is a no-op: shadow samples are written immediately on each call, with no
// aggregation flush needed.
func (s *shadowSender) Commit() {
	s.cyclemetricStats()
}

func (s *shadowSender) Gauge(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.GaugeType, false, false, 0)
}

func (s *shadowSender) GaugeNoIndex(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.GaugeType, false, true, 0)
}

func (s *shadowSender) Rate(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.RateType, false, false, 0)
}

func (s *shadowSender) Count(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.CountType, false, false, 0)
}

func (s *shadowSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.MonotonicCountType, false, false, 0)
}

func (s *shadowSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.MonotonicCountType, flushFirstValue, false, 0)
}

func (s *shadowSender) Counter(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.CounterType, false, false, 0)
}

func (s *shadowSender) Histogram(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.HistogramType, false, false, 0)
}

func (s *shadowSender) Historate(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.HistorateType, false, false, 0)
}

func (s *shadowSender) Distribution(metric string, value float64, hostname string, tags []string) {
	s.sendMetricSample(metric, value, hostname, tags, metrics.DistributionType, false, false, 0)
}

func (s *shadowSender) GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	if timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	s.sendMetricSample(metric, value, hostname, tags, metrics.GaugeWithTimestampType, false, false, timestamp)
	return nil
}

func (s *shadowSender) CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	if timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	s.sendMetricSample(metric, value, hostname, tags, metrics.CountWithTimestampType, false, false, timestamp)
	return nil
}

// HistogramBucket is a no-op in this iteration.
func (s *shadowSender) HistogramBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}

// OpenmetricsBucket is a no-op in this iteration.
func (s *shadowSender) OpenmetricsBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}

// ServiceCheck is a no-op: shadow pipeline captures metric samples only.
func (s *shadowSender) ServiceCheck(_ string, _ servicecheck.ServiceCheckStatus, _ string, _ []string, _ string) {
}

// SendRawServiceCheck is a no-op.
func (s *shadowSender) SendRawServiceCheck(_ *servicecheck.ServiceCheck) {}

// Event is a no-op: shadow pipeline captures metric samples only.
func (s *shadowSender) Event(_ event.Event) {}

// EventPlatformEvent is a no-op.
func (s *shadowSender) EventPlatformEvent(_ []byte, _ string) {}

// SendRawMetricSample is a no-op for shadow: callers should use the typed methods.
func (s *shadowSender) SendRawMetricSample(_ *metrics.MetricSample) {}

// OrchestratorMetadata is a no-op.
func (s *shadowSender) OrchestratorMetadata(_ []types.ProcessMessageBody, _ string, _ int) {}

// OrchestratorManifest is a no-op.
func (s *shadowSender) OrchestratorManifest(_ []types.ProcessMessageBody, _ string) {}

// shadowSenderManager implements sender.SenderManager.  It creates shadowSenders
// backed by a lookbackdef.Component and has no interaction with BufferedAggregator.
type shadowSenderManager struct {
	sink     lookbackdef.Component
	hostname string
	senders  map[checkid.ID]sender.Sender
	mu       sync.Mutex
}

// NewShadowSenderManager returns a SenderManager whose senders write to sink.
func NewShadowSenderManager(sink lookbackdef.Component, hostname string) sender.SenderManager {
	return &shadowSenderManager{
		sink:     sink,
		hostname: hostname,
		senders:  make(map[checkid.ID]sender.Sender),
	}
}

func (m *shadowSenderManager) GetSender(id checkid.ID) (sender.Sender, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.senders[id]; ok {
		return s, nil
	}
	s := newShadowSender(id, m.hostname, m.sink)
	m.senders[id] = s
	return s, nil
}

func (m *shadowSenderManager) SetSender(s sender.Sender, id checkid.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.senders[id] = s
	return nil
}

func (m *shadowSenderManager) DestroySender(id checkid.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.senders, id)
}

func (m *shadowSenderManager) GetDefaultSender() (sender.Sender, error) {
	return m.GetSender(checkid.ID("shadow-default"))
}

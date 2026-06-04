// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lookbacksender contains the shadow sender used by 1Hz check lookback.
package lookbacksender

import (
	"context"
	"errors"
	"sync"
	"time"

	aggregatorsender "github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	log "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Writer stores scalar metric samples emitted by shadow checks.
type Writer interface {
	Append(ctx context.Context, checkID checkid.ID, samples []metrics.MetricSample) error
}

// SenderManager provides lookback senders for shadow check IDs.
type SenderManager struct {
	mu              sync.Mutex
	ctx             context.Context
	defaultHostname string
	writer          Writer
	now             func() float64
	senders         map[checkid.ID]*Sender
	defaultSender   *noopSender
}

// NewSenderManager creates a sender manager for lookback shadow checks.
func NewSenderManager(ctx context.Context, defaultHostname string, writer Writer, now func() float64) *SenderManager {
	if ctx == nil {
		ctx = context.Background()
	}
	if now == nil {
		now = func() float64 {
			return float64(time.Now().UnixNano()) / 1e9
		}
	}

	return &SenderManager{
		ctx:             ctx,
		defaultHostname: defaultHostname,
		writer:          writer,
		now:             now,
		senders:         make(map[checkid.ID]*Sender),
		defaultSender:   &noopSender{},
	}
}

// GetSender returns one lookback sender per shadow check ID.
func (m *SenderManager) GetSender(id checkid.ID) (aggregatorsender.Sender, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, found := m.senders[id]; found {
		return s, nil
	}

	s := newSender(m.ctx, id, m.defaultHostname, m.writer, m.now)
	m.senders[id] = s
	return s, nil
}

// SetSender sets the sender for a check ID.
func (m *SenderManager) SetSender(s aggregatorsender.Sender, id checkid.ID) error {
	lookbackSender, ok := s.(*Sender)
	if !ok {
		return errors.New("sender must be a lookback sender")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.senders[id] = lookbackSender
	return nil
}

// DestroySender removes the sender for a shadow check ID.
func (m *SenderManager) DestroySender(id checkid.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.senders, id)
}

// GetDefaultSender returns a no-op sender for paths that request a default sender.
func (m *SenderManager) GetDefaultSender() (aggregatorsender.Sender, error) {
	return m.defaultSender, nil
}

// Sender implements sender.Sender for lookback shadow checks.
type Sender struct {
	ctx     context.Context
	id      checkid.ID
	writer  Writer
	factory *aggregatorsender.CheckMetricSampleFactory

	mu        sync.Mutex
	samples   []metrics.MetricSample
	stats     stats.SenderStats
	priorStat stats.SenderStats
	checkTags []string
	service   string
}

func newSender(ctx context.Context, id checkid.ID, defaultHostname string, writer Writer, now func() float64) *Sender {
	return &Sender{
		ctx:       ctx,
		id:        id,
		writer:    writer,
		factory:   aggregatorsender.NewCheckMetricSampleFactory(id, defaultHostname, now),
		stats:     stats.NewSenderStats(),
		priorStat: stats.NewSenderStats(),
	}
}

// Commit writes buffered scalar samples to the lookback writer.
func (s *Sender) Commit() {
	s.mu.Lock()
	samples := s.samples
	s.samples = nil
	s.priorStat = s.stats.Copy()
	s.stats = stats.NewSenderStats()
	s.mu.Unlock()

	if len(samples) == 0 || s.writer == nil {
		return
	}

	if err := s.writer.Append(s.ctx, s.id, samples); err != nil {
		log.Warnf("failed to append %d lookback samples for check %s: %v", len(samples), s.id, err)
	}
}

func (s *Sender) appendScalarSample(input aggregatorsender.ScalarSample) {
	sample := *s.factory.BuildMetricSample(input)
	sample.Tags = cloneTags(sample.Tags)

	s.mu.Lock()
	s.samples = append(s.samples, sample)
	s.stats.MetricSamples++
	s.mu.Unlock()
}

func cloneTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	return append([]string(nil), tags...)
}

// GetSenderStats returns sender stats from the previous committed run.
func (s *Sender) GetSenderStats() stats.SenderStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.priorStat.Copy()
}

// DisableDefaultHostname controls default hostname injection for scalar samples.
func (s *Sender) DisableDefaultHostname(disable bool) {
	s.factory.DisableDefaultHostname(disable)
}

// SetCheckCustomTags stores tags from check configuration.
func (s *Sender) SetCheckCustomTags(tags []string) {
	s.mu.Lock()
	s.checkTags = tags
	s.mu.Unlock()

	s.factory.SetCheckCustomTags(tags)
}

// SetCheckService stores the service tag to apply at finalization time.
func (s *Sender) SetCheckService(service string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.service = service
}

// SetNoIndex controls no-index behavior for scalar samples.
func (s *Sender) SetNoIndex(noIndex bool) {
	s.factory.SetNoIndex(noIndex)
}

// FinalizeCheckServiceTag applies the configured service tag to scalar samples.
func (s *Sender) FinalizeCheckServiceTag() {
	s.mu.Lock()
	if s.service == "" {
		s.mu.Unlock()
		return
	}

	s.checkTags = append(s.checkTags, "service:"+s.service)
	checkTags := s.checkTags
	s.mu.Unlock()

	s.factory.SetCheckCustomTags(checkTags)
}

func (s *Sender) Gauge(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(aggregatorsender.ScalarSample{Name: metric, Value: value, Hostname: hostname, Tags: tags, Type: metrics.GaugeType})
}

func (s *Sender) GaugeNoIndex(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(aggregatorsender.ScalarSample{Name: metric, Value: value, Hostname: hostname, Tags: tags, Type: metrics.GaugeType, NoIndex: true})
}

func (s *Sender) Rate(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(aggregatorsender.ScalarSample{Name: metric, Value: value, Hostname: hostname, Tags: tags, Type: metrics.RateType})
}

func (s *Sender) Count(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(aggregatorsender.ScalarSample{Name: metric, Value: value, Hostname: hostname, Tags: tags, Type: metrics.CountType})
}

func (s *Sender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(aggregatorsender.ScalarSample{Name: metric, Value: value, Hostname: hostname, Tags: tags, Type: metrics.MonotonicCountType})
}

func (s *Sender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	s.appendScalarSample(aggregatorsender.ScalarSample{Name: metric, Value: value, Hostname: hostname, Tags: tags, Type: metrics.MonotonicCountType, FlushFirstValue: flushFirstValue})
}

func (s *Sender) Counter(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(aggregatorsender.ScalarSample{Name: metric, Value: value, Hostname: hostname, Tags: tags, Type: metrics.CounterType})
}

func (s *Sender) Histogram(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(aggregatorsender.ScalarSample{Name: metric, Value: value, Hostname: hostname, Tags: tags, Type: metrics.HistogramType})
}

func (s *Sender) Historate(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(aggregatorsender.ScalarSample{Name: metric, Value: value, Hostname: hostname, Tags: tags, Type: metrics.HistorateType})
}

func (s *Sender) GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	if timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	s.appendScalarSample(aggregatorsender.ScalarSample{Name: metric, Value: value, Hostname: hostname, Tags: tags, Type: metrics.GaugeWithTimestampType, Timestamp: timestamp})
	return nil
}

func (s *Sender) CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	if timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	s.appendScalarSample(aggregatorsender.ScalarSample{Name: metric, Value: value, Hostname: hostname, Tags: tags, Type: metrics.CountWithTimestampType, Timestamp: timestamp})
	return nil
}

// Distribution is not captured by lookback V1.
func (s *Sender) Distribution(_ string, _ float64, _ string, _ []string) {}

// ServiceCheck is not captured by lookback V1.
func (s *Sender) ServiceCheck(_ string, _ servicecheck.ServiceCheckStatus, _ string, _ []string, _ string) {
}

// OpenmetricsBucket is not captured by lookback V1.
func (s *Sender) OpenmetricsBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}

// HistogramBucket is not captured by lookback V1.
func (s *Sender) HistogramBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}

// Event is not captured by lookback V1.
func (s *Sender) Event(_ event.Event) {}

// EventPlatformEvent is not captured by lookback V1.
func (s *Sender) EventPlatformEvent(_ []byte, _ string) {}

// OrchestratorMetadata is not captured by lookback V1.
func (s *Sender) OrchestratorMetadata(_ []types.ProcessMessageBody, _ string, _ int) {}

// OrchestratorManifest is not captured by lookback V1.
func (s *Sender) OrchestratorManifest(_ []types.ProcessMessageBody, _ string) {}

type noopSender struct{}

func (n *noopSender) Commit() {}

func (n *noopSender) Gauge(_ string, _ float64, _ string, _ []string) {}

func (n *noopSender) GaugeNoIndex(_ string, _ float64, _ string, _ []string) {}

func (n *noopSender) Rate(_ string, _ float64, _ string, _ []string) {}

func (n *noopSender) Count(_ string, _ float64, _ string, _ []string) {}

func (n *noopSender) MonotonicCount(_ string, _ float64, _ string, _ []string) {}

func (n *noopSender) MonotonicCountWithFlushFirstValue(_ string, _ float64, _ string, _ []string, _ bool) {
}

func (n *noopSender) Counter(_ string, _ float64, _ string, _ []string) {}

func (n *noopSender) Histogram(_ string, _ float64, _ string, _ []string) {}

func (n *noopSender) Historate(_ string, _ float64, _ string, _ []string) {}

func (n *noopSender) Distribution(_ string, _ float64, _ string, _ []string) {}

func (n *noopSender) ServiceCheck(_ string, _ servicecheck.ServiceCheckStatus, _ string, _ []string, _ string) {
}

func (n *noopSender) OpenmetricsBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}

func (n *noopSender) HistogramBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}

func (n *noopSender) GaugeWithTimestamp(_ string, _ float64, _ string, _ []string, timestamp float64) error {
	if timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	return nil
}

func (n *noopSender) CountWithTimestamp(_ string, _ float64, _ string, _ []string, timestamp float64) error {
	if timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	return nil
}

func (n *noopSender) Event(_ event.Event) {}

func (n *noopSender) EventPlatformEvent(_ []byte, _ string) {}

func (n *noopSender) GetSenderStats() stats.SenderStats { return stats.NewSenderStats() }

func (n *noopSender) DisableDefaultHostname(_ bool) {}

func (n *noopSender) SetCheckCustomTags(_ []string) {}

func (n *noopSender) SetCheckService(_ string) {}

func (n *noopSender) SetNoIndex(_ bool) {}

func (n *noopSender) FinalizeCheckServiceTag() {}

func (n *noopSender) OrchestratorMetadata(_ []types.ProcessMessageBody, _ string, _ int) {}

func (n *noopSender) OrchestratorManifest(_ []types.ProcessMessageBody, _ string) {}

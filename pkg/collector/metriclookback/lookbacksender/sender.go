// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lookbacksender contains the shadow sender used by metric lookback.
package lookbacksender

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	aggregatorsender "github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/infratags"
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
	senders         map[checkid.ID]*sender
	defaultSender   *noopSender
}

// NewSenderManager creates a sender manager for lookback shadow checks. It
// returns nil when no writer is configured so selected shadows cannot silently
// discard their only output.
func NewSenderManager(ctx context.Context, defaultHostname string, writer Writer, now func() float64) *SenderManager {
	if writer == nil {
		return nil
	}
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
		senders:         make(map[checkid.ID]*sender),
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
	lookbackSender, ok := s.(*sender)
	if !ok {
		return errors.New("sender must be a lookback sender")
	}
	if lookbackSender.id != id {
		return errors.New("sender ID " + string(lookbackSender.id) + " does not match check ID " + string(id))
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

type sender struct {
	ctx             context.Context
	id              checkid.ID
	checkName       string
	defaultHostname string
	writer          Writer
	now             func() float64
	metricSource    metrics.MetricSource

	mu                      sync.Mutex
	defaultHostnameDisabled bool
	checkTags               []string
	infraTagger             *infratags.Tagger
	service                 string
	noIndex                 bool
	samples                 []metrics.MetricSample
	stats                   stats.SenderStats
	priorStats              stats.SenderStats
	unsupportedDrops        map[string]int64
}

func newSender(ctx context.Context, id checkid.ID, defaultHostname string, writer Writer, now func() float64) *sender {
	checkName := checkid.IDToCheckName(id)
	return &sender{
		ctx:             ctx,
		id:              id,
		checkName:       checkName,
		defaultHostname: defaultHostname,
		writer:          writer,
		now:             now,
		metricSource:    metrics.CheckNameToMetricSource(checkName),
		stats:           stats.NewSenderStats(),
		priorStats:      stats.NewSenderStats(),
	}
}

// Commit writes buffered scalar samples to the lookback writer.
func (s *sender) Commit() {
	s.mu.Lock()
	samples := slices.Clone(s.samples)
	s.samples = nil
	unsupportedDrops := s.unsupportedDrops
	s.unsupportedDrops = nil
	s.priorStats = s.stats.Copy()
	s.stats = stats.NewSenderStats()
	s.mu.Unlock()

	for method, count := range unsupportedDrops {
		tlmUnsupportedDrops.Add(float64(count), method)
	}

	if len(samples) == 0 {
		return
	}

	start := time.Now()
	state := "ok"
	if err := s.writer.Append(s.ctx, s.id, samples); err != nil {
		state = "error"
		log.Warnf("failed to append %d lookback samples for check %s: %v", len(samples), s.id, err)
	}
	tlmWriterAppendSamples.Add(float64(len(samples)), s.checkName, state)
	tlmWriterAppendDuration.Set(time.Since(start).Seconds(), s.checkName, state)
}

// GetSenderStats returns sender stats from the previous committed run.
func (s *sender) GetSenderStats() stats.SenderStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.priorStats.Copy()
}

func (s *sender) appendScalarSample(metric string, value float64, hostname string, tags []string, mType metrics.MetricType, flushFirstValue bool, noIndex bool, timestamp float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if timestamp == 0 {
		timestamp = s.now()
	}
	sampleTags := slices.Concat(tags, s.checkTags)
	sampleTags = s.infraTagger.AppendTags(sampleTags)
	sample := metrics.MetricSample{
		Name:            metric,
		Value:           value,
		Mtype:           mType,
		Tags:            sampleTags,
		Host:            hostname,
		SampleRate:      1,
		Timestamp:       timestamp,
		FlushFirstValue: flushFirstValue,
		NoIndex:         s.noIndex || noIndex,
		Source:          s.metricSource,
	}
	if hostname == "" && !s.defaultHostnameDisabled {
		sample.Host = s.defaultHostname
	}

	s.samples = append(s.samples, sample)
	s.stats.MetricSamples++
}

func (s *sender) recordUnsupportedDrop(method string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unsupportedDrops == nil {
		s.unsupportedDrops = make(map[string]int64)
	}
	s.unsupportedDrops[method]++
}

// DisableDefaultHostname allows checks to opt out of the configured default hostname.
func (s *sender) DisableDefaultHostname(disable bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultHostnameDisabled = disable
}

// SetCheckCustomTags stores custom check tags appended to each scalar metric sample.
func (s *sender) SetCheckCustomTags(tags []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkTags = slices.Clone(tags)
}

// SetInfraTagger stores the tagger that appends infra mode tags to scalar metric samples.
func (s *sender) SetInfraTagger(tagger *infratags.Tagger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.infraTagger = tagger
}

// SetCheckService stores the check service to add as a tag when finalized.
func (s *sender) SetCheckService(service string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.service = service
}

// FinalizeCheckServiceTag appends the latest service tag to future scalar metric samples.
func (s *sender) FinalizeCheckServiceTag() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.service != "" {
		s.checkTags = append(s.checkTags, "service:"+s.service)
	}
}

// SetNoIndex marks future scalar metric samples as no-index.
func (s *sender) SetNoIndex(noIndex bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noIndex = noIndex
}

func (s *sender) Gauge(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(metric, value, hostname, tags, metrics.GaugeType, false, false, 0)
}

func (s *sender) GaugeNoIndex(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(metric, value, hostname, tags, metrics.GaugeType, false, true, 0)
}

func (s *sender) Rate(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(metric, value, hostname, tags, metrics.RateType, false, false, 0)
}

func (s *sender) Count(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(metric, value, hostname, tags, metrics.CountType, false, false, 0)
}

func (s *sender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(metric, value, hostname, tags, metrics.MonotonicCountType, false, false, 0)
}

func (s *sender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	s.appendScalarSample(metric, value, hostname, tags, metrics.MonotonicCountType, flushFirstValue, false, 0)
}

func (s *sender) Counter(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(metric, value, hostname, tags, metrics.CounterType, false, false, 0)
}

func (s *sender) Histogram(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(metric, value, hostname, tags, metrics.HistogramType, false, false, 0)
}

func (s *sender) Historate(metric string, value float64, hostname string, tags []string) {
	s.appendScalarSample(metric, value, hostname, tags, metrics.HistorateType, false, false, 0)
}

func (s *sender) GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	if timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	s.appendScalarSample(metric, value, hostname, tags, metrics.GaugeWithTimestampType, false, false, timestamp)
	return nil
}

func (s *sender) CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	if timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	s.appendScalarSample(metric, value, hostname, tags, metrics.CountWithTimestampType, false, false, timestamp)
	return nil
}

// Distribution is not captured by metric lookback.
func (s *sender) Distribution(_ string, _ float64, _ string, _ []string) {
	s.recordUnsupportedDrop("Distribution")
}

// ServiceCheck is not captured by metric lookback.
func (s *sender) ServiceCheck(_ string, _ servicecheck.ServiceCheckStatus, _ string, _ []string, _ string) {
	s.recordUnsupportedDrop("ServiceCheck")
}

// OpenmetricsBucket is not captured by metric lookback.
func (s *sender) OpenmetricsBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
	s.recordUnsupportedDrop("OpenmetricsBucket")
}

// HistogramBucket is not captured by metric lookback.
func (s *sender) HistogramBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
	s.recordUnsupportedDrop("HistogramBucket")
}

// Event is not captured by metric lookback.
func (s *sender) Event(_ event.Event) {
	s.recordUnsupportedDrop("Event")
}

// EventPlatformEvent is not captured by metric lookback.
func (s *sender) EventPlatformEvent(_ []byte, _ string) {
	s.recordUnsupportedDrop("EventPlatformEvent")
}

// OrchestratorMetadata is not captured by metric lookback.
func (s *sender) OrchestratorMetadata(_ []types.ProcessMessageBody, _ string, _ int) {
	s.recordUnsupportedDrop("OrchestratorMetadata")
}

// OrchestratorManifest is not captured by metric lookback.
func (s *sender) OrchestratorManifest(_ []types.ProcessMessageBody, _ string) {
	s.recordUnsupportedDrop("OrchestratorManifest")
}

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

func (n *noopSender) SetInfraTagger(_ *infratags.Tagger) {}

func (n *noopSender) SetCheckService(_ string) {}

func (n *noopSender) SetNoIndex(_ bool) {}

func (n *noopSender) FinalizeCheckServiceTag() {}

func (n *noopSender) OrchestratorMetadata(_ []types.ProcessMessageBody, _ string, _ int) {}

func (n *noopSender) OrchestratorManifest(_ []types.ProcessMessageBody, _ string) {}

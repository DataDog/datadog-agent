// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
)

// walSample is a buffered metric sample waiting to be committed to the backend.
type walSample struct {
	name  string
	value float64
	tags  []string
	tsUs  int64 // Unix microseconds
}

// walSender implements sender.Sender and routes all metric samples to the
// lookback backend instead of the agent's aggregator pipeline. It buffers
// samples until Commit() is called, matching the normal check sender contract.
type walSender struct {
	mu      sync.Mutex
	samples []walSample
	backend timeSeriesBackend
	log     log.Component
}

func newWALSender(backend timeSeriesBackend, l log.Component) *walSender {
	return &walSender{backend: backend, log: l}
}

func (s *walSender) buffer(name string, value float64, tags []string, tsUs int64) {
	s.mu.Lock()
	s.samples = append(s.samples, walSample{name: name, value: value, tags: sortedTagsCopy(tags), tsUs: tsUs})
	s.mu.Unlock()
}

// Commit flushes all buffered samples to the backend.
func (s *walSender) Commit() {
	s.mu.Lock()
	samples := s.samples
	s.samples = nil
	s.mu.Unlock()

	now := time.Now().UnixMicro()
	for _, m := range samples {
		tsUs := m.tsUs
		if tsUs == 0 {
			tsUs = now
		}
		s.backend.writeSample(m.name, m.tags, tsUs, m.value)
	}
}

func (s *walSender) Gauge(metric string, value float64, _ string, tags []string) {
	s.buffer(metric, value, tags, 0)
}
func (s *walSender) GaugeNoIndex(metric string, value float64, _ string, tags []string) {
	s.buffer(metric, value, tags, 0)
}
func (s *walSender) Rate(metric string, value float64, _ string, tags []string) {
	s.buffer(metric, value, tags, 0)
}
func (s *walSender) Count(metric string, value float64, _ string, tags []string) {
	s.buffer(metric, value, tags, 0)
}
func (s *walSender) MonotonicCount(metric string, value float64, _ string, tags []string) {
	s.buffer(metric, value, tags, 0)
}
func (s *walSender) MonotonicCountWithFlushFirstValue(metric string, value float64, _ string, tags []string, _ bool) {
	s.buffer(metric, value, tags, 0)
}
func (s *walSender) Counter(metric string, value float64, _ string, tags []string) {
	s.buffer(metric, value, tags, 0)
}
func (s *walSender) Histogram(metric string, value float64, _ string, tags []string) {
	s.buffer(metric, value, tags, 0)
}
func (s *walSender) Historate(metric string, value float64, _ string, tags []string) {
	s.buffer(metric, value, tags, 0)
}
func (s *walSender) Distribution(metric string, value float64, _ string, tags []string) {
	s.buffer(metric, value, tags, 0)
}
func (s *walSender) GaugeWithTimestamp(metric string, value float64, _ string, tags []string, timestamp float64) error {
	s.buffer(metric, value, tags, int64(timestamp*1e6))
	return nil
}
func (s *walSender) CountWithTimestamp(metric string, value float64, _ string, tags []string, timestamp float64) error {
	s.buffer(metric, value, tags, int64(timestamp*1e6))
	return nil
}

// --- no-op methods (events, service checks, orchestrator) ---

func (s *walSender) ServiceCheck(_ string, _ servicecheck.ServiceCheckStatus, _ string, _ []string, _ string) {
}
func (s *walSender) Event(_ event.Event)                                    {}
func (s *walSender) EventPlatformEvent(_ []byte, _ string)                  {}
func (s *walSender) OpenmetricsBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}
func (s *walSender) HistogramBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}
func (s *walSender) OrchestratorMetadata(_ []types.ProcessMessageBody, _ string, _ int) {}
func (s *walSender) OrchestratorManifest(_ []types.ProcessMessageBody, _ string) {}
func (s *walSender) GetSenderStats() stats.SenderStats                                     { return stats.SenderStats{} }
func (s *walSender) DisableDefaultHostname(_ bool)                                          {}
func (s *walSender) SetCheckCustomTags(_ []string)                                          {}
func (s *walSender) SetCheckService(_ string)                                               {}
func (s *walSender) SetNoIndex(_ bool)                                                      {}
func (s *walSender) FinalizeCheckServiceTag()                                               {}

// walSenderManager implements sender.SenderManager and routes all check
// metric output to walSender instances. It never touches the agent aggregator.
type walSenderManager struct {
	mu      sync.Mutex
	senders map[checkid.ID]*walSender
	backend timeSeriesBackend
	log     log.Component
}

func newWALSenderManager(backend timeSeriesBackend, l log.Component) *walSenderManager {
	return &walSenderManager{
		senders: make(map[checkid.ID]*walSender),
		backend: backend,
		log:     l,
	}
}

func (m *walSenderManager) GetSender(id checkid.ID) (sender.Sender, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.senders[id]; ok {
		return s, nil
	}
	s := newWALSender(m.backend, m.log)
	m.senders[id] = s
	return s, nil
}

func (m *walSenderManager) SetSender(s sender.Sender, id checkid.ID) error {
	if ws, ok := s.(*walSender); ok {
		m.mu.Lock()
		m.senders[id] = ws
		m.mu.Unlock()
	}
	return nil
}

func (m *walSenderManager) DestroySender(id checkid.ID) {
	m.mu.Lock()
	delete(m.senders, id)
	m.mu.Unlock()
}

// GetDefaultSender returns a no-op sender used by the worker for the post-run
// ServiceCheck/Commit call; the check's own walSender already committed.
func (m *walSenderManager) GetDefaultSender() (sender.Sender, error) {
	return &noopSender{}, nil
}

// noopSender implements sender.Sender with no-ops for all methods.
// Used as the "default" sender so worker's post-run Commit() is harmless.
type noopSender struct{}

func (n *noopSender) Commit()                                                                    {}
func (n *noopSender) Gauge(_ string, _ float64, _ string, _ []string)                           {}
func (n *noopSender) GaugeNoIndex(_ string, _ float64, _ string, _ []string)                    {}
func (n *noopSender) Rate(_ string, _ float64, _ string, _ []string)                            {}
func (n *noopSender) Count(_ string, _ float64, _ string, _ []string)                           {}
func (n *noopSender) MonotonicCount(_ string, _ float64, _ string, _ []string)                  {}
func (n *noopSender) MonotonicCountWithFlushFirstValue(_ string, _ float64, _ string, _ []string, _ bool) {
}
func (n *noopSender) Counter(_ string, _ float64, _ string, _ []string)                         {}
func (n *noopSender) Histogram(_ string, _ float64, _ string, _ []string)                       {}
func (n *noopSender) Historate(_ string, _ float64, _ string, _ []string)                       {}
func (n *noopSender) Distribution(_ string, _ float64, _ string, _ []string)                    {}
func (n *noopSender) ServiceCheck(_ string, _ servicecheck.ServiceCheckStatus, _ string, _ []string, _ string) {
}
func (n *noopSender) OpenmetricsBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}
func (n *noopSender) HistogramBucket(_ string, _ int64, _, _ float64, _ bool, _ string, _ []string, _ bool) {
}
func (n *noopSender) GaugeWithTimestamp(_ string, _ float64, _ string, _ []string, _ float64) error {
	return nil
}
func (n *noopSender) CountWithTimestamp(_ string, _ float64, _ string, _ []string, _ float64) error {
	return nil
}
func (n *noopSender) Event(_ event.Event)                                                       {}
func (n *noopSender) EventPlatformEvent(_ []byte, _ string)                                     {}
func (n *noopSender) GetSenderStats() stats.SenderStats                                         { return stats.SenderStats{} }
func (n *noopSender) DisableDefaultHostname(_ bool)                                             {}
func (n *noopSender) SetCheckCustomTags(_ []string)                                             {}
func (n *noopSender) SetCheckService(_ string)                                                  {}
func (n *noopSender) SetNoIndex(_ bool)                                                         {}
func (n *noopSender) FinalizeCheckServiceTag()                                                  {}
func (n *noopSender) OrchestratorMetadata(_ []types.ProcessMessageBody, _ string, _ int)      {}
func (n *noopSender) OrchestratorManifest(_ []types.ProcessMessageBody, _ string)              {}

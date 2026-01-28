// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"sync"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
)

// AggregatingSender wraps any sender to provide high-frequency observer
// recording with local aggregation before backend submission.
// This is completely transparent to checks - they just use the sender interface.
type AggregatingSender struct {
	backendSender    Sender
	observerHandle   observer.Handle
	aggregator       *localAggregator
	originalInterval time.Duration // Original check interval for backend flush
	stopCh           chan struct{}
	mu               sync.Mutex
}

// NewAggregatingSender wraps a backend sender for high-frequency collection.
// - backendSender: the real sender that submits to backend
// - observerHandle: observer handle for immediate high-freq observations
// - originalInterval: original check interval (for backend flush timing)
func NewAggregatingSender(
	backendSender Sender,
	observerHandle observer.Handle,
	originalInterval time.Duration,
) *AggregatingSender {
	s := &AggregatingSender{
		backendSender:    backendSender,
		observerHandle:   observerHandle,
		aggregator:       newLocalAggregator(),
		originalInterval: originalInterval,
		stopCh:           make(chan struct{}),
	}

	// Start flush loop at original interval
	go s.flushLoop()

	return s
}

// Gauge sends to observer immediately and accumulates for backend
func (s *AggregatingSender) Gauge(metric string, value float64, hostname string, tags []string) {
	// Observer: send immediately (high-frequency)
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      value,
			Mtype:      metrics.GaugeType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  float64(time.Now().Unix()),
		}
		s.observerHandle.ObserveMetric(sample)
	}

	// Aggregator: accumulate for backend
	s.mu.Lock()
	s.aggregator.addGauge(metric, value, hostname, tags)
	s.mu.Unlock()
}

// GaugeNoIndex sends to observer immediately and accumulates for backend
func (s *AggregatingSender) GaugeNoIndex(metric string, value float64, hostname string, tags []string) {
	// Observer: send immediately (high-frequency)
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      value,
			Mtype:      metrics.GaugeType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  float64(time.Now().Unix()),
		}
		s.observerHandle.ObserveMetric(sample)
	}

	// Aggregator: accumulate for backend (with noIndex flag)
	s.mu.Lock()
	s.aggregator.addGaugeNoIndex(metric, value, hostname, tags)
	s.mu.Unlock()
}

// Rate sends to observer immediately and accumulates for backend
func (s *AggregatingSender) Rate(metric string, value float64, hostname string, tags []string) {
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      value,
			Mtype:      metrics.RateType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  float64(time.Now().Unix()),
		}
		s.observerHandle.ObserveMetric(sample)
	}

	s.mu.Lock()
	s.aggregator.addRate(metric, value, hostname, tags)
	s.mu.Unlock()
}

// Count sends to observer immediately and accumulates for backend (sums)
func (s *AggregatingSender) Count(metric string, value float64, hostname string, tags []string) {
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      value,
			Mtype:      metrics.CountType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  float64(time.Now().Unix()),
		}
		s.observerHandle.ObserveMetric(sample)
	}

	s.mu.Lock()
	s.aggregator.addCount(metric, value, hostname, tags)
	s.mu.Unlock()
}

// MonotonicCount sends to observer immediately and accumulates for backend (sums)
func (s *AggregatingSender) MonotonicCount(metric string, value float64, hostname string, tags []string) {
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      value,
			Mtype:      metrics.MonotonicCountType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  float64(time.Now().Unix()),
		}
		s.observerHandle.ObserveMetric(sample)
	}

	s.mu.Lock()
	s.aggregator.addMonotonicCount(metric, value, hostname, tags)
	s.mu.Unlock()
}

// MonotonicCountWithFlushFirstValue sends to observer immediately and accumulates for backend
func (s *AggregatingSender) MonotonicCountWithFlushFirstValue(metric string, value float64, hostname string, tags []string, flushFirstValue bool) {
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      value,
			Mtype:      metrics.MonotonicCountType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  float64(time.Now().Unix()),
		}
		s.observerHandle.ObserveMetric(sample)
	}

	s.mu.Lock()
	s.aggregator.addMonotonicCountWithFlushFirstValue(metric, value, hostname, tags, flushFirstValue)
	s.mu.Unlock()
}

// Counter is deprecated but we still forward it
func (s *AggregatingSender) Counter(metric string, value float64, hostname string, tags []string) {
	s.Count(metric, value, hostname, tags)
}

// Histogram sends to observer immediately and accumulates for backend
func (s *AggregatingSender) Histogram(metric string, value float64, hostname string, tags []string) {
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      value,
			Mtype:      metrics.HistogramType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  float64(time.Now().Unix()),
		}
		s.observerHandle.ObserveMetric(sample)
	}

	s.mu.Lock()
	s.aggregator.addHistogram(metric, value, hostname, tags)
	s.mu.Unlock()
}

// Historate sends to observer immediately and accumulates for backend
func (s *AggregatingSender) Historate(metric string, value float64, hostname string, tags []string) {
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      value,
			Mtype:      metrics.HistorateType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  float64(time.Now().Unix()),
		}
		s.observerHandle.ObserveMetric(sample)
	}

	s.mu.Lock()
	s.aggregator.addHistorate(metric, value, hostname, tags)
	s.mu.Unlock()
}

// Distribution sends to observer immediately and forwards to backend
func (s *AggregatingSender) Distribution(metric string, value float64, hostname string, tags []string) {
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      value,
			Mtype:      metrics.DistributionType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  float64(time.Now().Unix()),
		}
		s.observerHandle.ObserveMetric(sample)
	}

	// Distributions are forwarded directly (no local aggregation)
	s.backendSender.Distribution(metric, value, hostname, tags)
}

// GaugeWithTimestamp sends to observer and forwards to backend
func (s *AggregatingSender) GaugeWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      value,
			Mtype:      metrics.GaugeType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  timestamp,
		}
		s.observerHandle.ObserveMetric(sample)
	}

	// Timestamped metrics are forwarded directly (no aggregation)
	return s.backendSender.GaugeWithTimestamp(metric, value, hostname, tags, timestamp)
}

// CountWithTimestamp sends to observer and forwards to backend
func (s *AggregatingSender) CountWithTimestamp(metric string, value float64, hostname string, tags []string, timestamp float64) error {
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      value,
			Mtype:      metrics.CountType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  timestamp,
		}
		s.observerHandle.ObserveMetric(sample)
	}

	// Timestamped metrics are forwarded directly (no aggregation)
	return s.backendSender.CountWithTimestamp(metric, value, hostname, tags, timestamp)
}

// ServiceCheck forwards directly to backend
func (s *AggregatingSender) ServiceCheck(checkName string, status servicecheck.ServiceCheckStatus, hostname string, tags []string, message string) {
	s.backendSender.ServiceCheck(checkName, status, hostname, tags, message)
}

// HistogramBucket sends to observer and accumulates for backend
func (s *AggregatingSender) HistogramBucket(metric string, value int64, lowerBound, upperBound float64, monotonic bool, hostname string, tags []string, flushFirstValue bool) {
	if s.observerHandle != nil {
		sample := &metrics.MetricSample{
			Name:       metric,
			Value:      float64(value),
			Mtype:      metrics.HistogramType,
			Tags:       tags,
			Host:       hostname,
			SampleRate: 1.0,
			Timestamp:  float64(time.Now().Unix()),
		}
		s.observerHandle.ObserveMetric(sample)
	}

	s.mu.Lock()
	s.aggregator.addHistogramBucket(metric, value, lowerBound, upperBound, monotonic, hostname, tags, flushFirstValue)
	s.mu.Unlock()
}

// Event forwards directly to backend
func (s *AggregatingSender) Event(e event.Event) {
	s.backendSender.Event(e)
}

// EventPlatformEvent forwards directly to backend
func (s *AggregatingSender) EventPlatformEvent(rawEvent []byte, eventType string) {
	s.backendSender.EventPlatformEvent(rawEvent, eventType)
}

// GetSenderStats forwards to backend sender
func (s *AggregatingSender) GetSenderStats() stats.SenderStats {
	return s.backendSender.GetSenderStats()
}

// DisableDefaultHostname forwards to backend sender
func (s *AggregatingSender) DisableDefaultHostname(disable bool) {
	s.backendSender.DisableDefaultHostname(disable)
}

// SetCheckCustomTags forwards to backend sender
func (s *AggregatingSender) SetCheckCustomTags(tags []string) {
	s.backendSender.SetCheckCustomTags(tags)
}

// SetCheckService forwards to backend sender
func (s *AggregatingSender) SetCheckService(service string) {
	s.backendSender.SetCheckService(service)
}

// SetNoIndex forwards to backend sender
func (s *AggregatingSender) SetNoIndex(noIndex bool) {
	s.backendSender.SetNoIndex(noIndex)
}

// FinalizeCheckServiceTag forwards to backend sender
func (s *AggregatingSender) FinalizeCheckServiceTag() {
	s.backendSender.FinalizeCheckServiceTag()
}

// OrchestratorMetadata forwards directly to backend
func (s *AggregatingSender) OrchestratorMetadata(msgs []types.ProcessMessageBody, clusterID string, nodeType int) {
	s.backendSender.OrchestratorMetadata(msgs, clusterID, nodeType)
}

// OrchestratorManifest forwards directly to backend
func (s *AggregatingSender) OrchestratorManifest(msgs []types.ProcessMessageBody, clusterID string) {
	s.backendSender.OrchestratorManifest(msgs, clusterID)
}

// Commit is a no-op - we flush on our own schedule (original interval)
func (s *AggregatingSender) Commit() {
	// Intentionally empty - we flush on timer at original interval
	// This maintains backend rate while check runs at high frequency
}

// flushLoop periodically flushes aggregated metrics to backend at original interval
func (s *AggregatingSender) flushLoop() {
	ticker := time.NewTicker(s.originalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			s.flush() // Final flush
			return
		case <-ticker.C:
			s.flush()
		}
	}
}

// flush sends aggregated metrics to backend sender
func (s *AggregatingSender) flush() {
	s.mu.Lock()
	aggregated := s.aggregator.flush()
	s.mu.Unlock()

	// Send aggregated metrics to backend
	for _, m := range aggregated {
		switch m.mtype {
		case metrics.GaugeType:
			if m.noIndex {
				s.backendSender.GaugeNoIndex(m.name, m.value, m.hostname, m.tags)
			} else {
				s.backendSender.Gauge(m.name, m.value, m.hostname, m.tags)
			}
		case metrics.RateType:
			s.backendSender.Rate(m.name, m.value, m.hostname, m.tags)
		case metrics.CountType:
			s.backendSender.Count(m.name, m.value, m.hostname, m.tags)
		case metrics.MonotonicCountType:
			if m.flushFirstValue {
				s.backendSender.MonotonicCountWithFlushFirstValue(m.name, m.value, m.hostname, m.tags, true)
			} else {
				s.backendSender.MonotonicCount(m.name, m.value, m.hostname, m.tags)
			}
		case metrics.HistogramType:
			s.backendSender.Histogram(m.name, m.value, m.hostname, m.tags)
		case metrics.HistorateType:
			s.backendSender.Historate(m.name, m.value, m.hostname, m.tags)
		}
	}

	// Flush histogram buckets
	for _, hb := range s.aggregator.flushHistogramBuckets() {
		s.backendSender.HistogramBucket(hb.name, hb.value, hb.lowerBound, hb.upperBound, hb.monotonic, hb.hostname, hb.tags, hb.flushFirstValue)
	}

	// Commit to backend at original interval
	s.backendSender.Commit()
}

// Stop halts the flush loop
func (s *AggregatingSender) Stop() {
	close(s.stopCh)
}

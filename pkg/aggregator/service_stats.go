// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"expvar"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// serviceStatsTracker tracks metric sample counts per service tag
// This allows us to measure the rate of metrics from each service in DogStatsD
type serviceStatsTracker struct {
	mu           sync.RWMutex
	sampleCounts map[string]*expvar.Int // service name -> sample count
}

// newServiceStatsTracker creates a new service statistics tracker
func newServiceStatsTracker() *serviceStatsTracker {
	return &serviceStatsTracker{
		sampleCounts: make(map[string]*expvar.Int),
	}
}

// trackSample extracts the service tag from a metric sample and increments its counter
func (s *serviceStatsTracker) trackSample(sample *metrics.MetricSample) {
	serviceName := extractServiceTag(sample.Tags)
	s.incrementService(serviceName)
}

// extractServiceTag extracts the service name from a tag slice
// Returns empty string if no service tag is found
func extractServiceTag(tags []string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "service:") {
			return strings.TrimPrefix(tag, "service:")
		}
	}
	return ""
}

// incrementService atomically increments the sample count for a service
func (s *serviceStatsTracker) incrementService(serviceName string) {
	s.mu.RLock()
	counter, exists := s.sampleCounts[serviceName]
	s.mu.RUnlock()

	if exists {
		counter.Add(1)
		return
	}

	// Need to create new counter
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	counter, exists = s.sampleCounts[serviceName]
	if exists {
		counter.Add(1)
		return
	}

	// Create new counter
	counter = &expvar.Int{}
	counter.Set(1)
	s.sampleCounts[serviceName] = counter
}

// exportToExpvar exports all service counters to an expvar.Map
func (s *serviceStatsTracker) exportToExpvar(m *expvar.Map) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for serviceName, counter := range s.sampleCounts {
		m.Set(serviceName, counter)
	}
}

// getSnapshot returns a snapshot of current service sample counts
// This is useful for testing and debugging
func (s *serviceStatsTracker) getSnapshot() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := make(map[string]int64, len(s.sampleCounts))
	for serviceName, counter := range s.sampleCounts {
		snapshot[serviceName] = counter.Value()
	}
	return snapshot
}

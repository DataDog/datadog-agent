// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tests

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
)

var _ statsd.ClientInterface = &StatsdClient{}

// StatsdClient is a statsd client for used for tests
type StatsdClient struct {
	sync.RWMutex
	counts map[string]int64
}

// NewStatsdClient returns a new StatsdClient
func NewStatsdClient() *StatsdClient {
	return &StatsdClient{
		counts: make(map[string]int64),
	}
}

// Get return the count
func (s *StatsdClient) Get(key string) int64 {
	s.RLock()
	defer s.RUnlock()
	return s.counts[key]
}

// Gauge does nothing and returns nil
func (s *StatsdClient) Gauge(name string, value float64, tags []string, rate float64) error {
	s.Lock()
	defer s.Unlock()

	if len(tags) == 0 {
		s.counts[name] = int64(value)
	}

	for _, tag := range tags {
		s.counts[name+":"+tag] = int64(value)
	}
	return nil
}

// Count does nothing and returns nil
func (s *StatsdClient) Count(name string, value int64, tags []string, rate float64) error {
	s.Lock()
	defer s.Unlock()

	if len(tags) == 0 {
		s.counts[name] += value
	}

	for _, tag := range tags {
		s.counts[name+":"+tag] += value
	}
	return nil
}

// Histogram does nothing and returns nil
func (s *StatsdClient) Histogram(name string, value float64, tags []string, rate float64) error {
	return nil
}

// Distribution does nothing and returns nil
func (s *StatsdClient) Distribution(name string, value float64, tags []string, rate float64) error {
	return nil
}

// Decr does nothing and returns nil
func (s *StatsdClient) Decr(name string, tags []string, rate float64) error {
	return nil
}

// Incr does nothing and returns nil
func (s *StatsdClient) Incr(name string, tags []string, rate float64) error {
	return nil
}

// Set does nothing and returns nil
func (s *StatsdClient) Set(name string, value string, tags []string, rate float64) error {
	return nil
}

// Timing does nothing and returns nil
func (s *StatsdClient) Timing(name string, value time.Duration, tags []string, rate float64) error {
	return nil
}

// TimeInMilliseconds does nothing and returns nil
func (s *StatsdClient) TimeInMilliseconds(name string, value float64, tags []string, rate float64) error {
	return nil
}

// Event does nothing and returns nil
func (s *StatsdClient) Event(e *statsd.Event) error {
	return nil
}

// SimpleEvent does nothing and returns nil
func (s *StatsdClient) SimpleEvent(title, text string) error {
	return nil
}

// ServiceCheck does nothing and returns nil
func (s *StatsdClient) ServiceCheck(sc *statsd.ServiceCheck) error {
	return nil
}

// SimpleServiceCheck does nothing and returns nil
func (s *StatsdClient) SimpleServiceCheck(name string, status statsd.ServiceCheckStatus) error {
	return nil
}

// Close does nothing and returns nil
func (s *StatsdClient) Close() error {
	return nil
}

// Flush does nothing and returns nil
func (s *StatsdClient) Flush() error {
	s.Lock()
	defer s.Unlock()

	s.counts = make(map[string]int64)
	return nil
}

// IsClosed does nothing and return false
func (s *StatsdClient) IsClosed() bool {
	return false
}

// GetTelemetry does nothing and returns an empty Telemetry
func (s *StatsdClient) GetTelemetry() statsd.Telemetry {
	return statsd.Telemetry{}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package statsdclient holds statsdclient related files
package statsdclient

import (
	"strings"
	"sync"

	"github.com/DataDog/datadog-go/v5/statsd"
)

var _ statsd.ClientInterface = &StatsdClient{}

// StatsdClient is a statsd client for used for tests
type StatsdClient struct {
	statsd.NoOpClient
	lock   sync.RWMutex
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
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.counts[key]
}

// GetByPrefix return the count
func (s *StatsdClient) GetByPrefix(prefix string) map[string]int64 {
	result := make(map[string]int64)

	s.lock.RLock()
	defer s.lock.RUnlock()

	for key, value := range s.counts {
		if strings.HasPrefix(key, prefix) {
			k := strings.Replace(key, prefix, "", -1)
			result[k] = value
		}
	}

	return result
}

// Gauge does nothing and returns nil
func (s *StatsdClient) Gauge(name string, value float64, tags []string, _ float64) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(tags) == 0 {
		s.counts[name] = int64(value)
	}

	for _, tag := range tags {
		s.counts[name+":"+tag] = int64(value)
	}
	return nil
}

// Count does nothing and returns nil
func (s *StatsdClient) Count(name string, value int64, tags []string, _ float64) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(tags) == 0 {
		s.counts[name] += value
	}

	for _, tag := range tags {
		s.counts[name+":"+tag] += value
	}
	return nil
}

// Flush does nothing and returns nil
func (s *StatsdClient) Flush() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.counts = make(map[string]int64)
	return nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package serverstore

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"

	"github.com/prometheus/client_golang/prometheus"
)

// inMemoryStore implements a thread-safe storage for raw and json dumped payloads
type inMemoryStore struct {
	mutex sync.RWMutex

	rawPayloads   map[string][]api.Payload
	totalAppended map[string]int // total payloads ever appended per route (never decreases on cleanup)
	lastAPIKey    string

	// NbPayloads is a prometheus metric to track the number of payloads collected by route
	NbPayloads *prometheus.GaugeVec
}

// newInMemoryStore initialise a new payloads store
func newInMemoryStore() *inMemoryStore {
	return &inMemoryStore{
		mutex:         sync.RWMutex{},
		rawPayloads:   map[string][]api.Payload{},
		totalAppended: map[string]int{},
		NbPayloads: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "payloads",
			Help: "Number of payloads collected by route",
		}, []string{"route"}),
	}
}

func (s *inMemoryStore) SetLastAPIKey(key string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.lastAPIKey = key
}

func (s *inMemoryStore) GetLastAPIKey() (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.lastAPIKey == "" {
		return "", errors.New("no apiKey sent")
	}
	return s.lastAPIKey, nil
}

// AppendPayload adds a payload to the store and tries parsing and adding a dumped json to the parsed store
func (s *inMemoryStore) AppendPayload(route string, apiKey string, data []byte, encoding string, contentType string, collectTime time.Time) error {
	// Set the last APIKey first, to avoid deadlocking
	s.SetLastAPIKey(apiKey)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	rawPayload := api.Payload{
		Timestamp:   collectTime,
		APIKey:      apiKey,
		Data:        data,
		Encoding:    encoding,
		ContentType: contentType,
	}
	s.rawPayloads[route] = append(s.rawPayloads[route], rawPayload)
	s.totalAppended[route]++
	s.NbPayloads.WithLabelValues(route).Set(float64(len(s.rawPayloads[route])))
	return nil
}

// CleanUpPayloadsOlderThan removes payloads older than time
func (s *inMemoryStore) CleanUpPayloadsOlderThan(time time.Time) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	log.Printf("Cleaning up payloads")
	// clean up raw payloads
	for route, payloads := range s.rawPayloads {
		lastInvalidPayloadIndex := -1
		for i, payload := range payloads {
			if payload.Timestamp.Before(time) {
				lastInvalidPayloadIndex = i
			}
		}
		s.rawPayloads[route] = s.rawPayloads[route][lastInvalidPayloadIndex+1:]
		s.NbPayloads.WithLabelValues(route).Set(float64(len(s.rawPayloads[route])))
	}
}

// GetRawPayloads returns payloads collected for route `route`
func (s *inMemoryStore) GetRawPayloads(route string) (payloads []api.Payload) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	payloads = make([]api.Payload, len(s.rawPayloads[route]))
	copy(payloads, s.rawPayloads[route])
	return payloads
}

// GetRawPayloadsAfter returns payloads collected for route `route` that were
// appended after the given cursor. The cursor is the total number of payloads
// ever appended to the route. Because cleanup removes old payloads from the
// front of the slice, the effective start index is adjusted by the number of
// cleaned-up payloads so the client receives only payloads it has not yet seen.
// The returned newCursor is the current total-appended count.
func (s *inMemoryStore) GetRawPayloadsAfter(route string, cursor int) (payloads []api.Payload, newCursor int) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	total := s.totalAppended[route]
	current := s.rawPayloads[route]

	// Number of payloads removed by cleanup = total ever appended - current length.
	cleanedUp := total - len(current)

	// The client has seen `cursor` payloads. Adjust for cleanup so we start
	// at the right offset in the current (compacted) slice.
	start := cursor - cleanedUp
	if start < 0 {
		start = 0
	}
	if start > len(current) {
		start = len(current)
	}

	payloads = make([]api.Payload, len(current)-start)
	copy(payloads, current[start:])
	return payloads, total
}

// GetRouteStats returns stats on collectedraw payloads by route
func (s *inMemoryStore) GetRouteStats() map[string]int {
	statsByRoute := map[string]int{}
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	for route, payloads := range s.rawPayloads {
		statsByRoute[route] = len(payloads)
	}
	return statsByRoute
}

// Flush cleans up any stored payload
func (s *inMemoryStore) Flush() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.rawPayloads = map[string][]api.Payload{}
	s.totalAppended = map[string]int{}
	s.NbPayloads.Reset()
}

// GetInternalMetrics returns the prometheus metrics for the store
func (s *inMemoryStore) GetInternalMetrics() []prometheus.Collector {
	return []prometheus.Collector{s.NbPayloads}
}

// Close is a noop
func (s *inMemoryStore) Close() {}

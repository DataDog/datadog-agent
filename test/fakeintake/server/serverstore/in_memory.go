// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package serverstore

import (
	"errors"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"

	"github.com/prometheus/client_golang/prometheus"
)

// inMemoryStore implements a thread-safe storage for raw and json dumped payloads
type inMemoryStore struct {
	mutex sync.RWMutex

	rawPayloads map[string][]api.Payload
	lastAPIKey  string

	// Strictly increasing sequence number assigned to each payload
	nextSeq uint64

	// NbPayloads is a prometheus metric to track the number of payloads collected by route
	NbPayloads *prometheus.GaugeVec
}

// newInMemoryStore initialise a new payloads store
func newInMemoryStore() *inMemoryStore {
	return &inMemoryStore{
		mutex:       sync.RWMutex{},
		rawPayloads: map[string][]api.Payload{},
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
	s.nextSeq++
	rawPayload := api.Payload{
		Timestamp:   collectTime,
		APIKey:      apiKey,
		Data:        data,
		Encoding:    encoding,
		ContentType: contentType,
		Seq:         s.nextSeq,
	}
	s.rawPayloads[route] = append(s.rawPayloads[route], rawPayload)
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

// GetRawPayloads returns raw payloads for a given route with sequence number `Seq` greater than afterSeq,
// up to `limit` payloads and `hasMore` if there are more payloads beyond the returned page.
// A `limit` of 0 returns all matching payloads.
func (s *inMemoryStore) GetRawPayloads(route string, afterSeq uint64, limit int) (payloads []api.Payload, hasMore bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	all := s.rawPayloads[route]
	start := sort.Search(len(all), func(i int) bool {
		return all[i].Seq > afterSeq
	})

	end := len(all)
	if limit > 0 && start+limit < end {
		end = start + limit
		hasMore = true
	}

	payloads = make([]api.Payload, end-start)
	copy(payloads, all[start:end])
	return payloads, hasMore
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
	s.nextSeq = 0
	s.NbPayloads.Reset()
}

// GetInternalMetrics returns the prometheus metrics for the store
func (s *inMemoryStore) GetInternalMetrics() []prometheus.Collector {
	return []prometheus.Collector{s.NbPayloads}
}

// Close is a noop
func (s *inMemoryStore) Close() {}

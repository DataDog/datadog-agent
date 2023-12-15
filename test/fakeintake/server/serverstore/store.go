// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverstore implements storing logic for fakeintake server
// Stores raw payloads and try parsing known payloads dumping them to json
package serverstore

import (
	"log"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/prometheus/client_golang/prometheus"
)

// Store implements a thread-safe storage for raw and json dumped payloads
type Store struct {
	mutex sync.RWMutex

	rawPayloads  map[string][]api.Payload
	jsonPayloads map[string][]api.ParsedPayload

	NbPayloads *prometheus.GaugeVec
}

// NewStore initialise a new payloads store
func NewStore() *Store {
	return &Store{
		mutex:        sync.RWMutex{},
		rawPayloads:  map[string][]api.Payload{},
		jsonPayloads: map[string][]api.ParsedPayload{},
		NbPayloads: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "payloads",
			Help: "Number of payloads collected by route",
		}, []string{"route"}),
	}
}

// AppendPayload adds a payload to the store and tries parsing and adding a dumped json to the parsed store
func (s *Store) AppendPayload(route string, data []byte, encoding string, collectTime time.Time) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	rawPayload := api.Payload{
		Timestamp: collectTime,
		Data:      data,
		Encoding:  encoding,
	}
	s.rawPayloads[route] = append(s.rawPayloads[route], rawPayload)
	s.NbPayloads.WithLabelValues(route).Set(float64(len(s.rawPayloads[route])))
	return s.tryParseAndAppendPayload(rawPayload, route)
}

func (s *Store) tryParseAndAppendPayload(rawPayload api.Payload, route string) error {
	if parsePayload, ok := parserMap[route]; ok {
		var err error
		data, err := parsePayload(rawPayload)
		if err != nil {
			return err
		}
		parsedPayload := api.ParsedPayload{
			Timestamp: rawPayload.Timestamp,
			Data:      data,
			Encoding:  rawPayload.Encoding,
		}

		s.jsonPayloads[route] = append(s.jsonPayloads[route], parsedPayload)
	}
	return nil
}

// CleanUpPayloadsOlderThan removes payloads older than time
func (s *Store) CleanUpPayloadsOlderThan(time time.Time) {
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
	// clean up parsed payloads
	for route, payloads := range s.jsonPayloads {
		// cleanup raw store
		lastInvalidPayloadIndex := -1
		for i, payload := range payloads {
			if payload.Timestamp.Before(time) {
				lastInvalidPayloadIndex = i
			}
		}
		s.jsonPayloads[route] = s.jsonPayloads[route][lastInvalidPayloadIndex+1:]
	}
}

// GetRawPayloads returns payloads collected for route `route`
func (s *Store) GetRawPayloads(route string) (payloads []api.Payload) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	payloads = make([]api.Payload, len(s.rawPayloads[route]))
	copy(payloads, s.rawPayloads[route])
	return payloads
}

// GetJSONPayloads returns payloads collected and parsed to json for route `route`
func (s *Store) GetJSONPayloads(route string) (payloads []api.ParsedPayload) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	payloads = make([]api.ParsedPayload, len(s.jsonPayloads[route]))
	copy(payloads, s.jsonPayloads[route])
	return payloads
}

// GetRouteStats returns stats on collectedraw payloads by route
func (s *Store) GetRouteStats() map[string]int {
	statsByRoute := map[string]int{}
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	for route, payloads := range s.rawPayloads {
		statsByRoute[route] = len(payloads)
	}
	return statsByRoute
}

// Flush cleans up any stored payload
func (s *Store) Flush() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.rawPayloads = map[string][]api.Payload{}
	s.jsonPayloads = map[string][]api.ParsedPayload{}
	s.NbPayloads.Reset()
}

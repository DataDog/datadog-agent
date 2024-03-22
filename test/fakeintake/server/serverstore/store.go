// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverstore implements storing logic for fakeintake server
// Stores raw payloads and try parsing known payloads dumping them to json
package serverstore

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// Store implements a thread-safe storage for raw and json dumped payloads
type Store struct {
	mutex sync.RWMutex

	rawPayloads map[string][]api.Payload

	NbPayloads *prometheus.GaugeVec
}

// NewStore initialise a new payloads store
func NewStore() *Store {
	return &Store{
		mutex:       sync.RWMutex{},
		rawPayloads: map[string][]api.Payload{},
		NbPayloads: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "payloads",
			Help: "Number of payloads collected by route",
		}, []string{"route"}),
	}
}

// AppendPayload adds a payload to the store and tries parsing and adding a dumped json to the parsed store
func (s *Store) AppendPayload(route string, data []byte, encoding string, collectTime time.Time) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	rawPayload := api.Payload{
		Timestamp: collectTime,
		Data:      data,
		Encoding:  encoding,
	}
	s.rawPayloads[route] = append(s.rawPayloads[route], rawPayload)
	s.NbPayloads.WithLabelValues(route).Set(float64(len(s.rawPayloads[route])))
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
func (s *Store) GetJSONPayloads(route string) (payloads []api.ParsedPayload, err error) {
	if _, found := parserMap[route]; !found {
		// Short path to returns directly if no parser is registered for the given route.
		// No need to acquire the lock in that case.
		return nil, fmt.Errorf("Json payload not supported for this route")
	}
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	payloads = make([]api.ParsedPayload, 0, len(s.rawPayloads[route]))
	for _, raw := range s.rawPayloads[route] {
		if jsonPayload, err := s.encodeToJSONRawPayload(raw, route); err == nil && jsonPayload != nil {
			payloads = append(payloads, *jsonPayload)
		}
	}
	return payloads, nil
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
	s.NbPayloads.Reset()
}

// encodeToJSONRawPayload used to decode a raw Payload into a Json Payload
// to know how to parse the raw payload that could be JSON or Protobuf, the function
// need to know the route.
func (s *Store) encodeToJSONRawPayload(rawPayload api.Payload, route string) (*api.ParsedPayload, error) {
	if parsePayload, ok := parserMap[route]; ok {
		var err error
		data, err := parsePayload(rawPayload)
		if err != nil {
			return nil, err
		}
		parsedPayload := &api.ParsedPayload{
			Timestamp: rawPayload.Timestamp,
			Data:      data,
			Encoding:  rawPayload.Encoding,
		}
		return parsedPayload, nil
	}
	return nil, nil
}

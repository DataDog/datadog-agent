// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverstore implements storing logic for fakeintake server
// Stores raw payloads and try parsing known payloads dumping them to json
package serverstore

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// Store is the interface for a store that can store payloads and try parsing them
type Store interface {
	// AppendPayload adds a payload to the store and tries parsing and adding a dumped json to the parsed store
	AppendPayload(route string, data []byte, encoding string, collectTime time.Time) error
	// CleanUpPayloadsOlderThan removes payloads older than the given time
	CleanUpPayloadsOlderThan(time.Time)
	// GetRawPayloads returns all raw payloads for a given route
	GetRawPayloads(route string) []api.Payload
	// GetRouteStats returns the number of payloads for each route
	GetRouteStats() map[string]int
	// Flush flushes the store
	Flush()
	// GetInternalMetrics returns the prometheus metrics for the store
	GetInternalMetrics() []prometheus.Collector
	// Close closes the store
	Close()
}

// NewStore returns a new store
func NewStore() Store {
	return newInMemoryStore()
}

// GetJSONPayloads returns the parsed payloads for a given route
func GetJSONPayloads(store Store, route string) ([]api.ParsedPayload, error) {
	parser, ok := parserMap[route]
	if !ok {
		return nil, fmt.Errorf("no parser for route %s", route)
	}
	payloads := store.GetRawPayloads(route)
	parsedPayloads := make([]api.ParsedPayload, 0, len(payloads))
	var errs []error
	for _, payload := range payloads {
		parsedPayload, err := parser(payload)
		if err != nil {
			log.Printf("failed to parse payload %+v: %v\n", payload, err)
			errs = append(errs, err)
			continue
		}
		parsedPayloads = append(parsedPayloads, api.ParsedPayload{
			Timestamp: payload.Timestamp,
			Data:      parsedPayload,
			Encoding:  payload.Encoding,
		})
	}
	return parsedPayloads, errors.Join(errs...)
}

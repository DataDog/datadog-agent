// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverstore implements storing logic for fakeintake server
// Stores raw payloads and try parsing known payloads dumping them to json
package serverstore

import (
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// Store is the interface for a store that can store payloads and try parsing them
type Store interface {
	// AppendPayload adds a payload to the store and tries parsing and adding a dumped json to the parsed store
	AppendPayload(route string, data []byte, encoding string, collectTime time.Time) error
	// CleanUpPayloadsOlderThan removes payloads older than the given time
	CleanUpPayloadsOlderThan(time time.Time)
	// GetRawPayloads returns all raw payloads for a given route
	GetRawPayloads(route string) (payloads []api.Payload)
	// GetJSONPayloads returns all parsed payloads for a given route
	GetJSONPayloads(route string) (payloads []api.ParsedPayload)
	// GetRouteStats returns the number of payloads for each route
	GetRouteStats() map[string]int
	// Flush flushes the store
	Flush()
}

func NewStore() Store {
	return NewInMemoryStore()
}

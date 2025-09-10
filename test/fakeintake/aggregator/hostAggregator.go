// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aggregator

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// Host struct contains agents host-tags payload and attributes to fit Aggregator implementation
//
// Use a pointer to assign `HostTags` inner struct.
// When we ParseHostPayload we receive variaous payload types.
// We only want to keep those with host-tags
// Using a pointer allows us to check if the pointer has been allocated
// and therefore found the right payload
type Host struct {
	HostTags *struct {
		System []string `json:"system"`
	} `json:"host-tags"`

	InternalHostname string `json:"internalHostname"`
	collectedTime    time.Time
}

// GetCollectedTime return the time the payload was collected
func (host *Host) GetCollectedTime() time.Time {
	return host.collectedTime
}

// GetTags returns the tags collected by the payload
// currently none
func (host *Host) GetTags() []string {
	return nil
}

// name return the payload name
func (host *Host) name() string {
	return host.InternalHostname
}

// ParseHostPayload parses the generic payload and returns a typed struct with hostImpl data
func ParseHostPayload(payload api.Payload) ([]*Host, error) {
	if len(payload.Data) == 0 {
		return nil, nil
	}

	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, fmt.Errorf("failed to inflate host Payload: %w", err)
	}

	var data = &Host{}

	if err := json.Unmarshal(inflated, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshall payload: %w", err)
	}

	// the current route returns different payload types,
	// we only want to keep the matching payloads with host information
	// return an empty list with no error to skip this non-matching payload
	if data.HostTags == nil {
		return []*Host{}, nil
	}

	// set hostname and collected time
	data.collectedTime = payload.Timestamp

	return []*Host{data}, nil
}

// HostAggregator structure
type HostAggregator struct {
	Aggregator[*Host]
}

// NewHostAggregator returns a new Host aggregator
func NewHostAggregator() HostAggregator {
	return HostAggregator{
		Aggregator: newAggregator(ParseHostPayload),
	}
}

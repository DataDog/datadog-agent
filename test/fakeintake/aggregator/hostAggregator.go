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

// HostTags struct contains agents host-tags payload and attributes to fit Aggregator implementation
//
// Use a pointer to assign `HostTags` inner struct.
// When we ParseHostPayload we receive variaous payload types.
// We only want to keep those with host-tags
// Using a pointer allows us to check if the pointer has been allocated
// and therefore found the right payload
type HostTags struct {
	InternalHostname string
	HostTags         []string

	collectedTime time.Time
}

// GetCollectedTime return the time the payload was collected
func (host *HostTags) GetCollectedTime() time.Time {
	return host.collectedTime
}

// GetTags returns the tags collected by the payload
// currently none
func (host *HostTags) GetTags() []string {
	return nil
}

// name return the payload name
func (host *HostTags) name() string {
	return host.InternalHostname
}

// ParseHostTagsPayload parses the generic payload and returns a typed struct with hostImpl data
func ParseHostTagsPayload(payload api.Payload) ([]*HostTags, error) {
	if len(payload.Data) == 0 {
		return nil, nil
	}

	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, fmt.Errorf("failed to inflate host Payload: %w", err)
	}

	var data struct {
		HostName string `json:"internalHostname"`
		HostTags *struct {
			System []string `json:"system"`
		} `json:"host-tags"`
	}

	if err := json.Unmarshal(inflated, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshall payload: %w", err)
	}

	// the current route returns different payload types,
	// we only want to keep the matching payloads with host information
	// return an empty list with no error to skip this non-matching payload
	if data.HostTags == nil {
		return []*HostTags{}, nil
	}

	return []*HostTags{
		{
			collectedTime:    payload.Timestamp,
			InternalHostname: data.HostName,
			HostTags:         data.HostTags.System,
		},
	}, nil
}

// HostTagsAggregator structure
type HostTagsAggregator struct {
	Aggregator[*HostTags]
}

// NewHostTagsAggregator returns a new Host aggregator
func NewHostTagsAggregator() HostTagsAggregator {
	return HostTagsAggregator{
		Aggregator: newAggregator(ParseHostTagsPayload),
	}
}

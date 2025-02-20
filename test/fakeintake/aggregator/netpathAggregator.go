// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// Netpath represents a network path payload
type Netpath struct {
	collectedTime time.Time
	payload.NetworkPath
}

func (p *Netpath) name() string {
	return fmt.Sprintf("%s:%d %s", p.Destination.Hostname, p.Destination.Port, p.Protocol)
}

// GetTags return the tags from a payload
func (p *Netpath) GetTags() []string {
	return []string{}
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (p *Netpath) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseNetpathPayload parses an api.Payload into a list of Netpath
func ParseNetpathPayload(payload api.Payload) (netpaths []*Netpath, err error) {
	if len(payload.Data) == 0 || bytes.Equal(payload.Data, []byte("{}")) {
		return []*Netpath{}, nil
	}
	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	netpaths = []*Netpath{}
	err = json.Unmarshal(inflated, &netpaths)
	if err != nil {
		return nil, err
	}
	for _, n := range netpaths {
		n.collectedTime = payload.Timestamp
	}
	return netpaths, err
}

// NetpathAggregator is an Aggregator for netpath payloads
type NetpathAggregator struct {
	Aggregator[*Netpath]
}

// NewNetpathAggregator return a new NetpathAggregator
func NewNetpathAggregator() NetpathAggregator {
	return NetpathAggregator{
		Aggregator: newAggregator(ParseNetpathPayload),
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/comp/netflow/payload"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// NDMFlow represents an ndmflow payload
type NDMFlow struct {
	collectedTime time.Time
	payload.FlowPayload
}

func (p *NDMFlow) name() string {
	return p.Host
}

// GetTags return the tags from a payload
func (p *NDMFlow) GetTags() []string {
	return []string{}
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (p *NDMFlow) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseNDMFlowPayload parses an api.Payload into a list of NDMFlow
func ParseNDMFlowPayload(payload api.Payload) (ndmflows []*NDMFlow, err error) {
	if len(payload.Data) == 0 || bytes.Equal(payload.Data, []byte("{}")) {
		return []*NDMFlow{}, nil
	}
	enflated, err := enflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	ndmflows = []*NDMFlow{}
	err = json.Unmarshal(enflated, &ndmflows)
	if err != nil {
		return nil, err
	}
	for _, n := range ndmflows {
		n.collectedTime = payload.Timestamp
	}
	return ndmflows, err
}

// NDMFlowAggregator is an Aggregator for ndmflow payloads
type NDMFlowAggregator struct {
	Aggregator[*NDMFlow]
}

// NewNDMFlowAggregator return a new NDMFlowAggregator
func NewNDMFlowAggregator() NDMFlowAggregator {
	return NDMFlowAggregator{
		Aggregator: newAggregator(ParseNDMFlowPayload),
	}
}

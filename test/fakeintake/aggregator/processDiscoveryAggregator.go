// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// ProcessDiscoveryPayload is a payload type for the process_discovery check
type ProcessDiscoveryPayload struct {
	agentmodel.CollectorProcDiscovery
	collectedTime time.Time
}

func (p ProcessDiscoveryPayload) name() string {
	return p.HostName
}

// GetTags is not implemented for process_discovery payloads
func (p ProcessDiscoveryPayload) GetTags() []string {
	return nil
}

// GetCollectedTime returns the time that the payload was received by the fake intake
func (p ProcessDiscoveryPayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseProcessDiscoveryPayload parses an api.Payload into a list of ProcessDiscoveryPayload
func ParseProcessDiscoveryPayload(payload api.Payload) ([]*ProcessDiscoveryPayload, error) {
	msg, err := agentmodel.DecodeMessage(payload.Data)
	if err != nil {
		return nil, err
	}

	switch m := msg.Body.(type) {
	case *agentmodel.CollectorProcDiscovery:
		return []*ProcessDiscoveryPayload{
			{CollectorProcDiscovery: *m, collectedTime: payload.Timestamp},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected type %s", msg.Header.Type)
	}
}

// ProcessDiscoveryAggregator is an Aggregator for ProcessDiscoveryPayload
type ProcessDiscoveryAggregator struct {
	Aggregator[*ProcessDiscoveryPayload]
}

// NewProcessDiscoveryAggregator returns a new ProcessDiscoveryAggregator
func NewProcessDiscoveryAggregator() ProcessDiscoveryAggregator {
	return ProcessDiscoveryAggregator{
		Aggregator: newAggregator(ParseProcessDiscoveryPayload),
	}
}

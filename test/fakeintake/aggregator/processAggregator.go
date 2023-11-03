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

// ProcessPayload is a payload type for the process check
type ProcessPayload struct {
	agentmodel.CollectorProc
	collectedTime time.Time
}

func (p ProcessPayload) name() string {
	return p.HostName
}

// GetTags is not implemented for process payloads
func (p ProcessPayload) GetTags() []string {
	return nil
}

// GetCollectedTime returns the time that the payload was received by the fake intake
func (p ProcessPayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseProcessPayload parses an api.Payload into a list of ProcessPayload
func ParseProcessPayload(payload api.Payload) ([]*ProcessPayload, error) {
	msg, err := agentmodel.DecodeMessage(payload.Data)
	if err != nil {
		return nil, err
	}

	switch m := msg.Body.(type) {
	case *agentmodel.CollectorProc:
		return []*ProcessPayload{{CollectorProc: *m, collectedTime: payload.Timestamp}}, nil
	default:
		return nil, fmt.Errorf("unexpected type %s", msg.Header.Type)
	}
}

// ProcessAggregator is an Aggregator for ProcessPayload
type ProcessAggregator struct {
	Aggregator[*ProcessPayload]
}

// NewProcessAggregator returns a new ProcessAggregator
func NewProcessAggregator() ProcessAggregator {
	return ProcessAggregator{
		Aggregator: newAggregator(ParseProcessPayload),
	}
}

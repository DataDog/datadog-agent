// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"

	"google.golang.org/protobuf/proto"
)

// TracePayload is a payload type for traces
type TracePayload struct {
	*pb.AgentPayload
	collectedTime time.Time
}

// name returns the hostname of the agent that produced the payload
func (tp *TracePayload) name() string {
	return tp.HostName
}

// GetTags is not implemented
func (tp *TracePayload) GetTags() []string {
	return nil
}

// GetCollectedTime returns the time that the payload was received by the fake intake
func (tp *TracePayload) GetCollectedTime() time.Time {
	return tp.collectedTime
}

// ParseTracePayload parses an api.Payload into a list of TracePayload
func ParseTracePayload(payload api.Payload) ([]*TracePayload, error) {
	enflated, err := enflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	var p pb.AgentPayload
	if err := proto.Unmarshal(enflated, &p); err != nil {
		return nil, err
	}
	return []*TracePayload{{AgentPayload: &p, collectedTime: payload.Timestamp}}, nil
}

// TraceAggregator is an Aggregator for TracePayload
type TraceAggregator struct {
	Aggregator[*TracePayload]
}

// NewTraceAggregator returns a new TraceAggregator
func NewTraceAggregator() TraceAggregator {
	return TraceAggregator{
		Aggregator: newAggregator(ParseTracePayload),
	}
}

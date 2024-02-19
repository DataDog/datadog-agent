// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"

	"github.com/tinylib/msgp/msgp"
)

// APMStatsPayload is a payload type for APM stats
// It implements PayloadItem
type APMStatsPayload struct {
	*pb.StatsPayload
	collectedTime time.Time
}

var _ PayloadItem = &APMStatsPayload{}

// name returns the hostname of the agent that produced the payload
func (p *APMStatsPayload) name() string {
	return p.AgentHostname
}

// GetTags is not implemented
func (p *APMStatsPayload) GetTags() []string {
	return nil
}

// GetCollectedTime returns the time that the payload was received by the fake intake
// APMStatsPayload implements PayloadItem
func (p *APMStatsPayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseAPMStatsPayload parses an api.Payload into a list of APMStatsPayload
func ParseAPMStatsPayload(payload api.Payload) ([]*APMStatsPayload, error) {
	rc, err := getReadCloserForEncoding(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var p pb.StatsPayload
	if err := msgp.Decode(rc, &p); err != nil {
		return nil, err
	}
	return []*APMStatsPayload{{StatsPayload: &p, collectedTime: payload.Timestamp}}, nil
}

// APMStatsAggregator is an Aggregator for APMStatsPayload
type APMStatsAggregator struct {
	Aggregator[*APMStatsPayload]
}

// NewAPMStatsAggregator returns a new APMStatsAggregator
func NewAPMStatsAggregator() APMStatsAggregator {
	return APMStatsAggregator{
		Aggregator: newAggregator(ParseAPMStatsPayload),
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/genresources"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// GenResourcePayload is a generic resource event collected by fakeintake.
type GenResourcePayload struct {
	genresources.GenericResourceEvent
	collectedTime time.Time
}

func (p *GenResourcePayload) name() string {
	resource := p.GetResource()
	return resource.GetType() + ":" + resource.GetKey()
}

// GetTags returns no tags for generic resource events.
func (p *GenResourcePayload) GetTags() []string {
	return nil
}

// GetCollectedTime returns when fakeintake received the event.
func (p *GenResourcePayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseGenResources parses the single protobuf event in a genresources request.
func ParseGenResources(payload api.Payload) ([]*GenResourcePayload, error) {
	if len(payload.Data) == 0 {
		return nil, nil
	}
	data, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}

	// Event platform connectivity checks may send an empty JSON object.
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("{}")) {
		return nil, nil
	}

	result := &GenResourcePayload{collectedTime: payload.Timestamp}
	if err := proto.Unmarshal(data, &result.GenericResourceEvent); err != nil {
		return nil, err
	}
	return []*GenResourcePayload{result}, nil
}

// GenResourcesAggregator stores events by resource type and key.
type GenResourcesAggregator struct {
	Aggregator[*GenResourcePayload]
}

// NewGenResourcesAggregator returns a new GenResourcesAggregator.
func NewGenResourcesAggregator() GenResourcesAggregator {
	return GenResourcesAggregator{Aggregator: newAggregator(ParseGenResources)}
}

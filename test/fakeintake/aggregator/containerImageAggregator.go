// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/contimage"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"

	"google.golang.org/protobuf/proto"
)

// ContainerImagePayload is a payload type for the container image check
type ContainerImagePayload struct {
	*agentmodel.ContainerImage
	collectedTime time.Time
}

func (p *ContainerImagePayload) name() string {
	return p.Name
}

// GetTags return the tags from a payload
func (p *ContainerImagePayload) GetTags() []string {
	return p.DdTags
}

// GetCollectedTime returns the time that the payload was received by the fake intake
func (p *ContainerImagePayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseContainerImagePayload parses an api.Payload into a list of ContainerImagePayload
func ParseContainerImagePayload(payload api.Payload) ([]*ContainerImagePayload, error) {
	enflated, err := enflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, fmt.Errorf("could not enflate payload: %w", err)
	}

	msg := agentmodel.ContainerImagePayload{}
	if err := proto.Unmarshal(enflated, &msg); err != nil {
		return nil, err
	}

	payloads := make([]*ContainerImagePayload, len(msg.Images))
	for i, containerImage := range msg.Images {
		payloads[i] = &ContainerImagePayload{ContainerImage: containerImage, collectedTime: payload.Timestamp}
	}
	return payloads, nil
}

// ContainerImageAggregator is an Aggregator for ContainerImagePayload
type ContainerImageAggregator struct {
	Aggregator[*ContainerImagePayload]
}

// NewContainerImageAggregator returns a new ContainerImageAggregator
func NewContainerImageAggregator() ContainerImageAggregator {
	return ContainerImageAggregator{
		Aggregator: newAggregator(ParseContainerImagePayload),
	}
}

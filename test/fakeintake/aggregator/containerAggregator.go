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

// ContainerPayload is a payload type for the container check
type ContainerPayload struct {
	agentmodel.CollectorContainer
	collectedTime time.Time
}

func (p ContainerPayload) name() string {
	return p.HostName
}

// GetTags is not implemented for container payloads
func (p ContainerPayload) GetTags() []string {
	return nil
}

// GetCollectedTime returns the time that the payload was received by the fake intake
func (p ContainerPayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseContainerPayload parses an api.Payload into a list of ContainerPayload
func ParseContainerPayload(payload api.Payload) ([]*ContainerPayload, error) {
	msg, err := agentmodel.DecodeMessage(payload.Data)
	if err != nil {
		return nil, err
	}

	switch m := msg.Body.(type) {
	case *agentmodel.CollectorContainer:
		return []*ContainerPayload{{CollectorContainer: *m, collectedTime: payload.Timestamp}}, nil
	default:
		return nil, fmt.Errorf("unexpected type %s", msg.Header.Type)
	}
}

// ContainerAggregator is an Aggregator for ContainerPayload
type ContainerAggregator struct {
	Aggregator[*ContainerPayload]
}

// NewContainerAggregator returns a new ContainerAggregator
func NewContainerAggregator() ContainerAggregator {
	return ContainerAggregator{
		Aggregator: newAggregator(ParseContainerPayload),
	}
}

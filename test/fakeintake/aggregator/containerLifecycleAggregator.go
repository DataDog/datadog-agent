// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/contlcycle"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"

	"google.golang.org/protobuf/proto"
)

// ContainerLifecyclePayload is a payload type for the container lifecycle check
type ContainerLifecyclePayload struct {
	*agentmodel.Event
	collectedTime time.Time
}

func (p *ContainerLifecyclePayload) name() string {
	if container := p.Event.GetContainer(); container != nil {
		return fmt.Sprintf("container_id://%s", container.GetContainerID())
	} else if pod := p.Event.GetPod(); pod != nil {
		return fmt.Sprintf("kubernetes_pod_uid://%s", pod.GetPodUID())
	}
	return ""
}

// GetTags is not implemented for container lifecycle payloads
func (p *ContainerLifecyclePayload) GetTags() []string {
	return nil
}

// GetCollectedTime returns the time that the payload was received by the fake intake
func (p *ContainerLifecyclePayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseContainerLifecyclePayload parses an api.Payload into a list of ContainerLifecyclePayload
func ParseContainerLifecyclePayload(payload api.Payload) ([]*ContainerLifecyclePayload, error) {
	enflated, err := enflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, fmt.Errorf("could not enflate payload: %w", err)
	}

	msg := agentmodel.EventsPayload{}
	if err := proto.Unmarshal(enflated, &msg); err != nil {
		return nil, err
	}

	events := make([]*ContainerLifecyclePayload, len(msg.Events))
	for i, event := range msg.Events {
		events[i] = &ContainerLifecyclePayload{Event: event, collectedTime: payload.Timestamp}
	}
	return events, nil
}

// ContainerLifecycleAggregator is an Aggregator for ContainerLifecyclePayload
type ContainerLifecycleAggregator struct {
	Aggregator[*ContainerLifecyclePayload]
}

// NewContainerLifecycleAggregator returns a new ContainerLifecycleAggregator
func NewContainerLifecycleAggregator() ContainerLifecycleAggregator {
	return ContainerLifecycleAggregator{
		Aggregator: newAggregator(ParseContainerLifecyclePayload),
	}
}

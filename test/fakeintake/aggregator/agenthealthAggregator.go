// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// AgentHealthPayload represents a health report payload from the agent
type AgentHealthPayload struct {
	*healthplatform.HealthReport
	collectedTime time.Time
}

func (ahp *AgentHealthPayload) name() string {
	if ahp.HealthReport != nil && ahp.HealthReport.Host != nil {
		return ahp.HealthReport.Host.Hostname
	}
	return ""
}

// GetTags return the tags from a payload
func (ahp *AgentHealthPayload) GetTags() []string {
	return []string{}
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (ahp *AgentHealthPayload) GetCollectedTime() time.Time {
	return ahp.collectedTime
}

// ParseAgentHealthPayload parses the agent health payload from the API
func ParseAgentHealthPayload(payload api.Payload) (agentHealthPayloads []*AgentHealthPayload, err error) {
	if bytes.Equal(payload.Data, []byte("{}")) {
		// empty JSON object
		return []*AgentHealthPayload{}, nil
	}

	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}

	var healthReport healthplatform.HealthReport
	err = json.Unmarshal(inflated, &healthReport)
	if err != nil {
		return nil, err
	}

	agentHealthPayload := &AgentHealthPayload{
		HealthReport:  &healthReport,
		collectedTime: payload.Timestamp,
	}

	return []*AgentHealthPayload{agentHealthPayload}, nil
}

// AgentHealthAggregator is a type that represents an agent health aggregator
type AgentHealthAggregator struct {
	Aggregator[*AgentHealthPayload]
}

// NewAgentHealthAggregator returns a new agent health aggregator
func NewAgentHealthAggregator() AgentHealthAggregator {
	return AgentHealthAggregator{
		Aggregator: newAggregator(ParseAgentHealthPayload),
	}
}

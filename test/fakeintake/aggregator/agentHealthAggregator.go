// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aggregator

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// Issue represents an individual issue in the health report.
// Only includes fields needed for fakeintake testing.
type Issue struct {
	ID       string         `json:"ID"`
	Category string         `json:"Category"`
	Title    string         `json:"Title"`
	Tags     []string       `json:"Tags"`
	Extra    map[string]any `json:"Extra,omitempty"`
}

// HealthReport represents the formatted health report structure.
// Only includes fields needed for fakeintake testing.
type HealthReport struct {
	SchemaVersion string            `json:"schema_version"`
	EventType     string            `json:"event_type"`
	Host          HostInfo          `json:"host"`
	Issues        map[string]*Issue `json:"issues"`
}

// HostInfo represents the host information in the health report.
// Only includes fields needed for fakeintake testing.
type HostInfo struct {
	Hostname     string `json:"hostname"`
	AgentVersion string `json:"agent_version"`
}

// AgentHealthPayload wraps the HealthReport from the component with fakeintake metadata
type AgentHealthPayload struct {
	HealthReport
	collectedTime time.Time
}

func (ahp *AgentHealthPayload) name() string {
	return ahp.Host.Hostname
}

// GetTags return the tags from a payload (agent health doesn't have top-level tags)
func (ahp *AgentHealthPayload) GetTags() []string {
	return []string{}
}

// GetCollectedTime return the time when the payload has been collected by the fakeintake server
func (ahp *AgentHealthPayload) GetCollectedTime() time.Time {
	return ahp.collectedTime
}

// ParseAgentHealthPayload parses agent health payloads from the raw API payload
func ParseAgentHealthPayload(payload api.Payload) (payloads []*AgentHealthPayload, err error) {
	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}

	var healthReport HealthReport
	err = json.Unmarshal(inflated, &healthReport)
	if err != nil {
		return nil, err
	}

	agentHealthPayload := &AgentHealthPayload{
		HealthReport:  healthReport,
		collectedTime: payload.Timestamp,
	}

	return []*AgentHealthPayload{agentHealthPayload}, nil
}

// AgentHealthAggregator aggregates agent health payloads
type AgentHealthAggregator struct {
	Aggregator[*AgentHealthPayload]
}

// NewAgentHealthAggregator returns a new agent health aggregator
func NewAgentHealthAggregator() AgentHealthAggregator {
	return AgentHealthAggregator{
		Aggregator: newAggregator(ParseAgentHealthPayload),
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// AgentDiscoveryPayload represents an Agent Discovery payload sent through Event Platform.
type AgentDiscoveryPayload struct {
	collectedTime time.Time
	RawPayload    map[string]json.RawMessage `json:"-"`
	Integration   string                     `json:"integration"`
	ServiceID     string                     `json:"service_id"`
	Runtime       string                     `json:"runtime"`
	Configs       []AgentDiscoveryConfig     `json:"configs"`
}

// AgentDiscoveryConfig represents a discovered configuration object.
type AgentDiscoveryConfig struct {
	Type          string `json:"type"`
	Path          string `json:"path"`
	ContentBase64 string `json:"content_base64"`
	Truncated     bool   `json:"truncated"`
}

func (p *AgentDiscoveryPayload) name() string {
	if p.ServiceID == "" {
		return p.Integration
	}
	return p.Integration + ":" + p.ServiceID
}

// GetTags returns no tags for Agent Discovery payloads.
func (p *AgentDiscoveryPayload) GetTags() []string {
	return []string{}
}

// GetCollectedTime returns the time when the payload has been collected by the fakeintake server.
func (p *AgentDiscoveryPayload) GetCollectedTime() time.Time {
	return p.collectedTime
}

// ParseAgentDiscoveryPayload parses an api.Payload into Agent Discovery payloads.
func ParseAgentDiscoveryPayload(payload api.Payload) ([]*AgentDiscoveryPayload, error) {
	if len(payload.Data) == 0 {
		return []*AgentDiscoveryPayload{}, nil
	}

	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	inflated = bytes.TrimSpace(inflated)
	if len(inflated) == 0 || bytes.Equal(inflated, []byte("{}")) {
		return []*AgentDiscoveryPayload{}, nil
	}

	rawPayloads := []json.RawMessage{}
	if inflated[0] == '[' {
		if err := json.Unmarshal(inflated, &rawPayloads); err != nil {
			return nil, err
		}
	} else {
		rawPayloads = append(rawPayloads, json.RawMessage(inflated))
	}

	payloads := make([]*AgentDiscoveryPayload, 0, len(rawPayloads))
	for _, rawPayload := range rawPayloads {
		parsedPayload := AgentDiscoveryPayload{
			collectedTime: payload.Timestamp,
		}
		if err := json.Unmarshal(rawPayload, &parsedPayload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(rawPayload, &parsedPayload.RawPayload); err != nil {
			return nil, err
		}
		payloads = append(payloads, &parsedPayload)
	}

	return payloads, nil
}

// AgentDiscoveryAggregator is an Aggregator for Agent Discovery payloads.
type AgentDiscoveryAggregator struct {
	Aggregator[*AgentDiscoveryPayload]
}

// NewAgentDiscoveryAggregator returns a new AgentDiscoveryAggregator.
func NewAgentDiscoveryAggregator() AgentDiscoveryAggregator {
	return AgentDiscoveryAggregator{
		Aggregator: newAggregator(ParseAgentDiscoveryPayload),
	}
}

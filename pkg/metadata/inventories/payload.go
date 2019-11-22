// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package inventories

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// AgentMetadata contains metadata provided by the agent itself
type AgentMetadata map[string]interface{}

// CheckMetadata contains metadata provided by all integrations.
// Each check has an entry in the top level map, each containing an array with
// all its instances, each containing its metadata.
type CheckMetadata map[string][]*CheckInstanceMetadata

// CheckInstanceMetadata contains metadata provided by an instance of an integration.
type CheckInstanceMetadata map[string]interface{}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname      string         `json:"hostname"`
	Timestamp     int64          `json:"timestamp"`
	CheckMetadata *CheckMetadata `json:"check_metadata"`
	AgentMetadata *AgentMetadata `json:"agent_metadata"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// Marshal not implemented
func (p *Payload) Marshal() ([]byte, error) {
	return nil, fmt.Errorf("V5 Payload serialization is not implemented")
}

// SplitPayload breaks the payload into times number of pieces
func (p *Payload) SplitPayload(times int) ([]marshaler.Marshaler, error) {
	return nil, fmt.Errorf("Inventories Payload splitting is not implemented")
}

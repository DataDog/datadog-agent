// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	HostMetadata  *HostMetadata  `json:"host_metadata"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
//
// In this case, the payload can only be split at the top level, so `times` is ignored
// and each top-level component is returned as a distinct payload.
func (p *Payload) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	newPayloads := []marshaler.AbstractMarshaler{}
	fieldName := ""

	// Each field can be sent individually but we can't split any more than this as the backend expects each payload
	// to be received complete.

	if p.CheckMetadata != nil {
		fieldName = "check_metadata"
		newPayloads = append(newPayloads,
			&Payload{
				Hostname:      p.Hostname,
				Timestamp:     p.Timestamp,
				CheckMetadata: p.CheckMetadata,
			})
	}
	if p.AgentMetadata != nil {
		fieldName = "agent_metadata"
		newPayloads = append(newPayloads,
			&Payload{
				Hostname:      p.Hostname,
				Timestamp:     p.Timestamp,
				AgentMetadata: p.AgentMetadata,
			})
	}
	if p.HostMetadata != nil {
		fieldName = "host_metadata"
		newPayloads = append(newPayloads,
			&Payload{
				Hostname:     p.Hostname,
				Timestamp:    p.Timestamp,
				HostMetadata: p.HostMetadata,
			})
	}

	// if only one field is set we can't split any more
	if len(newPayloads) <= 1 {
		return nil, fmt.Errorf("could not split inventories payload any more, %s metadata is too big for intake", fieldName)
	}

	return newPayloads, nil
}

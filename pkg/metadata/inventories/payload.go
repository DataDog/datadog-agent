// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package inventories

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
	Timestamp     int64          `json:"timestamp"`
	CheckMetadata *CheckMetadata `json:"check_metadata"`
	AgentMetadata *AgentMetadata `json:"agent_metadata"`
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentmetadata builds the common `agent_metadata` fields and the
// inventory wire payload.
package agentmetadata

import "encoding/json"

type agentMetadata map[string]interface{}

// RawFile is a scrubbed configuration file and the hash of its scrubbed content.
type RawFile struct {
	RawConfig string `json:"raw_config"`
	Hash      string `json:"hash"`
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname      string             `json:"hostname"`
	Timestamp     int64              `json:"timestamp"`
	Metadata      agentMetadata      `json:"agent_metadata"`
	FilesMetadata map[string]RawFile `json:"files_metadata,omitempty"`
	UUID          string             `json:"uuid"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

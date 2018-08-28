// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package agentchecks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"

	json "github.com/json-iterator/go"
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	CommonPayload
	MetaPayload
	ACPayload
	ExternalHostPayload
}

// MetaPayload wraps Meta from the host package (this is cached)
type MetaPayload struct {
	host.Meta `json:"meta"`
}

// CommonPayload wraps Payload from the common package
type CommonPayload struct {
	common.Payload
}

// ACPayload wraps the Agent Checks payload
type ACPayload struct {
	AgentChecks []interface{} `json:"agent_checks"`
}

// ExternalHostPayload wraps Payload from the `externalhost` package
type ExternalHostPayload struct {
	externalhost.Payload `json:"external_host_tags"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing
	type PayloadAlias Payload

	return json.Marshal((*PayloadAlias)(p))
}

// Marshal not implemented
func (p *Payload) Marshal() ([]byte, error) {
	return nil, fmt.Errorf("V5 Payload serialization is not implemented")
}

// SplitPayload breaks the payload into times number of pieces
func (p *Payload) SplitPayload(times int) ([]marshaler.Marshaler, error) {
	return nil, fmt.Errorf("AgentChecks Payload splitting is not implemented")
}

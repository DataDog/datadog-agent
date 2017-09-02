// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package agentchecks

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	CommonPayload
	HostPayload
	AgentChecksPayload
}

type HostPayload struct {
	host.Payload
}

type CommonPayload struct {
	common.Payload
}

type AgentChecksPayload struct {
	AgentChecks []interface{} `json:"agent_checks"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinit recursion while serializing
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentchecks

import (
	"encoding/json"
	"fmt"

	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/utils"
	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
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
	hostMetadataUtils.Meta `json:"meta"`
}

// CommonPayload wraps Payload from the common package
type CommonPayload struct {
	hostMetadataUtils.CommonPayload
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

// SplitPayload breaks the payload into times number of pieces
func (p *Payload) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("AgentChecks Payload splitting is not implemented")
}

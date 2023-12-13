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
	return nil, fmt.Errorf("could not split inventories payload any more, payload is too big for intake")
}

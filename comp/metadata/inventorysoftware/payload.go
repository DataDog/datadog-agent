// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package inventorysoftware

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// SoftwareMetadata is the metadata for a software product
type SoftwareMetadata struct {
	ProductCode string            `json:"product_code"`
	Metadata    map[string]string `json:"metadata"`
}

// Payload is the payload for the inventory software component
type Payload struct {
	Metadata []*SoftwareMetadata `json:"software_inventory_metadata"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
// In this case, the payload can't be split any further.
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories software payload any more, payload is too big for intake")
}

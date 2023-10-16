// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows || darwin

package host

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/metadata/host/utils"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/utils"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/gohai"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	hostMetadataUtils.CommonPayload
	hostMetadataUtils.Payload

	ResourcesPayload interface{} `json:"resources,omitempty"`
	GohaiPayload     string      `json:"gohai"`
}

// SplitPayload breaks the payload into times number of pieces
func (p *Payload) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	// Metadata payloads are analyzed as a whole, so they cannot be split
	return nil, fmt.Errorf("V5 Payload splitting is not implemented")
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing
	type PayloadAlias Payload

	return json.Marshal((*PayloadAlias)(p))
}

// getPayload returns the complete metadata payload as seen in Agent v5
func (h *host) getPayload(ctx context.Context) *Payload {
	p := &Payload{
		CommonPayload: *utils.GetCommonPayload(h.hostname, h.config),
		Payload:       *hostMetadataUtils.GetPayload(ctx, h.config),
	}

	if r := h.resources.Get(); r != nil {
		p.ResourcesPayload = r["resources"]
	}

	if h.config.GetBool("enable_gohai") {
		gohaiPayload, err := gohai.GetPayloadAsString(pkgconfig.IsContainerized())
		if err != nil {
			h.log.Errorf("Could not serialize gohai payload: %s", err)
		} else {
			p.GohaiPayload = gohaiPayload
		}
	}
	return p
}

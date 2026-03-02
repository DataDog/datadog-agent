// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package hostimpl

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	utils.CommonPayload
	utils.Payload
}

// MarshalJSON serializes a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// getPayload returns the complete metadata payload as seen in Agent v5. Note: gohai can't be used on AIX.
func (h *host) getPayload(ctx context.Context) *Payload {
	return &Payload{
		CommonPayload: *utils.GetCommonPayload(h.hostname, h.config),
		Payload:       *utils.GetPayload(ctx, h.config, h.hostnameComp),
	}
}

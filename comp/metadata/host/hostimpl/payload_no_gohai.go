// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build freebsd || netbsd || openbsd || solaris || dragonfly

package hostimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	utils.CommonPayload
	utils.Payload
	// Notice: ResourcesPayload requires gohai so it can't be included
	// TODO: gohai alternative (or fix gohai)
}

// SplitPayload breaks the payload into times number of pieces
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	// Metadata payloads are analyzed as a whole, so they cannot be split
	return nil, fmt.Errorf("host Payload splitting is not implemented")
}

// getPayload returns the complete metadata payload as seen in Agent v5. Note: gohai can't be used on the platforms
// this module builds for
func (h *host) getPayload(hostname string) *Payload {
	return &Payload{
		CommonPayload: *utils.GetCommonPayload(h.hostname, h.config),
		Payload:       *utils.GetPayload(ctx, h.config),
	}
}

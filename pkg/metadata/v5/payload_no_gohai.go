// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build freebsd || netbsd || openbsd || solaris || dragonfly
// +build freebsd netbsd openbsd solaris dragonfly

package v5

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	CommonPayload
	HostPayload
	// Notice: ResourcesPayload requires gohai so it can't be included
	// TODO: gohai alternative (or fix gohai)
}

// SplitPayload breaks the payload into times number of pieces
func (p *Payload) SplitPayload(times int) ([]marshaler.AbstractMarshaler, error) {
	// Metadata payloads are analyzed as a whole, so they cannot be split
	return nil, fmt.Errorf("V5 Payload splitting is not implemented")
}

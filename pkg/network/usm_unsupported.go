// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !((linux && linux_bpf) || (windows && npm))

package network

import (
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
)

// USMProtocolsData is a placeholder for unsupported platforms.
type USMProtocolsData struct{}

// NewUSMProtocolsData creates a new instance of USMProtocolsData with initialized maps.
// This is a no-op for unsupported platforms.
func NewUSMProtocolsData() USMProtocolsData {
	return USMProtocolsData{}
}

// Reset clears the maps in USMProtocolsData.
// This is a no-op for unsupported platforms.
func (*USMProtocolsData) Reset() {}

// processUSMDelta processes the USM delta for unsupported platforms.
// This is a no-op for unsupported platforms.
func (*networkState) processUSMDelta(map[protocols.ProtocolType]interface{}) {}

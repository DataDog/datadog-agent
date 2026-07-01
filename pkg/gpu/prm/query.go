// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package prm

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// Device is the minimal GPU device interface needed for PRM queries.
type Device interface {
	GetArchitecture() (nvml.DeviceArchitecture, error)
	//nolint:revive // Maintaining consistency with go-nvml API naming
	ReadWritePRM_v1(buffer *nvml.PRMTLV_v1) error
}

// QueryPortCounters issues a raw PRM query for a device/port/group and returns the decoded counters.
func QueryPortCounters(device Device, group int, port int) (map[string]uint64, error) {
	tlvBytes := PackPPCNTTLV(uint32(group), uint32(port))
	var prm nvml.PRMTLV_v1
	if len(tlvBytes) > len(prm.InData) {
		return nil, fmt.Errorf("PPCNT TLV payload too large: %d", len(tlvBytes))
	}

	prm.DataSize = uint32(len(tlvBytes))
	copy(prm.InData[:], tlvBytes)

	if err := device.ReadWritePRM_v1(&prm); err != nil {
		return nil, fmt.Errorf("issue raw PRM query: %w", err)
	}

	return UnpackTLV(prm.InData[:])
}

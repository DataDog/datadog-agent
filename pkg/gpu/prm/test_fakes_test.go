// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package prm

import (
	"errors"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

var errDeviceNotFound = errors.New("device not found")

type testDevice struct {
	arch         nvml.DeviceArchitecture
	readWritePRM func(buffer *nvml.PRMTLV_v1) error
}

func (d *testDevice) GetArchitecture() (nvml.DeviceArchitecture, error) {
	return d.arch, nil
}

//nolint:revive // Maintaining consistency with go-nvml API naming
func (d *testDevice) ReadWritePRM_v1(buffer *nvml.PRMTLV_v1) error {
	if d.readWritePRM == nil {
		return nvml.ERROR_NOT_SUPPORTED
	}
	return d.readWritePRM(buffer)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package testutil holds different utilities and stubs for testing
package testutil

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

// DefaultGpuCores is the default number of cores for a GPU device in the mock.
const DefaultGpuCores = 10

// GetBasicNvmlMock returns a mock of the nvml.Interface with a single device with 10 cores,
// useful for basic tests that need only the basic interaction with NVML to be working.
func GetBasicNvmlMock() *nvmlmock.Interface {
	return &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return 1, nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(int) (nvml.Device, nvml.Return) {
			return &nvmlmock.Device{
				GetNumGpuCoresFunc: func() (int, nvml.Return) {
					return DefaultGpuCores, nvml.SUCCESS
				},
				GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
					return 7, 5, nvml.SUCCESS
				},
			}, nvml.SUCCESS
		},
		DeviceGetCudaComputeCapabilityFunc: func(nvml.Device) (int, int, nvml.Return) {
			return 7, 5, nvml.SUCCESS
		},
	}
}

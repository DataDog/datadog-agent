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

// GPUUUIDs is a list of UUIDs for the devices returned by the mock
var GPUUUIDs = []string{
	"GPU-12345678-1234-1234-1234-123456789012",
	"GPU-99999999-1234-1234-1234-123456789013",
	"GPU-00000000-1234-1234-1234-123456789014",
}

// DefaultGpuUUID is the UUID for the default device returned by the mock
var DefaultGpuUUID = GPUUUIDs[0]

// GetDeviceMock returns a mock of the nvml.Device with the given UUID.
func GetDeviceMock(uuid string) *nvmlmock.Device {
	return &nvmlmock.Device{
		GetNumGpuCoresFunc: func() (int, nvml.Return) {
			return DefaultGpuCores, nvml.SUCCESS
		},
		GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
			return 7, 5, nvml.SUCCESS
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}
}

// GetBasicNvmlMock returns a mock of the nvml.Interface with a single device with 10 cores,
// useful for basic tests that need only the basic interaction with NVML to be working.
func GetBasicNvmlMock() *nvmlmock.Interface {
	return &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return len(GPUUUIDs), nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			return GetDeviceMock(GPUUUIDs[index]), nvml.SUCCESS
		},
		DeviceGetCudaComputeCapabilityFunc: func(nvml.Device) (int, int, nvml.Return) {
			return 7, 5, nvml.SUCCESS
		},
	}
}

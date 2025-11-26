// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// TestNVMLDeviceEnumeration tests that NVML can enumerate GPU devices on the current system.
// This validates the check's ability to discover and interact with GPUs.
func TestNVMLDeviceEnumeration(t *testing.T) {
	testutil.RequireGPU(t)

	lib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err, "NVML library should be available")
	require.NotNil(t, lib, "NVML library should not be nil")

	deviceCount, err := lib.DeviceGetCount()
	require.NoError(t, err, "Should be able to get device count")
	require.Greater(t, deviceCount, 0, "Should have at least one GPU")

	for i := 0; i < deviceCount; i++ {
		device, err := lib.DeviceGetHandleByIndex(i)
		require.NoError(t, err, "Should be able to get device handle for index %d", i)
		require.NotNil(t, device, "Device handle should not be nil")

		name, err := device.GetName()
		require.NoError(t, err, "Should be able to get device name")
		t.Logf("GPU %d: %s", i, name)

		uuid, err := device.GetUUID()
		require.NoError(t, err, "Should be able to get device UUID")
		t.Logf("GPU %d UUID: %s", i, uuid)
	}
}

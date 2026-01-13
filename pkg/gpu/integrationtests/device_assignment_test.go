// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package integrationtests

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/gpu"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// requireDocker skips the test if Docker is not available
func requireDocker(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker not available, skipping test")
	}
}

// addContainerToWorkloadmeta adds a Docker container to the workloadmeta store.
// This populates the store with the real container data that was discovered.
func addContainerToWorkloadmeta(t *testing.T, wmeta workloadmetamock.Mock, containerID string, pid int) {
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: containerID,
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
		PID:     pid,
	}
	wmeta.Set(container)

	// Verify the container was added
	storedContainer, err := wmeta.GetContainer(containerID)
	require.NoError(t, err, "container should be found in the store")
	require.NotNil(t, storedContainer, "container should not be nil")
}

// deviceAssignmentTestCase defines a test case for device assignment
type deviceAssignmentTestCase struct {
	name                  string
	cudaVisibleDevices    string // Value for CUDA_VISIBLE_DEVICES env var (empty = not set)
	selectedDeviceIndex   int32  // Device index to select via SetDeviceSelection
	expectedPhysicalIndex int    // Expected physical device index in the result
	requireMultiGPU       bool   // Skip if less than 2 GPUs available
}

// TestDeviceAssignment tests the GPU device assignment logic in SystemContext
// for both bare metal processes and Docker containers, with and without
// CUDA_VISIBLE_DEVICES environment variable.
//
// All tests use real processes and real procfs - no fake/mock process data.
// The workloadmeta store is populated with real container data for Docker tests.
func TestDeviceAssignment(t *testing.T) {
	testutil.RequireGPU(t)

	// Initialize real NVML and get device information
	lib := initNVML(t)
	deviceCount, err := lib.DeviceGetCount()
	require.NoError(t, err)
	require.Greater(t, deviceCount, 0, "Need at least one GPU for this test")

	// Get all device UUIDs for verification
	deviceUUIDs := make([]string, deviceCount)
	for i := 0; i < deviceCount; i++ {
		device, err := lib.DeviceGetHandleByIndex(i)
		require.NoError(t, err)
		uuid, err := device.GetUUID()
		require.NoError(t, err)
		deviceUUIDs[i] = uuid
		t.Logf("Device %d: %s", i, uuid)
	}

	// Common test cases for both bare metal and Docker
	testCases := []deviceAssignmentTestCase{
		{
			name:                  "NoEnvVar_SelectDevice0",
			cudaVisibleDevices:    "",
			selectedDeviceIndex:   0,
			expectedPhysicalIndex: 0,
			requireMultiGPU:       false,
		},
		{
			name:                  "NoEnvVar_SelectDevice1",
			cudaVisibleDevices:    "",
			selectedDeviceIndex:   1,
			expectedPhysicalIndex: 1,
			requireMultiGPU:       true,
		},
		{
			name:                  "WithCudaVisibleDevices_1_0_SelectIndex0",
			cudaVisibleDevices:    "1,0",
			selectedDeviceIndex:   0,
			expectedPhysicalIndex: 1, // CUDA_VISIBLE_DEVICES=1,0 means index 0 -> device 1
			requireMultiGPU:       true,
		},
		{
			name:                  "WithCudaVisibleDevices_1_0_SelectIndex1",
			cudaVisibleDevices:    "1,0",
			selectedDeviceIndex:   1,
			expectedPhysicalIndex: 0, // CUDA_VISIBLE_DEVICES=1,0 means index 1 -> device 0
			requireMultiGPU:       true,
		},
	}

	// BareMetal tests: Run sample binary directly on the host
	// Uses real process with real procfs
	t.Run("BareMetal", func(t *testing.T) {
		for _, tc := range testCases {
			tc := tc // capture range variable
			t.Run(tc.name, func(t *testing.T) {
				if tc.requireMultiGPU && deviceCount < 2 {
					t.Skip("Need at least 2 GPUs for this test")
				}

				// Run the sample binary directly on the host with the specified env var
				args := &testutil.GPUUUIDsSampleArgs{
					CudaVisibleDevicesEnv: tc.cudaVisibleDevices,
				}
				out := testutil.RunSampleWithArgs(t, testutil.GPUUUIDsSample, args)
				cmd := out.Command
				require.NotNil(t, cmd)

				pid := out.PID
				t.Logf("Sample started: PID=%d, CUDA_VISIBLE_DEVICES=%q", pid, tc.cudaVisibleDevices)

				// Use real procfs and workloadmeta (no container)
				wmeta := testutil.GetWorkloadMetaMock(t)
				sysCtx, err := gpu.GetSystemContext(gpu.WithProcRoot(kernel.ProcFSRoot()), gpu.WithWorkloadMeta(wmeta), gpu.WithTelemetry(testutil.GetTelemetryMock(t)))
				require.NoError(t, err)

				sysCtx.SetDeviceSelection(pid, pid, tc.selectedDeviceIndex)

				// Empty container ID for bare metal
				device, err := sysCtx.GetCurrentActiveGpuDevice(pid, pid, func() string { return "" })
				require.NoError(t, err)
				require.Equal(t, deviceUUIDs[tc.expectedPhysicalIndex], device.GetDeviceInfo().UUID)
			})
		}
	})

	// DockerContainer tests: Run sample binary inside a Docker container
	// Uses real container process with real procfs, workloadmeta populated with real container data
	t.Run("DockerContainer", func(t *testing.T) {
		requireDocker(t)

		for _, tc := range testCases {
			tc := tc // capture range variable
			t.Run(tc.name, func(t *testing.T) {
				if tc.requireMultiGPU && deviceCount < 2 {
					t.Skip("Need at least 2 GPUs for this test")
				}

				// Run sample in Docker with the specified CUDA_VISIBLE_DEVICES
				args := &testutil.GPUUUIDsSampleArgs{
					CudaVisibleDevicesEnv: tc.cudaVisibleDevices,
				}
				out := testutil.RunSampleInDockerWithArgs(t, testutil.GPUUUIDsSample, testutil.MinimalDockerImage, args)
				pid := out.PID
				containerID := out.ContainerID
				require.NotZero(t, pid, "Container PID should not be zero")
				require.NotEmpty(t, containerID, "Container ID should not be empty")

				t.Logf("Container started: PID=%d, ID=%s, CUDA_VISIBLE_DEVICES=%q", pid, containerID, tc.cudaVisibleDevices)

				// Give the container process a moment to fully initialize
				time.Sleep(100 * time.Millisecond)

				// Use real procfs, populate workloadmeta with real container data
				wmeta := testutil.GetWorkloadMetaMock(t)
				addContainerToWorkloadmeta(t, wmeta, containerID, pid)

				sysCtx, err := gpu.GetSystemContext(gpu.WithProcRoot(kernel.ProcFSRoot()), gpu.WithWorkloadMeta(wmeta), gpu.WithTelemetry(testutil.GetTelemetryMock(t)))
				require.NoError(t, err)

				sysCtx.SetDeviceSelection(pid, pid, tc.selectedDeviceIndex)

				device, err := sysCtx.GetCurrentActiveGpuDevice(pid, pid, func() string { return containerID })
				require.NoError(t, err)
				require.Equal(t, deviceUUIDs[tc.expectedPhysicalIndex], device.GetDeviceInfo().UUID)
			})
		}
	})
}

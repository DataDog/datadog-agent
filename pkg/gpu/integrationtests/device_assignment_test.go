// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml && docker

package integrationtests

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	workloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetadefaults "github.com/DataDog/datadog-agent/comp/core/workloadmeta/defaults"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/gpu"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	cudaTestsDockerImage = "ubuntu:24.04" // use Ubuntu images instead of Alpine for CUDA tests, to avoid issues with libc
)

// requireDocker skips the test if Docker is not available
func requireDocker(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("Docker not available, skipping test: %v", err)
	}
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
				pid := out.PID
				t.Logf("Sample started: PID=%d, CUDA_VISIBLE_DEVICES=%q", pid, tc.cudaVisibleDevices)

				// Use real procfs and workloadmeta (no container)
				wmeta := testutil.GetWorkloadMetaMock(t)
				sysCtx := gpu.GetTestSystemContext(t, wmeta, testutil.GetTelemetryMock(t))
				sysCtx.SetDeviceSelection(pid, pid, tc.selectedDeviceIndex)

				// Empty container ID for bare metal
				device, err := sysCtx.GetCurrentActiveGpuDevice(pid, pid, func() string { return "" })
				require.NoError(t, err)
				require.Equal(t, deviceUUIDs[tc.expectedPhysicalIndex], device.GetDeviceInfo().UUID)
			})
		}
	})

	// DockerContainer tests: Run sample binary inside a Docker container
	// Uses real container process with real procfs and real workloadmeta docker collector
	t.Run("DockerContainer", func(t *testing.T) {
		requireDocker(t)
		env.SetFeatures(t, env.Docker)

		type workloadmetaDeps struct {
			fx.In
			Wmeta workloadmeta.Component
		}

		wmeta := fxutil.Test[workloadmetaDeps](t, fx.Options(
			fx.Supply(core.BundleParams{
				ConfigParams:         config.NewParams("", config.WithIgnoreErrors(true)),
				SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
				LogParams:            log.ForOneShot("gpu-device-assignment-test", "off", true),
			}),
			core.Bundle(),
			secretsnoopfx.Module(),
			workloadfilterfx.Module(),
			wmcatalog.GetCatalog(),
			workloadmetafx.Module(workloadmetadefaults.DefaultParams()),
		)).Wmeta

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
				out := testutil.RunSampleInDockerWithArgs(t, testutil.GPUUUIDsSample, cudaTestsDockerImage, args)
				pid := out.PID
				containerID := out.ContainerID
				require.NotZero(t, pid, "Container PID should not be zero")
				require.NotEmpty(t, containerID, "Container ID should not be empty")

				t.Logf("Container started: PID=%d, ID=%s, CUDA_VISIBLE_DEVICES=%q", pid, containerID, tc.cudaVisibleDevices)

				// Give the container process a moment to fully initialize
				time.Sleep(100 * time.Millisecond)

				require.Eventually(t, func() bool {
					container, err := wmeta.GetContainer(containerID)
					return err == nil && container != nil
				}, 30*time.Second, 500*time.Millisecond, "container %s not found in workloadmeta", containerID)

				sysCtx := gpu.GetTestSystemContext(t, wmeta, testutil.GetTelemetryMock(t))
				sysCtx.SetDeviceSelection(pid, pid, tc.selectedDeviceIndex)

				device, err := sysCtx.GetCurrentActiveGpuDevice(pid, pid, func() string { return containerID })
				require.NoError(t, err)
				require.Equal(t, deviceUUIDs[tc.expectedPhysicalIndex], device.GetDeviceInfo().UUID)
			})
		}
	})
}

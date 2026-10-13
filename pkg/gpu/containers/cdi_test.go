// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package containers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
)

func setCDISpecDirs(t *testing.T, dirs ...string) {
	original := CDISpecDirs
	CDISpecDirs = dirs
	t.Cleanup(func() { CDISpecDirs = original })
}

func TestCheckNvidiaDRACDISpecs(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	empty := t.TempDir()
	otherVendor := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(otherVendor, "k8s.example.com-claim_uid.yaml"), []byte("{}"), 0o644))
	withSpec := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(withSpec, NvidiaDRAClaimSpecFileName("uid")), []byte("{}"), 0o644))

	t.Run("dir with NVIDIA DRA claim specs", func(t *testing.T) {
		setCDISpecDirs(t, missing, withSpec)
		assert.NoError(t, CheckNvidiaDRACDISpecs())
	})

	t.Run("no dir with NVIDIA DRA claim specs", func(t *testing.T) {
		setCDISpecDirs(t, missing, empty, otherVendor)
		err := CheckNvidiaDRACDISpecs()
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrNotExist)
		assert.Contains(t, err.Error(), missing)
		assert.Contains(t, err.Error(), empty)
		assert.Contains(t, err.Error(), otherVendor)
	})

	t.Run("NVIDIA DRA claim specs are not readable", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("file permissions are not enforced for root")
		}
		unreadable := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(unreadable, NvidiaDRAClaimSpecFileName("uid")), []byte("{}"), 0o000))
		setCDISpecDirs(t, unreadable)
		err := CheckNvidiaDRACDISpecs()
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrPermission)
	})
}

func TestHasUnresolvedDRAGPUClaim(t *testing.T) {
	draResource := workloadmeta.ContainerAllocatedResource{
		Name:       string(gpuutil.GpuNvidiaDRA),
		CdiDevices: []string{"k8s.gpu.nvidia.com/claim=uid-gpu-0"},
	}
	running := workloadmeta.ContainerState{Running: true}

	tests := []struct {
		name      string
		container *workloadmeta.Container
		expected  bool
	}{
		{
			name: "unresolved DRA claim",
			container: &workloadmeta.Container{
				State:                      running,
				ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{draResource},
			},
			expected: true,
		},
		{
			name:      "stopped container",
			container: &workloadmeta.Container{ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{draResource}},
		},
		{
			name: "resolved DRA claim",
			container: &workloadmeta.Container{
				State:                      running,
				ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{draResource},
				GPUDeviceIDs:               []string{"GPU-0"},
			},
		},
		{
			name: "DRA claim without CDI devices",
			container: &workloadmeta.Container{
				State:                      running,
				ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{{Name: string(gpuutil.GpuNvidiaDRA)}},
			},
		},
		{
			name: "device plugin resource",
			container: &workloadmeta.Container{
				State:                      running,
				ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{{Name: string(gpuutil.GpuNvidiaGeneric), ID: "GPU-0"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, HasUnresolvedDRAGPUClaim(tt.container))
		})
	}
}

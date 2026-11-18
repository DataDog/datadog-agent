// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package gpu

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// On a Cloud TPU host, which the TPU check collects, the NVIDIA GPU check
// neither fails nor loads NVML, whether TPU collection is enabled or not.
func TestNVIDIACheckOnTPUHost(t *testing.T) {
	for _, tpuEnabled := range []bool{true, false} {
		nvmlLoads := 0
		ddnvml.WithMockNvmlNewFunc(t, func(...nvml.LibraryOption) nvml.Interface {
			nvmlLoads++
			return testutil.NewMockNVML(testutil.WithInitReturn(nvml.ERROR_LIBRARY_NOT_FOUND))
		})
		root := t.TempDir()
		dir := filepath.Join(root, "bus", "pci", "devices", "0000:00:04.0")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor"), []byte("0x1ae0\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "device"), []byte("0x006f\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "class"), []byte("0x120000\n"), 0o644))

		check := setupNVIDIACheckOnHost(t, root, map[string]any{"gpu.tpu.enabled": tpuEnabled})
		for range 3 {
			require.NoError(t, check.Run())
		}
		assert.Zero(t, nvmlLoads, "gpu.tpu.enabled=%v", tpuEnabled)
	}
}

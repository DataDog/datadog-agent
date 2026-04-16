// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package spec

import (
	"fmt"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestLoadSpecNotEmpty(t *testing.T) {
	specs, err := LoadSpecs()
	require.NoError(t, err)
	
	require.NotEmpty(t, specs.Metrics.MetricPrefix, "metric_prefix should not be empty")
	require.NotEmpty(t, specs.Metrics.Metrics, "metrics should not be empty")
	require.NotEmpty(t, specs.Tags.Tags, "tags should not be empty")
	require.NotEmpty(t, specs.Tags.Tagsets, "tagsets should not be empty")
	for name := range specs.Metrics.Metrics {
		require.NotEmpty(t, name, "metric name should not be empty")
	}
	for tagsetName, tagsetSpec := range tagsSpec.Tagsets {
		for _, tagName := range tagsetSpec.Tags {
			_, ok := tagsSpec.Tags[tagName]
			require.Truef(t, ok, "tagset %s references unknown tag %s", tagsetName, tagName)
		}
	}

	for metricName, metricSpec := range metricsSpec.Metrics {
		for deviceMode := range metricSpec.Support.DeviceModes {
			require.Containsf(t, []string{"physical", "mig", "vgpu"}, string(deviceMode), "metric %s has invalid device mode key %q", metricName, deviceMode)
		}
	}
}

func TestLoadArchitecturesNotEmpty(t *testing.T) {
	archSpecFile, err := LoadArchitecturesSpec()
	require.NoError(t, err)

	require.NotEmpty(t, archSpecFile.Architectures, "architectures should not be empty")
	for name, archSpec := range archSpecFile.Architectures {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, archSpec.UnsupportedDeviceModes, "unsupported_device_modes should be present")
		})
	}
}

func TestLoadTagsSpecNotEmpty(t *testing.T) {
	tagsSpec, err := LoadTagsSpec()
	require.NoError(t, err)

	require.NotEmpty(t, tagsSpec.Tags, "tags should not be empty")
	require.NotEmpty(t, tagsSpec.Tagsets, "tagsets should not be empty")
	for tagsetName, tagsetSpec := range tagsSpec.Tagsets {
		for _, tagName := range tagsetSpec.Tags {
			_, ok := tagsSpec.Tags[tagName]
			require.Truef(t, ok, "tagset %s references unknown tag %s", tagsetName, tagName)
		}
	}
}

func TestTagSpecUnmarshalYAML(t *testing.T) {
	t.Run("compiles regex", func(t *testing.T) {
		var spec TagSpec

		err := yaml.Unmarshal([]byte(`regex: "^foo$"`), &spec)

		require.NoError(t, err)
		require.NotNil(t, spec.Regex)
		require.True(t, spec.Regex.MatchString("foo"))
		require.False(t, spec.Regex.MatchString("bar"))
	})

	t.Run("rejects invalid regex", func(t *testing.T) {
		var spec TagSpec

		err := yaml.Unmarshal([]byte(`regex: "["`), &spec)

		require.Error(t, err)
		require.ErrorContains(t, err, `compile tag regex "["`)
	})
}

// TestMockCapabilitiesMatchArchitectureSpec ensures that for each architecture and supported device mode,
// the NVML mock configured from architectures.yaml returns API behavior that matches the capability flags
// (gpm, unsupported_fields_by_device_mode). This validates that the mock actually applies the spec.
func TestMockCapabilitiesMatchArchitectureSpec(t *testing.T) {
	specs, err := LoadSpecs()
	require.NoError(t, err)

	configs := KnownGPUConfigs(specs)
	for _, config := range configs {
		subtestName := fmt.Sprintf("arch=%s/mode=%s", config.Architecture, config.DeviceMode)
		t.Run(subtestName, func(t *testing.T) {
			archSpec := specs.Architectures.Architectures[config.Architecture]
			opts := BuildMockOptionsForConfig(t, config, archSpec)

			ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(opts...))

			lib, err := ddnvml.GetSafeNvmlLib()
			require.NoError(t, err, "should be able to get NVML lib")
			dev, err := lib.DeviceGetHandleByIndex(0)
			require.NoError(t, err, "should be able to get device 0")

			// gpm -> GpmQueryDeviceSupport(): IsSupportedDevice 1 when enabled, 0 when disabled
			support, err := dev.GpmQueryDeviceSupport()
			require.NoError(t, err, "GpmQueryDeviceSupport should not report an error")
			expected := uint32(0)
			if archSpec.Capabilities.GPM {
				expected = 1
			}
			if config.DeviceMode == DeviceModeVGPU {
				// Mocks model vGPU as not supporting GPM collection even on architectures
				// where physical devices support GPM.
				expected = 0
			}
			assert.Equal(t, expected, support.IsSupportedDevice, "GpmQueryDeviceSupport.IsSupportedDevice should be %d when gpm=%v", expected, archSpec.Capabilities.GPM)

			// Check also that GpmSampleGet returns NOT_SUPPORTED when GPM is not supported
			var sample testutil.MockGpmSample
			err = dev.GpmSampleGet(sample)
			if archSpec.Capabilities.GPM && config.DeviceMode != DeviceModeVGPU {
				require.NoError(t, err, "GpmSampleGet should not return an error")
			} else {
				require.Error(t, err, "GpmSampleGet should return an error")
				require.True(t, ddnvml.IsUnsupported(err), "GpmSampleGet should return an API_UNSUPPORTED_ON_DEVICE error")
			}

			unsupportedIDs := UnsupportedFieldIDsForMode(t, archSpec, config.DeviceMode)
			unsupportedSet := make(map[uint32]struct{}, len(unsupportedIDs))
			for _, id := range unsupportedIDs {
				unsupportedSet[id] = struct{}{}
			}

			fieldValues := AllConfiguredNVMLFieldValues()
			err = dev.GetFieldValues(fieldValues)
			require.NoError(t, err, "GetFieldValues should not return an error")
			for _, fv := range fieldValues {
				_, isUnsupported := unsupportedSet[fv.FieldId]
				if isUnsupported {
					require.Equal(t, uint32(nvml.ERROR_NOT_SUPPORTED), fv.NvmlReturn, "field id %d should be unsupported", fv.FieldId)
				} else {
					require.Equal(t, uint32(nvml.SUCCESS), fv.NvmlReturn, "field id %d should be supported", fv.FieldId)
				}
			}
		})
	}
}

func TestBuildMockOptionsCreatesCorrectDevices(t *testing.T) {
	modes := []DeviceMode{DeviceModePhysical, DeviceModeMIG, DeviceModeVGPU}
	archSpec, err := LoadArchitecturesSpec()
	require.NoError(t, err)
	archName := "blackwell"
	require.Contains(t, archSpec.Architectures, archName)
	arch := archSpec.Architectures[archName]

	for _, mode := range modes {
		require.True(t, IsModeSupportedByArchitecture(arch, mode))

		t.Run(string(mode), func(t *testing.T) {
			config := GPUConfig{Architecture: archName, DeviceMode: mode}
			opts := BuildMockOptionsForConfig(t, config, arch)
			ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(opts...))

			lib, err := ddnvml.GetSafeNvmlLib()
			require.NoError(t, err, "should be able to get NVML lib")

			count, err := lib.DeviceGetCount()
			require.NoError(t, err, "should be able to get device count")
			assert.Equal(t, 1, count, "should be 1 device for physical and vgpu modes")

			for i := 0; i < count; i++ {
				dev, err := lib.DeviceGetHandleByIndex(i)
				require.NoError(t, err, "should be able to get device %d", i)

				isMig, err := dev.IsMigDeviceHandle()
				require.NoError(t, err, "should be able to check if device is a MIG device")
				assert.False(t, isMig, "top-level devices should not be MIG devices")

				virtMode, err := dev.GetVirtualizationMode()
				require.NoError(t, err, "should be able to get virtualization mode")

				migEnabled, _, err := dev.GetMigMode()
				require.NoError(t, err, "should be able to get MIG mode")

				if mode != DeviceModeMIG {
					assert.False(t, isMig, "device %d should not be a MIG device", i)
					if mode == DeviceModeVGPU {
						assert.Equal(t, nvml.GPU_VIRTUALIZATION_MODE_VGPU, virtMode, "virtualization mode should be vGPU for vgpu mode")
					} else {
						assert.Equal(t, nvml.GPU_VIRTUALIZATION_MODE_NONE, virtMode, "virtualization mode should be none for physical device")
					}
				} else {
					assert.Equal(t, nvml.DEVICE_MIG_ENABLE, migEnabled, "MIG mode should be enabled on the parent GPU")
					migChildrenCount, err := dev.GetMaxMigDeviceCount()
					require.NoError(t, err, "should be able to get MIG children count")
					assert.Equal(t, 1, migChildrenCount, "should have 1 MIG child for mig mode")

					for j := 0; j < migChildrenCount; j++ {
						migChild, err := dev.GetMigDeviceHandleByIndex(j)
						require.NoError(t, err, "should be able to get MIG child %d", j)

						migChildIsMig, err := migChild.IsMigDeviceHandle()
						require.NoError(t, err, "should be able to check if MIG child is a MIG device")
						assert.True(t, migChildIsMig, "MIG child %d should be a MIG device", j)

						migChildVirtMode, err := migChild.GetVirtualizationMode()
						require.NoError(t, err, "should be able to get virtualization mode for MIG child")
						assert.Equal(t, nvml.GPU_VIRTUALIZATION_MODE_NONE, migChildVirtMode, "virtualization mode should be none for MIG child")
					}
				}

			}
		})
	}
}

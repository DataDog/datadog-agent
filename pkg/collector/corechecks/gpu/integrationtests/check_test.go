// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// specDevices groups the host's GPUs by the device mode they are validated under.
type specDevices struct {
	// physical are the GPUs without MIG mode.
	physical []safenvml.Device
	// migParents are the GPUs in MIG mode. They usually have MIG devices, but may have none yet.
	migParents []safenvml.Device
	// migChildren are the MIG devices of migParents.
	migChildren []safenvml.Device
}

func discoverSpecDevices(t *testing.T) specDevices {
	t.Helper()
	lib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err)

	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))
	require.NoError(t, cache.Refresh())

	devices, err := cache.AllPhysicalDevices()
	require.NoError(t, err)
	require.NotEmpty(t, devices)

	var grouped specDevices
	for _, device := range devices {
		physicalDevice, ok := device.(*safenvml.PhysicalDevice)
		require.True(t, ok, "device %s should be a PhysicalDevice", device.GetDeviceInfo().UUID)

		if !physicalDevice.HasMIGFeatureEnabled {
			grouped.physical = append(grouped.physical, device)
			continue
		}
		// A GPU in MIG mode is validated as a MIG parent even if it has no MIG device: NVML supports the same
		// APIs on it either way.
		grouped.migParents = append(grouped.migParents, device)
		for _, child := range physicalDevice.MIGChildren {
			grouped.migChildren = append(grouped.migChildren, child)
		}
	}
	return grouped
}

// specScenario is a set of devices validated against the spec by running the check while idle and
// with a workload.
type specScenario struct {
	// devices are the devices whose metrics are validated.
	devices []safenvml.Device
	configs map[string]gpuspec.GPUConfig
	// smiOptions selects the devices compared with nvidia-smi; nil disables nvidia-smi validation.
	smiOptions map[string][]testutil.SmiCollectionOption
	// xidDevices get an XID event injected so the XID metrics are emitted.
	xidDevices []safenvml.Device
	// startWorkload starts a GPU workload on the devices and returns the calibrated values to validate
	// against, keyed by lowercase UUID; nil disables calibrated validation.
	startWorkload func(t *testing.T, lib safenvml.SafeNVML) map[string]map[string]*float64
	// configFeatures are the Agent configuration features enabled for the check, such as
	// system-probe with the GPU eBPF probes.
	configFeatures map[gpuspec.ConfigFeature]bool
}

func runSpecScenario(t *testing.T, specs *gpuspec.Specs, scenario specScenario) {
	uuids := make([]string, len(scenario.devices))
	var workloadUUIDs, workloadFreeUUIDs []string
	for i, device := range scenario.devices {
		uuid := device.GetDeviceInfo().UUID
		uuids[i] = uuid
		// The devices that never run a workload, such as MIG parents, are validated as idle while the
		// others run the workload.
		config := scenario.configs[strings.ToLower(uuid)]
		if config.CanRunWorkloads() {
			workloadUUIDs = append(workloadUUIDs, uuid)
		} else {
			workloadFreeUUIDs = append(workloadFreeUUIDs, uuid)
		}
	}

	t.Run("idle", func(t *testing.T) {
		metricsByUUID, smiSamples := collectCheckAndNvidiaSmiMetrics(t, checkCollectionOptions{
			passes:           1,
			interval:         time.Second,
			smiOptionsByUUID: scenario.smiOptions,
			injectXIDDevices: scenario.xidDevices,
		})
		validateCollectedMetrics(t, specs, uuids, scenario.configs, metricsByUUID, smiSamples, nil, gpuspec.ValidationOptions{
			ConfigFeatures: scenario.configFeatures,
		})
	})

	t.Run("with-workloads", func(t *testing.T) {
		// The idle check's cleanup shuts down the shared NVML singleton.
		lib, err := safenvml.GetSafeNvmlLib()
		require.NoError(t, err)

		calibratedValuesByUUID := scenario.startWorkload(t, lib)
		metricsByUUID, smiSamples := collectCheckAndNvidiaSmiMetrics(t, checkCollectionOptions{
			passes:           gpuBurnerCollectionPasses,
			interval:         gpuBurnerCollectionInterval,
			smiOptionsByUUID: scenario.smiOptions,
			injectXIDDevices: scenario.xidDevices,
		})
		validateCollectedMetrics(t, specs, workloadUUIDs, scenario.configs, metricsByUUID, smiSamples, calibratedValuesByUUID, gpuspec.ValidationOptions{
			WorkloadActive:  true,
			ConfigFeatures:  scenario.configFeatures,
			WorkloadTagsets: map[string]bool{"process": true},
		})
		validateCollectedMetrics(t, specs, workloadFreeUUIDs, scenario.configs, metricsByUUID, smiSamples, nil, gpuspec.ValidationOptions{
			ConfigFeatures: scenario.configFeatures,
		})
	})
}

// TestSpec validates the GPUs that are not in MIG mode.
func TestSpec(t *testing.T) {
	testutil.RequireGPU(t)
	testutil.RequireSmi(t)
	env.SetFeatures(t, env.KubernetesDevicePlugins, env.NVML)

	specs, err := gpuspec.LoadSpecs()
	require.NoError(t, err)
	devices := discoverSpecDevices(t).physical
	if len(devices) == 0 {
		t.Skip("all GPUs are in MIG mode, see TestSpecMIG")
	}
	configFeatures := startSystemProbeIfRequested(t)

	runSpecScenario(t, specs, specScenario{
		configFeatures: configFeatures,
		devices:        devices,
		configs:        deviceConfigs(t, specs, devices, gpuspec.DeviceModePhysical),
		smiOptions:     smiOptionsForPhysicalDevices(t, devices),
		xidDevices:     devices,
		startWorkload: func(t *testing.T, lib safenvml.SafeNVML) map[string]map[string]*float64 {
			indices := make([]int, len(devices))
			for i, device := range devices {
				indices[i] = device.GetDeviceInfo().Index
			}
			return startCalibratedWorkload(t, lib, indices, 100)
		},
	})
}

// TestSpecMIG validates GPUs in MIG mode: the parents against the mig_parent spec and their MIG devices
// against the mig spec. It runs on hosts that are already in MIG mode. With GPU_TEST_CONFIGURE_MIG set,
// it first puts the MIG-capable GPUs in MIG mode and creates MIG devices, and restores the GPUs afterwards.
func TestSpecMIG(t *testing.T) {
	testutil.RequireGPU(t)
	env.SetFeatures(t, env.KubernetesDevicePlugins, env.NVML)

	if configureMIGRequested(t) {
		configureMIG(t)
	}

	specs, err := gpuspec.LoadSpecs()
	require.NoError(t, err)
	grouped := discoverSpecDevices(t)
	if len(grouped.migParents) == 0 {
		t.Skipf("no GPU in MIG mode, enable it or set %s to let the test configure one", configureMIGEnv)
	}
	// Started after configureMIG, so system-probe sees the MIG devices and stops before the MIG
	// configuration is restored.
	configFeatures := startSystemProbeIfRequested(t)

	devices := slices.Concat(grouped.migParents, grouped.migChildren)
	configs := deviceConfigs(t, specs, grouped.migParents, gpuspec.DeviceModeMIGParent)
	maps.Copy(configs, deviceConfigs(t, specs, grouped.migChildren, gpuspec.DeviceModeMIG))

	runSpecScenario(t, specs, specScenario{
		configFeatures: configFeatures,
		devices:        devices,
		configs:        configs,
		// nvidia-smi dmon cannot sample MIG devices, and a parent in MIG mode reports no usable utilization.
		smiOptions: nil,
		// The XID metrics are not supported in MIG mode.
		xidDevices: nil,
		startWorkload: func(t *testing.T, _ safenvml.SafeNVML) map[string]map[string]*float64 {
			// A CUDA process sees a single MIG device, so run one gpu-burner per MIG device.
			for _, child := range grouped.migChildren {
				StartGPUBurner(t, child.GetDeviceInfo().UUID, 1, 100)
			}
			return nil
		},
	})
}

func linkCount(t *testing.T, device safenvml.Device, name string, countFunc func(safenvml.Device) (int, error)) int {
	t.Helper()

	count, err := countFunc(device)
	if err != nil {
		if safenvml.IsAPIUnsupportedOnDevice(err, device) {
			return 0
		}
		require.NoError(t, err, "%s link count probe failed for GPU %s", name, device.GetDeviceInfo().UUID)
		return 0
	}
	return count
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func physicalDevices(t *testing.T) []safenvml.Device {
	t.Helper()
	lib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err)

	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))
	require.NoError(t, cache.Refresh())

	devices, err := cache.AllPhysicalDevices()
	require.NoError(t, err)
	require.NotEmpty(t, devices)
	return devices
}

func TestSpec(t *testing.T) {
	testutil.RequireGPU(t)
	testutil.RequireSmi(t)
	env.SetFeatures(t, env.KubernetesDevicePlugins, env.NVML)

	specs, err := gpuspec.LoadSpecs()
	require.NoError(t, err)
	devices := physicalDevices(t)
	configsByUUID, smiOptionsByUUID := physicalDeviceConfigs(t, specs, devices)
	uuids := make([]string, len(devices))
	for i, device := range devices {
		uuids[i] = device.GetDeviceInfo().UUID
	}

	t.Run("idle", func(t *testing.T) {
		metricsByUUID, smiSamples := collectCheckAndNvidiaSmiMetrics(t, checkCollectionOptions{
			passes:           1,
			interval:         time.Second,
			smiOptionsByUUID: smiOptionsByUUID,
			injectXIDDevices: devices,
		})
		validateCollectedMetrics(t, specs, uuids, configsByUUID, metricsByUUID, smiSamples, nil, gpuspec.ValidationOptions{
			// This test does not configure system-probe.
			ConfigFeatures: nil,
		})
	})

	t.Run("with-workloads", func(t *testing.T) {
		// The idle check's cleanup shuts down the shared NVML singleton.
		lib, err := safenvml.GetSafeNvmlLib()
		require.NoError(t, err)

		indices := make([]int, len(devices))
		for index := range devices {
			indices[index] = index
		}
		calibratedValuesByUUID := startCalibratedWorkload(t, lib, indices, 100)
		metricsByUUID, smiSamples := collectCheckAndNvidiaSmiMetrics(t, checkCollectionOptions{
			passes:           gpuBurnerCollectionPasses,
			interval:         gpuBurnerCollectionInterval,
			smiOptionsByUUID: smiOptionsByUUID,
			injectXIDDevices: devices,
		})
		validateCollectedMetrics(t, specs, uuids, configsByUUID, metricsByUUID, smiSamples, calibratedValuesByUUID, gpuspec.ValidationOptions{
			WorkloadActive:  true,
			ConfigFeatures:  nil,
			WorkloadTagsets: map[string]bool{"process": true},
		})
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

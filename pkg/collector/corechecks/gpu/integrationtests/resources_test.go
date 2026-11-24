// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	gpuresources "github.com/DataDog/datadog-agent/pkg/gpu/resources"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/genresources"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

const resourcesTestTTL = 30 * time.Minute

type publishedResource struct {
	event  *genresources.GenericResourceEvent
	device gpuresources.Device
}

// TestGPUResources runs the real check with resource publication enabled and
// checks each published gpu_device against the devices NVML reports.
func TestGPUResources(t *testing.T) {
	testutil.RequireGPU(t)

	// The Agent skips vGPU guest devices, so they are not expected.
	expectedDevices := make(map[string]*safenvml.DeviceInfo)
	discovered := discoverSpecDevices(t)
	// MIG parents are published like any other physical GPU.
	for _, device := range append(discovered.physical, discovered.migParents...) {
		info := device.GetDeviceInfo()
		if info.VirtualizationMode != nvml.GPU_VIRTUALIZATION_MODE_VGPU {
			expectedDevices[strings.ToLower(info.UUID)] = info
		}
	}
	require.NotEmpty(t, expectedDevices, "no physical non-vGPU devices to publish")

	// The workloadmeta NVML collector is disabled unless the NVML feature is present.
	env.SetFeatures(t, env.NVML)
	// Mock the global config first: the first config mock replaces it, which would drop the settings below.
	configmock.New(t)
	// CI runners have no usable OS hostname, and publication is skipped without one.
	setConfigInTest(t, "hostname", "gpu-resources-test-host")
	setConfigInTest(t, "gpu.resources.enabled", true)
	setConfigInTest(t, "gpu.resources.ttl", resourcesTestTTL)

	checkInternal, mockSender := setupGPUCheck(t)
	start := time.Now()
	require.NoError(t, checkInternal.Run())
	end := time.Now()

	published := publishedResources(t, mockSender)
	require.Len(t, published, len(expectedDevices), "expected one gpu_device per physical GPU")

	agentHostname, err := hostname.Get(context.Background())
	require.NoError(t, err)

	for uuid, info := range expectedDevices {
		resource, ok := published[uuid]
		require.True(t, ok, "GPU %s was not published", uuid)
		t.Run(uuid, func(t *testing.T) {
			event, device := resource.event, resource.device
			assert.Equal(t, "integrations-core", event.GetSource())
			assert.Equal(t, "gpu_device", event.GetResource().GetType())
			assert.Equal(t, uuid, device.UUID)
			assert.Equal(t, agentHostname, device.Hostname)
			assert.Equal(t, "nvidia", device.Vendor)

			seenAt := event.GetResource().GetSeenAt().AsTime()
			assert.False(t, seenAt.Before(start.Truncate(time.Second)) || seenAt.After(end), "seen_at %s outside check run [%s, %s]", seenAt, start, end)
			assert.Equal(t, resourcesTestTTL, event.GetResource().GetExpireAt().AsTime().Sub(seenAt))

			// The published values must match what NVML reports for the device, including the empty values
			// of attributes the device cannot report.
			assert.Equal(t, gpuutil.NormalizeGPUDeviceName(info.Name), device.Model)
			assert.Equal(t, info.PCIBusID, device.PCIBusID)
			assert.Equal(t, info.DriverVersion, device.DriverVersion)
			assert.Equal(t, gpuutil.ArchToString(info.Architecture), device.Architecture)
			assert.Equal(t, fmt.Sprintf("%d.%d", info.SMVersion/10, info.SMVersion%10), device.ComputeCapability)
			assert.Equal(t, info.CoreCount, device.CoreCount)
			assert.Equal(t, info.Memory, device.MemoryTotalBytes)
			assert.Equal(t, info.MemoryBusWidth, device.MemoryBusWidth)
			assert.Equal(t, info.VirtualizationModeName, device.VirtualizationMode)
			assert.Equal(t, strings.ToLower(info.BoardSerial), device.BoardSerial)
			assert.Equal(t, strings.ToLower(info.ChassisSerial), device.ChassisSerial)
			assert.Equal(t, info.FabricGUID, device.FabricGUID)
			assert.Equal(t, info.FabricClusterUUID, device.FabricClusterUUID)
			if info.FabricClusterUUID != "" {
				require.NotNil(t, device.FabricCliqueID)
				assert.Equal(t, info.FabricCliqueID, *device.FabricCliqueID)
				assert.Equal(t, fmt.Sprintf("%s:%d", info.FabricClusterUUID, info.FabricCliqueID), device.NVLinkPartition)
			} else {
				assert.Nil(t, device.FabricCliqueID)
				assert.Empty(t, device.NVLinkPartition)
			}
		})
	}

	t.Run("unchanged-devices-not-republished", func(t *testing.T) {
		mockSender.ResetCalls()
		require.NoError(t, checkInternal.Run())
		assert.Empty(t, publishedResources(t, mockSender), "unchanged devices were published again before the refresh interval")
	})
}

func setConfigInTest(t *testing.T, key string, value any) {
	t.Helper()
	cfg := pkgconfigsetup.Datadog()
	previous := cfg.Get(key)
	cfg.SetInTest(key, value)
	t.Cleanup(func() { cfg.SetInTest(key, previous) })
}

// publishedResources decodes the genresources events submitted to the sender, keyed by GPU UUID.
func publishedResources(t *testing.T, mockSender *mocksender.MockSender) map[string]publishedResource {
	t.Helper()
	result := make(map[string]publishedResource)
	for _, call := range mockSender.Mock.Calls {
		if call.Method != "EventPlatformEvent" || call.Arguments.String(1) != eventplatform.EventTypeGenResources {
			continue
		}
		payload, ok := call.Arguments.Get(0).([]byte)
		require.True(t, ok)

		event := &genresources.GenericResourceEvent{}
		require.NoError(t, proto.Unmarshal(payload, event))
		var device gpuresources.Device
		require.NoError(t, json.Unmarshal(event.GetResource().GetFieldsJson(), &device))

		key := event.GetResource().GetKey()
		require.Equal(t, key, device.UUID, "resource key must be the GPU UUID")
		require.NotContains(t, result, key, "GPU %s published twice in one run", key)
		result[key] = publishedResource{event: event, device: device}
	}
	return result
}

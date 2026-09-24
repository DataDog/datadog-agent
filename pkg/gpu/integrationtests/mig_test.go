// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// canonicalMIGProfile matches GPU-instance-level MIG profile names: "1g.35gb",
// and suffixed variants such as "1g.24gb+me", "1g.24gb-me", "1g.24gb+me.all"
// or "4g.96gb+gfx". A split-CI device's leading "<n>c." is not part of it.
var canonicalMIGProfile = regexp.MustCompile(`^[0-9]+g\.[0-9]+gb(?:[+-][a-z]+(?:\.[a-z]+)*)?$`)

func requireMIGTests(t *testing.T) {
	t.Helper()

	if os.Getenv("RUN_MIG_TESTS") == "" {
		t.Skip("Skipping MIG tests: RUN_MIG_TESTS environment variable not set")
	}
}

// TestMIGDeviceListing tests that MIG devices can be listed and have valid properties.
func TestMIGDeviceListing(t *testing.T) {
	testutil.RequireGPU(t)
	requireMIGTests(t)

	lib := initNVML(t)
	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))

	migDevices, err := cache.AllMigDevices()
	require.NoError(t, err, "Should be able to get MIG devices")
	require.NotEmpty(t, migDevices, "Should have at least one MIG device")

	for _, device := range migDevices {
		info := device.GetDeviceInfo()
		t.Logf("MIG device %s has %d cores", info.UUID, info.CoreCount)
		require.NotEmpty(t, info.UUID, "MIG device UUID should not be empty")
		require.NotEmpty(t, info.Name, "MIG device name should not be empty")

		// We cannot know for sure the real expected value, but we can be sure that it's greater than 500. This way we avoid
		// issues when we're just reporting the number of multiprocessors instead of the actual number of cores.
		require.Greater(t, info.CoreCount, 500, "MIG device should have more than 500 cores")
	}
}

// TestMIGParentChildCoreCount tests that the parent device's core count equals
// the sum of its MIG children's core counts.
func TestMIGParentChildCoreCount(t *testing.T) {
	testutil.RequireGPU(t)
	requireMIGTests(t)

	lib := initNVML(t)
	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))

	physicalDevices, err := cache.AllPhysicalDevices()
	require.NoError(t, err, "Should be able to get physical devices")

	foundMIGParent := false
	for _, device := range physicalDevices {
		physDev, ok := device.(*safenvml.PhysicalDevice)
		require.True(t, ok, "Device should be a PhysicalDevice")

		if !physDev.HasMIGFeatureEnabled || len(physDev.MIGChildren) == 0 {
			t.Logf("Physical device %s has no MIG children, core count is %d", physDev.GetDeviceInfo().UUID, physDev.GetDeviceInfo().CoreCount)
			continue
		}

		foundMIGParent = true
		parentInfo := physDev.GetDeviceInfo()

		childCoreSum := 0
		distinctProfiles := make(map[string]struct{})
		for _, migChild := range physDev.MIGChildren {
			childInfo := migChild.GetDeviceInfo()
			childCoreSum += childInfo.CoreCount
			distinctProfiles[migChild.Profile] = struct{}{}
		}

		// The parent's MIG-mode core estimate is derived from a single GPU
		// instance profile (device.go), so parent/child equality only holds
		// for uniform layouts. Mixed layouts -- e.g. 3x 1g.35gb + 1x 1g.18gb
		// on the same card -- legitimately add up to less than the estimate.
		if len(distinctProfiles) > 1 {
			t.Logf("Physical device %s has a mixed MIG layout (%d distinct profiles, %d children, child sum %d vs parent estimate %d); skipping core-count comparison",
				parentInfo.UUID, len(distinctProfiles), len(physDev.MIGChildren), childCoreSum, parentInfo.CoreCount)
			continue
		}

		require.Equal(t, parentInfo.CoreCount, childCoreSum,
			"Parent device core count should equal sum of MIG children core counts")
		t.Logf("Parent device %s has %d cores, sum of MIG children core counts is %d", parentInfo.UUID, parentInfo.CoreCount, childCoreSum)
	}

	require.True(t, foundMIGParent, "Should have at least one physical device with MIG enabled and children")
}

// TestMIGDeviceProfileName tests that every MIG device resolves the canonical
// profile name that backs the gpu_mig_profile tag. This is the hardware side of
// that tag: the unit tests in pkg/gpu/safenvml cover the NVML call chain with a
// mocked driver, but only real MIG hardware proves the driver actually returns a
// name for a live GPU instance, and that the "MIG " prefix it uses is stripped.
func TestMIGDeviceProfileName(t *testing.T) {
	testutil.RequireGPU(t)
	requireMIGTests(t)

	lib := initNVML(t)
	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))

	migDevices, err := cache.AllMigDevices()
	require.NoError(t, err, "Should be able to get MIG devices")
	require.NotEmpty(t, migDevices, "Should have at least one MIG device")

	for _, device := range migDevices {
		migDevice, ok := device.(*safenvml.MIGDevice)
		require.True(t, ok, "Device %s should be a MIGDevice", device.GetDeviceInfo().UUID)

		info := migDevice.GetDeviceInfo()
		t.Logf("MIG device %s (GPU instance %d) has profile %q", info.UUID, migDevice.MIGInstanceID, migDevice.Profile)

		require.NotEmpty(t, migDevice.Profile, "MIG device %s should have a profile name", info.UUID)
		require.NotContains(t, strings.ToLower(migDevice.Profile), "mig",
			"profile name %q should not carry the driver's MIG prefix", migDevice.Profile)
		require.Regexp(t, canonicalMIGProfile, migDevice.Profile,
			"profile name %q should be a canonical MIG profile name", migDevice.Profile)
		// On a privileged runner the fallback can produce the profile too, and
		// would mask a broken primary path; check the name-based one directly.
		require.Equal(t, migDevice.Profile, safenvml.ParseMIGProfileFromDeviceName(info.Name),
			"device name %q should carry the profile, so no privileged fallback is needed", info.Name)
	}
}

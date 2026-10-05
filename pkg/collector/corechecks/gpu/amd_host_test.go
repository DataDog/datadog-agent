// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && test

package gpu

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/gpuenvironment"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/amd"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// withoutNVML makes every NVML initialization fail as on a host without the
// NVIDIA driver.
func withoutNVML(t *testing.T) {
	ddnvml.WithMockNvmlNewFunc(t, func(...nvml.LibraryOption) nvml.Interface {
		return testutil.NewMockNVML(testutil.WithInitReturn(nvml.ERROR_LIBRARY_NOT_FOUND))
	})
}

// fakeAMDHost returns a sysfs root with one MI300X-like GPU.
func fakeAMDHost(t *testing.T) string {
	fs := amd.NewFakeSysfs(t)
	fs.SetDriverVersion("6.14.14")
	devDir := fs.AddPCIDevice("0000:c1:00.0", "amdgpu", amd.MI300XAttributes("00c0ffee00c0ffee"))
	fs.AddHwmon(devDir, "hwmon0", amd.JunctionOnlyHwmon())
	fs.AddCard("card0", devDir)
	return fs.Root
}

// applyAMDTestSettings enables GPU monitoring and AMD collection, unless a
// test overrides it, and applies the settings for the duration of the test.
func applyAMDTestSettings(t *testing.T, settings map[string]any) {
	t.Helper()
	WithGPUConfigEnabled(t)
	if _, overridden := settings["gpu.amd.enabled"]; !overridden {
		settings = maps.Clone(settings)
		if settings == nil {
			settings = map[string]any{}
		}
		settings["gpu.amd.enabled"] = true
	}
	for key, value := range settings {
		previous := pkgconfigsetup.Datadog().Get(key)
		t.Cleanup(func() { pkgconfigsetup.Datadog().SetInTest(key, previous) })
		pkgconfigsetup.Datadog().SetInTest(key, value)
	}
}

// setupNVIDIACheckOnHost configures the NVIDIA GPU check with the PCI
// inventory under sysRoot, to test its behavior on hosts with AMD GPUs.
func setupNVIDIACheckOnHost(t *testing.T, sysRoot string, settings map[string]any) *Check {
	t.Helper()
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	checkGeneric := newCheck(taggerfxmock.SetupFakeTagger(t), testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMock(t))
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	applyAMDTestSettings(t, settings)
	check.containerProvider = newMockContainerProvider(t, map[int]string{})
	check.sysRoot = sysRoot
	require.NoError(t, check.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test", "provider"))
	t.Cleanup(func() { check.Cancel() })
	mocksender.NewMockSenderWithSenderManager(check.ID(), senderManager).SetupAcceptAll()
	return check
}

// On an AMD-only host, which the AMD GPU check collects, NVML being
// unavailable must neither fail the NVIDIA GPU check nor raise its issue.
func TestNVIDIACheckOnAMDOnlyHost(t *testing.T) {
	withoutNVML(t)
	healthStore := healthplatformmock.New(t)
	check := setupNVIDIACheckOnHost(t, fakeAMDHost(t), nil)
	check.SetIssueReporter(healthStore)
	check.syncNvmlHealthIssue(true, false)
	issueID := gpuHealthIssueID(gpuenvironment.ReasonNvmlUnavailable)
	require.NotNil(t, healthStore.GetIssue(issueID))

	require.NoError(t, check.Run())
	assert.Nil(t, healthStore.GetIssue(issueID))
}

// An NVIDIA audio function is not a GPU: the host is still AMD-only.
func TestNVIDIACheckOnAMDHostWithNVIDIAAudio(t *testing.T) {
	withoutNVML(t)
	root := fakeAMDHost(t)
	dir := filepath.Join(root, "bus", "pci", "devices", "0000:21:00.1")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor"), []byte("0x10de\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "class"), []byte("0x040300\n"), 0o644))
	check := setupNVIDIACheckOnHost(t, root, nil)
	healthStore := healthplatformmock.New(t)
	check.SetIssueReporter(healthStore)
	check.syncNvmlHealthIssue(true, false)

	require.NoError(t, check.Run())
	assert.Nil(t, healthStore.GetIssue(gpuHealthIssueID(gpuenvironment.ReasonNvmlUnavailable)))
}

// An AMD GPU does not make a broken NVIDIA driver healthy: with an NVIDIA GPU,
// an unreadable inventory, AMD collection disabled, or no GPU at all, the
// NVIDIA GPU check keeps failing and reporting the NVML issue.
func TestNVIDIACheckKeepsNVMLIssueUnlessAMDOnly(t *testing.T) {
	for name, tc := range map[string]struct {
		pciAttributes map[string]string // an extra PCI device, if any
		amdGPU        bool
		settings      map[string]any
	}{
		"unbound NVIDIA GPU": {pciAttributes: map[string]string{"vendor": "0x10de\n", "class": "0x030200\n"}, amdGPU: true},
		"NVIDIA accelerator": {pciAttributes: map[string]string{"vendor": "0x10de\n", "class": "0x120000\n"}, amdGPU: true},
		"unknown class":      {pciAttributes: map[string]string{"vendor": "0x10de\n"}, amdGPU: true},
		"invalid vendor":     {pciAttributes: map[string]string{"vendor": "invalid\n"}, amdGPU: true},
		"AMD collection off": {amdGPU: true, settings: map[string]any{"gpu.amd.enabled": false}},
		"no GPU":             {},
	} {
		t.Run(name, func(t *testing.T) {
			withoutNVML(t)
			root := t.TempDir()
			if tc.amdGPU {
				root = fakeAMDHost(t)
			}
			if tc.pciAttributes != nil {
				dir := filepath.Join(root, "bus", "pci", "devices", "0000:21:00.0")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				for key, value := range tc.pciAttributes {
					require.NoError(t, os.WriteFile(filepath.Join(dir, key), []byte(value), 0o644))
				}
			}
			healthStore := healthplatformmock.New(t)
			check := setupNVIDIACheckOnHost(t, root, tc.settings)
			check.SetIssueReporter(healthStore)
			check.syncNvmlHealthIssue(true, false)

			require.Error(t, check.Run())
			assert.NotNil(t, healthStore.GetIssue(gpuHealthIssueID(gpuenvironment.ReasonNvmlUnavailable)))
		})
	}
}

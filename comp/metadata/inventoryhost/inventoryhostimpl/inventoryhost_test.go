// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryhostimpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	pkgUtils "github.com/DataDog/datadog-agent/comp/metadata/packagesigning/utils"
	"github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	"github.com/DataDog/datadog-agent/pkg/gohai/memory"
	"github.com/DataDog/datadog-agent/pkg/gohai/network"
	"github.com/DataDog/datadog-agent/pkg/gohai/platform"
	gohaiutils "github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func cpuMock() *cpu.Info {
	return &cpu.Info{
		CPUCores:             gohaiutils.NewValue[uint64](6),
		CPULogicalProcessors: gohaiutils.NewValue[uint64](6),
		VendorID:             gohaiutils.NewValue("GenuineIntel"),
		ModelName:            gohaiutils.NewValue("Intel_i7-8750H"),
		Model:                gohaiutils.NewValue("158"),
		Family:               gohaiutils.NewValue("6"),
		Stepping:             gohaiutils.NewValue("10"),
		Mhz:                  gohaiutils.NewValue(2208.006),
		CacheSizeKB:          gohaiutils.NewValue[uint64](9216),
		CPUPkgs:              gohaiutils.NewValue[uint64](6),
		CPUNumaNodes:         gohaiutils.NewValue[uint64](6),
		CacheSizeL1Bytes:     gohaiutils.NewValue[uint64](1234),
		CacheSizeL2Bytes:     gohaiutils.NewValue[uint64](1234),
		CacheSizeL3Bytes:     gohaiutils.NewValue[uint64](1234),
	}
}

func memoryMock() *memory.Info {
	return &memory.Info{
		TotalBytes:  gohaiutils.NewValue[uint64](1234567890),
		SwapTotalKb: gohaiutils.NewValue[uint64](1205632),
	}
}

func networkMock() (*network.Info, error) {
	return &network.Info{
		IPAddress:   "192.168.24.138",
		IPAddressV6: gohaiutils.NewValue("fe80::20c:29ff:feb6:d232"),
		MacAddress:  "00:0c:29:b6:d2:32",
	}, nil
}

func platformMock() *platform.Info {
	return &platform.Info{
		KernelName:       gohaiutils.NewValue("Linux"),
		KernelRelease:    gohaiutils.NewValue("5.17.0-1-amd64"),
		KernelVersion:    gohaiutils.NewValue("Debian_5.17.3-1"),
		OS:               gohaiutils.NewValue("GNU/Linux"),
		HardwarePlatform: gohaiutils.NewValue("unknown"),
		GoVersion:        gohaiutils.NewValue("1.17.7"),
		GoOS:             gohaiutils.NewValue("linux"),
		GoArch:           gohaiutils.NewValue("amd64"),
		Hostname:         gohaiutils.NewValue("debdev"),
		Machine:          gohaiutils.NewValue("x86_64"),
		Family:           gohaiutils.NewErrorValue[string](gohaiutils.ErrNotCollectable),
		Processor:        gohaiutils.NewValue("unknown"),
	}
}
func pkgSigningMock(_ log.Component) (bool, bool) { return true, false }

func cpuErrorMock() *cpu.Info                  { return &cpu.Info{} }
func memoryErrorMock() *memory.Info            { return &memory.Info{} }
func networkErrorMock() (*network.Info, error) { return nil, fmt.Errorf("err") }
func platformErrorMock() *platform.Info        { return &platform.Info{} }

func setupHostMetadataMock(t *testing.T) {
	t.Cleanup(func() {
		cpuGet = cpu.CollectInfo
		memoryGet = memory.CollectInfo
		networkGet = network.CollectInfo
		platformGet = platform.CollectInfo
		osVersionGet = utils.GetOSVersion
		pkgSigningGet = pkgUtils.GetLinuxGlobalSigningPolicies
	})

	cpuGet = cpuMock
	memoryGet = memoryMock
	networkGet = networkMock
	platformGet = platformMock
	osVersionGet = func() string { return "testOS" }
	dmi.SetupMock(t, "hypervisorUUID", "dmiUUID", "boardTag", "boardVendor")
	cloudproviders.Mock(t, "some_cloud_provider", "some_host_id", "test_source", "test_id_1234")
	pkgSigningGet = pkgSigningMock
}

func setupHostMetadataErrorMock(t *testing.T) {
	setupHostMetadataMock(t)

	cpuGet = cpuErrorMock
	memoryGet = memoryErrorMock
	networkGet = networkErrorMock
	platformGet = platformErrorMock
	dmi.SetupMock(t, "", "", "", "")
}

func getTestInventoryHost(t *testing.T) *invHost {
	p := newInventoryHostProvider(
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			fx.Provide(func() serializer.MetricSerializer { return &serializer.MockSerializer{} }),
		),
	)
	return p.Comp.(*invHost)
}

func TestGetPayload(t *testing.T) {
	setupHostMetadataMock(t)

	expectedMetadata := &hostMetadata{
		CPUCores:                     6,
		CPULogicalProcessors:         6,
		CPUVendor:                    "GenuineIntel",
		CPUModel:                     "Intel_i7-8750H",
		CPUModelID:                   "158",
		CPUFamily:                    "6",
		CPUStepping:                  "10",
		CPUFrequency:                 2208.006,
		CPUCacheSize:                 9437184,
		KernelName:                   "Linux",
		KernelRelease:                "5.17.0-1-amd64",
		KernelVersion:                "Debian_5.17.3-1",
		OS:                           "GNU/Linux",
		CPUArchitecture:              "unknown",
		MemoryTotalKb:                1205632,
		MemorySwapTotalKb:            1205632,
		IPAddress:                    "192.168.24.138",
		IPv6Address:                  "fe80::20c:29ff:feb6:d232",
		MacAddress:                   "00:0c:29:b6:d2:32",
		AgentVersion:                 version.AgentVersion,
		CloudProvider:                "some_cloud_provider",
		CloudProviderAccountID:       "some_host_id",
		CloudProviderSource:          "test_source",
		CloudProviderHostID:          "test_id_1234",
		OsVersion:                    "testOS",
		HypervisorGuestUUID:          "hypervisorUUID",
		DmiProductUUID:               "dmiUUID",
		DmiBoardAssetTag:             "boardTag",
		DmiBoardVendor:               "boardVendor",
		LinuxPackageSigningEnabled:   true,
		RPMGlobalRepoGPGCheckEnabled: false,
	}

	ih := getTestInventoryHost(t)

	p := ih.getPayload().(*Payload)
	assert.Equal(t, expectedMetadata, p.Metadata)
}

func TestGetPayloadError(t *testing.T) {
	setupHostMetadataErrorMock(t)

	ih := getTestInventoryHost(t)

	p := ih.getPayload().(*Payload)
	expected := &hostMetadata{
		AgentVersion:                 version.AgentVersion,
		CloudProvider:                "some_cloud_provider",
		CloudProviderAccountID:       "some_host_id",
		CloudProviderSource:          "test_source",
		CloudProviderHostID:          "test_id_1234",
		OsVersion:                    "testOS",
		LinuxPackageSigningEnabled:   true,
		RPMGlobalRepoGPGCheckEnabled: false,
	}
	assert.Equal(t, expected, p.Metadata)
}

func TestFlareProviderFilename(t *testing.T) {
	ih := getTestInventoryHost(t)
	assert.Equal(t, "host.json", ih.FlareFileName)
}

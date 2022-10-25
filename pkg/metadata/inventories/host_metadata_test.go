// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package inventories

import (
	"fmt"
	"testing"

	"github.com/DataDog/gohai/cpu"
	"github.com/DataDog/gohai/memory"
	"github.com/DataDog/gohai/network"
	"github.com/DataDog/gohai/platform"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	expectedMetadata = &HostMetadata{
		CPUCores:             6,
		CPULogicalProcessors: 6,
		CPUVendor:            "GenuineIntel",
		CPUModel:             "Intel_i7-8750H",
		CPUModelID:           "158",
		CPUFamily:            "6",
		CPUStepping:          "10",
		CPUFrequency:         2208.006,
		CPUCacheSize:         9437184,
		KernelName:           "Linux",
		KernelRelease:        "5.17.0-1-amd64",
		KernelVersion:        "Debian_5.17.3-1",
		OS:                   "GNU/Linux",
		CPUArchitecture:      "unknown",
		MemoryTotalKb:        1205632,
		MemorySwapTotalKb:    1205632,
		IPAddress:            "192.168.24.138",
		IPv6Address:          "fe80::20c:29ff:feb6:d232",
		MacAddress:           "00:0c:29:b6:d2:32",
		AgentVersion:         version.AgentVersion,
		CloudProvider:        "some_cloud_provider",
		CloudIdentifiers:     map[string]string{"cloud-test": "some_identifier"},
		OsVersion:            "testOS",
	}
)

func cpuMock() (*cpu.Cpu, []string, error) {
	return &cpu.Cpu{
		CpuCores:             6,
		CpuLogicalProcessors: 6,
		VendorId:             "GenuineIntel",
		ModelName:            "Intel_i7-8750H",
		Model:                "158",
		Family:               "6",
		Stepping:             "10",
		Mhz:                  2208.006,
		CacheSizeBytes:       9437184,
		CpuPkgs:              6,
		CpuNumaNodes:         6,
		CacheSizeL1Bytes:     1234,
		CacheSizeL2Bytes:     1234,
		CacheSizeL3Bytes:     1234,
	}, nil, nil
}

func memoryMock() (*memory.Memory, []string, error) {
	return &memory.Memory{
		TotalBytes:     1234567890,
		SwapTotalBytes: 1234567890,
	}, nil, nil
}

func networkMock() (*network.Network, []string, error) {
	return &network.Network{
		IpAddress:   "192.168.24.138",
		IpAddressv6: "fe80::20c:29ff:feb6:d232",
		MacAddress:  "00:0c:29:b6:d2:32",
	}, nil, nil
}

func platformMock() (*platform.Platform, []string, error) {
	return &platform.Platform{
		KernelName:       "Linux",
		KernelRelease:    "5.17.0-1-amd64",
		KernelVersion:    "Debian_5.17.3-1",
		OS:               "GNU/Linux",
		HardwarePlatform: "unknown",
		GoVersion:        "1.17.7",
		GoOS:             "linux",
		GoArch:           "amd64",
		Hostname:         "debdev",
		Machine:          "x86_64",
		Family:           "",
		Processor:        "unknown",
	}, nil, nil
}

func setupHostMetadataMock() func() {
	reset := func() {
		cpuGet = cpu.Get
		memoryGet = memory.Get
		networkGet = network.Get
		platformGet = platform.Get

		inventoryMutex.Lock()
		delete(agentMetadata, string(AgentCloudProvider))
		delete(hostMetadata, string(HostOSVersion))
		inventoryMutex.Unlock()
	}

	cpuGet = cpuMock
	memoryGet = memoryMock
	networkGet = networkMock
	platformGet = platformMock

	SetAgentMetadata(AgentCloudProvider, "some_cloud_provider")
	SetHostMetadata(HostCloudIdentifiers, map[string]string{"cloud-test": "some_identifier"})
	SetHostMetadata(HostOSVersion, "testOS")

	return reset
}

func TestGetHostMetadata(t *testing.T) {
	resetFunc := setupHostMetadataMock()
	defer resetFunc()

	m := getHostMetadata()
	assert.Equal(t, expectedMetadata, m)
}

func cpuErrorMock() (*cpu.Cpu, []string, error)                { return nil, nil, fmt.Errorf("err") }
func memoryErrorMock() (*memory.Memory, []string, error)       { return nil, nil, fmt.Errorf("err") }
func networkErrorMock() (*network.Network, []string, error)    { return nil, nil, fmt.Errorf("err") }
func platformErrorMock() (*platform.Platform, []string, error) { return nil, nil, fmt.Errorf("err") }

func setupHostMetadataErrorMock() func() {
	reset := func() {
		cpuGet = cpu.Get
		memoryGet = memory.Get
		networkGet = network.Get
		platformGet = platform.Get
	}

	cpuGet = cpuErrorMock
	memoryGet = memoryErrorMock
	networkGet = networkErrorMock
	platformGet = platformErrorMock
	hostMetadata = make(AgentMetadata)
	return reset
}

func TestGetHostMetadataError(t *testing.T) {
	resetFunc := setupHostMetadataErrorMock()
	defer resetFunc()

	m := getHostMetadata()
	expected := &HostMetadata{AgentVersion: version.AgentVersion}
	assert.Equal(t, expected, m)
}

// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package main

import (
	"encoding/json"
	"net"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectedCollectors_String(t *testing.T) {
	sc := &SelectedCollectors{
		"foo": struct{}{},
		"bar": struct{}{},
	}
	assert.Equal(t, "[bar foo]", sc.String())
}

// gohaiPayload defines the format we expect the gohai information
// to be in.
// Any change to this datastructure should be notified to the backend
// team to ensure compatibility.
type gohaiPayload struct {
	CPU struct {
		CPUCores             string `json:"cpu_cores"`
		CPULogicalProcessors string `json:"cpu_logical_processors"`
		Family               string `json:"family"`
		Mhz                  string `json:"mhz"`
		Model                string `json:"model"`
		ModelName            string `json:"model_name"`
		Stepping             string `json:"stepping"`
		VendorID             string `json:"vendor_id"`

		// CacheSize is only reported on Linux
		CacheSize string `json:"cache_size"`

		// On Windows, we report additional fields
		CacheSizeL1  string `json:"cache_size_l1"`
		CacheSizeL2  string `json:"cache_size_l2"`
		CacheSizeL3  string `json:"cache_size_l3"`
		CPUNumaNodes string `json:"cpu_numa_nodes"`
		CPUPkgs      string `json:"cpu_pkgs"`
	} `json:"cpu"`
	Filesystem []struct {
		KbSize string `json:"kb_size"`
		// MountedOn can be empty on Windows
		MountedOn string `json:"mounted_on"`
		Name      string `json:"name"`
	} `json:"filesystem"`
	Memory struct {
		// SwapTotal is not reported on Windows
		SwapTotal string `json:"swap_total"`
		Total     string `json:"total"`
	} `json:"memory"`
	Network struct {
		Interfaces []struct {
			Ipv4        []string `json:"ipv4"`
			Ipv6        []string `json:"ipv6"`
			Ipv6Network string   `json:"ipv6-network"`
			Macaddress  string   `json:"macaddress"`
			Name        string   `json:"name"`
			Ipv4Network string   `json:"ipv4-network"`
		} `json:"interfaces"`
		Ipaddress   string `json:"ipaddress"`
		Ipaddressv6 string `json:"ipaddressv6"`
		Macaddress  string `json:"macaddress"`
	} `json:"network"`
	Platform struct {
		Gooarch       string `json:"GOOARCH"`
		Goos          string `json:"GOOS"`
		GoV           string `json:"goV"`
		Hostname      string `json:"hostname"`
		KernelName    string `json:"kernel_name"`
		KernelRelease string `json:"kernel_release"`
		// KernelVersion is not reported on Windows
		KernelVersion string `json:"kernel_version"`
		Machine       string `json:"machine"`
		Os            string `json:"os"`
		Processor     string `json:"processor"`
		// On Windows, we report additional fields
		Family string `json:"family"`
	} `json:"platform"`
}

func TestGohaiSerialization(t *testing.T) {
	gohai, err := Collect()

	assert.NoError(t, err)

	gohaiJSON, err := json.Marshal(gohai)
	assert.NoError(t, err)

	var payload gohaiPayload
	assert.NoError(t, json.Unmarshal(gohaiJSON, &payload))

	assert.NotEmpty(t, payload.CPU.CPUCores)
	assert.NotEmpty(t, payload.CPU.CPULogicalProcessors)
	assert.NotEmpty(t, payload.CPU.Family)
	if runtime.GOARCH != "arm64" {
		// (Mhz is not defined on ARM64)
		assert.NotEmpty(t, payload.CPU.Mhz)
	}
	assert.NotEmpty(t, payload.CPU.Model)
	assert.NotEmpty(t, payload.CPU.ModelName)
	assert.NotEmpty(t, payload.CPU.Stepping)
	assert.NotEmpty(t, payload.CPU.VendorID)

	if runtime.GOOS == "windows" {
		// Additional fields that we report on Windows
		assert.NotEmpty(t, payload.CPU.CacheSizeL1)
		assert.NotEmpty(t, payload.CPU.CacheSizeL2)
		assert.NotEmpty(t, payload.CPU.CacheSizeL3)
		assert.NotEmpty(t, payload.CPU.CPUNumaNodes)
		assert.NotEmpty(t, payload.CPU.CPUPkgs)
	}

	if assert.NotEmpty(t, payload.Filesystem) {
		if runtime.GOOS != "windows" {
			// On Windows, MountedOn can be empty
			assert.NotEmpty(t, payload.Filesystem[0].MountedOn, 0)
		}
		assert.NotEmpty(t, payload.Filesystem[0].KbSize, 0)
		assert.NotEmpty(t, payload.Filesystem[0].Name, 0)
	}
	if runtime.GOOS != "windows" {
		// Not reported on Windows
		assert.NotEmpty(t, payload.Memory.SwapTotal)
	}
	assert.NotEmpty(t, payload.Memory.Total)

	if assert.NotEmpty(t, payload.Network.Interfaces) {
		for _, itf := range payload.Network.Interfaces {
			assert.NotEmpty(t, itf.Name)
			// Some interfaces don't have MacAddresses
			//assert.NotEmpty(t, itf.Macaddress)

			if len(itf.Ipv4) == 0 && len(itf.Ipv6) == 0 {
				// Disabled interfaces won't have any IP address
				continue
			}
			if len(itf.Ipv4) == 0 {
				assert.NotEmpty(t, itf.Ipv6)
				assert.NotEmpty(t, itf.Ipv6Network)
				for _, ip := range itf.Ipv6 {
					assert.NotNil(t, net.ParseIP(ip))
				}
			} else {
				assert.NotEmpty(t, itf.Ipv4)
				assert.NotEmpty(t, itf.Ipv4Network)
				for _, ip := range itf.Ipv4 {
					assert.NotNil(t, net.ParseIP(ip))
				}
			}
		}
	}
	assert.NotEmpty(t, payload.Network.Ipaddress)
	assert.NotNil(t, net.ParseIP(payload.Network.Ipaddress))
	// Ipaddressv6 *can* be empty
	// assert.NotEmpty(t, payload.Network.Ipaddressv6)
	assert.NotEmpty(t, payload.Network.Macaddress)

	assert.NotEmpty(t, payload.Platform.Gooarch)
	assert.NotEmpty(t, payload.Platform.Goos)
	assert.NotEmpty(t, payload.Platform.GoV)
	assert.NotEmpty(t, payload.Platform.Hostname)
	assert.NotEmpty(t, payload.Platform.KernelName)
	assert.NotEmpty(t, payload.Platform.KernelRelease)
	assert.NotEmpty(t, payload.Platform.Machine)
	assert.NotEmpty(t, payload.Platform.Os)
	if runtime.GOOS != "windows" {
		// Not reported on Windows
		assert.NotEmpty(t, payload.Platform.KernelVersion)
		assert.NotEmpty(t, payload.Platform.Processor)
	} else {
		// Additional fields that we report on Windows
		assert.NotEmpty(t, payload.Platform.Family)
	}
}

// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build test

package cpu

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/stretchr/testify/assert"
)

func TestCollectCPU(t *testing.T) {
	cpuInfo := CollectInfo()

	errorGetters := map[string]error{
		"VendorID":             cpuInfo.VendorID.Error(),
		"ModelName":            cpuInfo.ModelName.Error(),
		"CPUCores":             cpuInfo.CPUCores.Error(),
		"CPULogicalProcessors": cpuInfo.CPULogicalProcessors.Error(),
		"Mhz":                  cpuInfo.Mhz.Error(),
		"CacheSizeKB":          cpuInfo.CacheSizeKB.Error(),
		"Family":               cpuInfo.Family.Error(),
		"Model":                cpuInfo.Model.Error(),
		"Stepping":             cpuInfo.Stepping.Error(),
		"CPUPkgs":              cpuInfo.CPUPkgs.Error(),
		"CPUNumaNodes":         cpuInfo.CPUNumaNodes.Error(),
		"CacheSizeL1Bytes":     cpuInfo.CacheSizeL1Bytes.Error(),
		"CacheSizeL2Bytes":     cpuInfo.CacheSizeL2Bytes.Error(),
		"CacheSizeL3Bytes":     cpuInfo.CacheSizeL3Bytes.Error(),
	}

	for fieldname, err := range errorGetters {
		if err != nil {
			assert.ErrorIsf(t, err, utils.ErrNotCollectable, "cpu: field %s could not be collected", fieldname)
		}
	}
}

func TestCPUAsJSON(t *testing.T) {
	cpuInfo := CollectInfo()

	// Any change to this datastructure should be notified to the backend
	// team to ensure compatibility.
	type CPU struct {
		CPUCores             string `json:"cpu_cores"`
		CPULogicalProcessors string `json:"cpu_logical_processors"`
		Family               string `json:"family"`
		Mhz                  string `json:"mhz"`
		Model                string `json:"model"`
		ModelName            string `json:"model_name"`
		Stepping             string `json:"stepping"`
		VendorID             string `json:"vendor_id"`
		CacheSizeKB          string `json:"cache_size"`
		CacheSizeL1Bytes     string `json:"cache_size_l1"`
		CacheSizeL2Bytes     string `json:"cache_size_l2"`
		CacheSizeL3Bytes     string `json:"cache_size_l3"`
		CPUNumaNodes         string `json:"cpu_numa_nodes"`
		CPUPkgs              string `json:"cpu_pkgs"`
	}

	var decodedCPU CPU
	utils.RequireMarshallJSON(t, cpuInfo, &decodedCPU)

	// output CPUCores, CPULogicalProcessors, Family, Mhz, Model, ModelName, Stepping, VendorID, CacheSizeKB, CacheSizeL1Bytes, CacheSizeL2Bytes, CacheSizeL3Bytes, CPUNumaNodes, CPUPkgs
	fmt.Println("CPUCores: ", decodedCPU.CPUCores)
	fmt.Println("CPULogicalProcessors: ", decodedCPU.CPULogicalProcessors)
	fmt.Println("Family: ", decodedCPU.Family)
	fmt.Println("Mhz: ", decodedCPU.Mhz)
	fmt.Println("Model: ", decodedCPU.Model)
	fmt.Println("ModelName: ", decodedCPU.ModelName)
	fmt.Println("Stepping: ", decodedCPU.Stepping)
	fmt.Println("VendorID: ", decodedCPU.VendorID)
	fmt.Println("CacheSizeKB: ", decodedCPU.CacheSizeKB)
	fmt.Println("CacheSizeL1Bytes: ", decodedCPU.CacheSizeL1Bytes)
	fmt.Println("CacheSizeL2Bytes: ", decodedCPU.CacheSizeL2Bytes)
	fmt.Println("CacheSizeL3Bytes: ", decodedCPU.CacheSizeL3Bytes)
	fmt.Println("CPUNumaNodes: ", decodedCPU.CPUNumaNodes)
	fmt.Println("CPUPkgs: ", decodedCPU.CPUPkgs)

	utils.AssertDecodedValue(t, decodedCPU.CPUCores, &cpuInfo.CPUCores, "")
	utils.AssertDecodedValue(t, decodedCPU.CPULogicalProcessors, &cpuInfo.CPULogicalProcessors, "")
	utils.AssertDecodedValue(t, decodedCPU.Family, &cpuInfo.Family, "")
	utils.AssertDecodedValue(t, decodedCPU.Mhz, &cpuInfo.Mhz, "")
	utils.AssertDecodedValue(t, decodedCPU.Model, &cpuInfo.Model, "")
	utils.AssertDecodedValue(t, decodedCPU.ModelName, &cpuInfo.ModelName, "")
	utils.AssertDecodedValue(t, decodedCPU.Stepping, &cpuInfo.Stepping, "")
	utils.AssertDecodedValue(t, decodedCPU.VendorID, &cpuInfo.VendorID, "")
	utils.AssertDecodedValue(t, decodedCPU.CacheSizeKB, &cpuInfo.CacheSizeKB, " KB")
	utils.AssertDecodedValue(t, decodedCPU.CacheSizeL1Bytes, &cpuInfo.CacheSizeL1Bytes, "")
	utils.AssertDecodedValue(t, decodedCPU.CacheSizeL2Bytes, &cpuInfo.CacheSizeL2Bytes, "")
	utils.AssertDecodedValue(t, decodedCPU.CacheSizeL3Bytes, &cpuInfo.CacheSizeL3Bytes, "")
	utils.AssertDecodedValue(t, decodedCPU.CPUNumaNodes, &cpuInfo.CPUNumaNodes, "")
	utils.AssertDecodedValue(t, decodedCPU.CPUPkgs, &cpuInfo.CPUPkgs, "")
}

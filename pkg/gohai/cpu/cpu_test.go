// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package cpu

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	marshallable, _, err := cpuInfo.AsJSON()
	require.NoError(t, err)

	marshalled, err := json.Marshal(marshallable)
	require.NoError(t, err)

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

	decoder := json.NewDecoder(bytes.NewReader(marshalled))
	// do not ignore unknown fields
	decoder.DisallowUnknownFields()

	var decodedCPU CPU
	err = decoder.Decode(&decodedCPU)
	require.NoError(t, err)

	// check that we read the full json
	require.False(t, decoder.More())

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

// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build linux && !arm64

package cpu

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/stretchr/testify/require"
)

func TestGetCPUInfo(t *testing.T) {
	cpuinfo := []string{
		"processor       : 0",
		"vendor_id       : GenuineIntel",
		"cpu family      : 6",
		"model\t         : 45",
		"model name      : Intel(R) Xeon(R) CPU E5-2660 0 @ 2.20GHz",
		"stepping        : 6",
		"microcode       : 1561",
		"cpu MHz       \t: 600.000",
		"cache size      : 20480 KB",
		"physical id     : 0",
		"siblings        : 16",
		"core id         : 0",
		"cpu cores       : 8",
		"apicid          : 0",
		"initial apicid  : 0",
		"fpu             : yes",
		"fpu_exception   : yes",
		"cpuid level     : 13",
		"wp              : yes",
		"",
		"bogomips        : 4399.93",
		"clflush size    : 64",
		"cache_alignment : 64",
		"address sizes   : 46 bits physical, 48 bits virtual",
		"power management:",
	}

	cpuInfo := getCPUInfoWithReader(func() ([]string, error) { return cpuinfo, nil })
	require.NotNil(t, cpuInfo)

	expected := &Info{
		CPUPkgs:              utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CPUNumaNodes:         utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL1Bytes:     utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL2Bytes:     utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL3Bytes:     utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		VendorID:             utils.NewValue("GenuineIntel"),
		ModelName:            utils.NewValue("Intel(R) Xeon(R) CPU E5-2660 0 @ 2.20GHz"),
		CPUCores:             utils.NewValue(uint64(8)),
		CPULogicalProcessors: utils.NewValue(uint64(16)),
		Mhz:                  utils.NewValue(600.),
		CacheSizeKB:          utils.NewValue(uint64(20480)),
		Family:               utils.NewValue("6"),
		Model:                utils.NewValue("45"),
		Stepping:             utils.NewValue("6"),
	}

	require.Equal(t, expected, cpuInfo)
}

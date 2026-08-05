// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package cpu

import (
	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	gopsutilcpu "github.com/shirou/gopsutil/v4/cpu"
)

func getCPUInfo() *Info {
	info := &Info{
		VendorID:         utils.NewValue("IBM"),
		CacheSizeKB:      utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		Family:           utils.NewErrorValue[string](utils.ErrNotCollectable),
		Model:            utils.NewErrorValue[string](utils.ErrNotCollectable),
		Stepping:         utils.NewErrorValue[string](utils.ErrNotCollectable),
		CPUPkgs:          utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CPUNumaNodes:     utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL1Bytes: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL2Bytes: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL3Bytes: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
	}

	cpuInfos, err := gopsutilcpu.Info()
	if err != nil {
		info.ModelName = utils.NewErrorValue[string](err)
		info.Mhz = utils.NewErrorValue[float64](err)
	} else if len(cpuInfos) == 0 {
		info.ModelName = utils.NewErrorValue[string](utils.ErrNotCollectable)
		info.Mhz = utils.NewErrorValue[float64](utils.ErrNotCollectable)
	} else {
		if cpuInfos[0].ModelName != "" {
			info.ModelName = utils.NewValue(cpuInfos[0].ModelName)
		} else {
			info.ModelName = utils.NewErrorValue[string](utils.ErrNotCollectable)
		}
		if cpuInfos[0].Mhz > 0 {
			info.Mhz = utils.NewValue(cpuInfos[0].Mhz)
		} else {
			info.Mhz = utils.NewErrorValue[float64](utils.ErrNotCollectable)
		}
	}

	logical, err := gopsutilcpu.Counts(true)
	info.CPULogicalProcessors = utils.NewValueFrom(uint64(logical), err)

	physical, err := gopsutilcpu.Counts(false)
	info.CPUCores = utils.NewValueFrom(uint64(physical), err)

	return info
}

// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package memory

import (
	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	gopsutilmem "github.com/shirou/gopsutil/v4/mem"
)

func (info *Info) fillMemoryInfo() {
	vmem, err := gopsutilmem.VirtualMemory()
	if err == nil {
		info.TotalBytes = utils.NewValue(vmem.Total)
	} else {
		info.TotalBytes = utils.NewErrorValue[uint64](err)
	}

	swap, err := gopsutilmem.SwapMemory()
	if err == nil {
		info.SwapTotalKb = utils.NewValue(swap.Total / 1024)
	} else {
		info.SwapTotalKb = utils.NewErrorValue[uint64](err)
	}
}

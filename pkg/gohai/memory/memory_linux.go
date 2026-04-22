// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package memory

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/shirou/gopsutil/v4/mem"
)

func (info *Info) fillMemoryInfo() {
	v, err := mem.VirtualMemory()
	if err != nil {
		info.TotalBytes = utils.NewErrorValue[uint64](fmt.Errorf("could not get virtual memory info: %w", err))
	} else {
		info.TotalBytes = utils.NewValue(v.Total)
	}

	s, err := mem.SwapMemory()
	if err != nil {
		info.SwapTotalKb = utils.NewErrorValue[uint64](fmt.Errorf("could not get swap memory info: %w", err))
	} else {
		info.SwapTotalKb = utils.NewValue(s.Total / 1024)
	}
}

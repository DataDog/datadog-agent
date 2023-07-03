// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package memory

import (
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"golang.org/x/sys/unix"
)

func getTotalBytes() (uint64, error) {
	return unix.SysctlUint64("hw.memsize")
}

func getTotalSwapKb() (uint64, error) {
	// see struct xsw_usage defined in sys/sysctl.h
	type xswUsage struct {
		xsuTotal     uint64
		xsuAvail     uint64
		xsuUsed      uint64
		xsuPagesize  uint32
		xsuEncrypted bool
	}

	// sysctl returns an xsw_usage struct, so we use the raw variant
	// and then cast the result
	value, err := unix.SysctlRaw("vm.swapusage")
	if err != nil {
		return 0, err
	}

	xswSize := unsafe.Sizeof(xswUsage{})
	if uintptr(len(value)) != xswSize {
		return 0, fmt.Errorf("sysctl should return %d bytes but returned %d", xswSize, len(value))
	}

	xsw := (*xswUsage)(unsafe.Pointer(&value[0]))
	return xsw.xsuTotal / 1024, nil // xsuTotal is in bytes
}

func (info *Info) fillMemoryInfo() {
	info.TotalBytes = utils.NewValueFrom(getTotalBytes())
	info.SwapTotalKb = utils.NewValueFrom(getTotalSwapKb())
}

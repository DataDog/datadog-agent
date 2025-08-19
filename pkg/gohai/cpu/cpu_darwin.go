// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package cpu

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"golang.org/x/sys/unix"
)

func getSysctl[U any, V any](call func(string) (U, error), cast func(U) V, key string) utils.Value[V] {
	value, err := call(key)
	// sysctl returns ENOENT when the key doesn't exists on the device
	// eg. on ARM, the frequency and the family don't exist
	if errors.Is(err, unix.ENOENT) {
		err = utils.ErrNotCollectable
	}
	return utils.NewValueFrom(cast(value), err)
}

// type returned by sysctl is string, stored as string
func getSysctlString(key string) utils.Value[string] {
	castFun := func(val string) string { return val }
	return getSysctl(unix.Sysctl, castFun, key)
}

// type returned by sysctl is uint32, stored as string
func getSysctlInt32String(key string) utils.Value[string] {
	castFun := func(val uint32) string { return fmt.Sprintf("%d", val) }
	return getSysctl(unix.SysctlUint32, castFun, key)
}

// type returned by sysctl is uint32, stored as uint64
func getSysctlInt32Int64(key string) utils.Value[uint64] {
	castFun := func(val uint32) uint64 { return uint64(val) }
	return getSysctl(unix.SysctlUint32, castFun, key)
}

func getCPUInfo() *Info {
	cpuInfo := &Info{
		CacheSizeKB:      utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CPUPkgs:          utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CPUNumaNodes:     utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL1Bytes: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL2Bytes: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL3Bytes: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
	}

	// use `man 3 sysctl` to see the type returned when using each option

	cpuInfo.VendorID = getSysctlString("machdep.cpu.vendor")
	cpuInfo.ModelName = getSysctlString("machdep.cpu.brand_string")

	cpuInfo.Family = getSysctlInt32String("machdep.cpu.family")
	cpuInfo.Model = getSysctlInt32String("machdep.cpu.model")
	cpuInfo.Stepping = getSysctlInt32String("machdep.cpu.stepping")

	cpuInfo.CPUCores = getSysctlInt32Int64("hw.physicalcpu")
	cpuInfo.CPULogicalProcessors = getSysctlInt32Int64("hw.logicalcpu")

	// mhz is returned in hz but stored in mhz so we use a specific cast function
	mhzCast := func(value uint64) float64 {
		return float64(value) / 1000000
	}
	// unix.SysctlUint64 takes extra arguments so we have to use a wrapper
	sysctlUint64 := func(key string) (uint64, error) { return unix.SysctlUint64(key) }
	cpuInfo.Mhz = getSysctl(sysctlUint64, mhzCast, "hw.cpufrequency")

	return cpuInfo
}

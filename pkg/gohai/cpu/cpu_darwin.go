// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package cpu

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"golang.org/x/sys/unix"
)

func storeSysctl[U any, V any](call func(string) (U, error), cast func(U) V, field *utils.Value[V], key string) {
	value, err := call(key)
	// sysctl returns ENOENT when the key doesn't exists on the device
	// eg. on ARM, the frequency and the family don't exist
	if err == unix.ENOENT {
		err = utils.ErrNotCollectable
	}
	(*field) = utils.NewValueFrom(cast(value), err)
}

// type returned by sysctl is string, stored as string
func storeSysctlString(field *utils.Value[string], key string) {
	castFun := func(val string) string { return val }
	storeSysctl(unix.Sysctl, castFun, field, key)
}

// type returned by sysctl is uint32, stored as string
func storeSysctlInt32String(field *utils.Value[string], key string) {
	castFun := func(val uint32) string { return fmt.Sprintf("%d", val) }
	storeSysctl(unix.SysctlUint32, castFun, field, key)
}

// type returned by sysctl is uint32, stored as uint64
func storeSysctlInt32Int64(field *utils.Value[uint64], key string) {
	castFun := func(val uint32) uint64 { return uint64(val) }
	storeSysctl(unix.SysctlUint32, castFun, field, key)
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

	storeSysctlString(&cpuInfo.VendorID, "machdep.cpu.vendor")
	storeSysctlString(&cpuInfo.ModelName, "machdep.cpu.brand_string")

	storeSysctlInt32String(&cpuInfo.Family, "machdep.cpu.family")
	storeSysctlInt32String(&cpuInfo.Model, "machdep.cpu.model")
	storeSysctlInt32String(&cpuInfo.Stepping, "machdep.cpu.stepping")

	storeSysctlInt32Int64(&cpuInfo.CPUCores, "hw.physicalcpu")
	storeSysctlInt32Int64(&cpuInfo.CPULogicalProcessors, "hw.logicalcpu")

	// mhz is returned in hz but stored in mhz so we use a specific cast function
	mhzCast := func(value uint64) float64 {
		return float64(value) / 1000000
	}
	// unix.SysctlUint64 takes extra arguments so we have to use a wrapper
	sysctlUint64 := func(key string) (uint64, error) { return unix.SysctlUint64(key) }
	storeSysctl(sysctlUint64, mhzCast, &cpuInfo.Mhz, "hw.cpufrequency")

	return cpuInfo
}

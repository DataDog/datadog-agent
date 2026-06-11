// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package cpu

import (
	"errors"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"golang.org/x/sys/unix"
)

func getSysctl[U any, V any](call func(string) (U, error), cast func(U) V, key string) utils.Value[V] {
	value, err := call(key)
	// sysctl returns ENOENT when the key doesn't exist on the device
	// eg. on ARM, the frequency and the family don't exist
	if errors.Is(err, unix.ENOENT) {
		err = utils.ErrNotCollectable
	}
	return utils.NewValueFrom(cast(value), err)
}

// getSysctlOptional is like getSysctl but also treats EINVAL as ErrNotCollectable.
// Some sysctl keys exist in the MIB but return EINVAL on hardware that doesn't
// support the feature (e.g. hw.l3cachesize on Apple Silicon).
func getSysctlOptional[U any, V any](call func(string) (U, error), cast func(U) V, key string) utils.Value[V] {
	value, err := call(key)
	if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EINVAL) {
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
	castFun := func(val uint32) string { return strconv.FormatUint(uint64(val), 10) }
	return getSysctl(unix.SysctlUint32, castFun, key)
}

// type returned by sysctl is uint32, stored as uint64
func getSysctlInt32Int64(key string) utils.Value[uint64] {
	castFun := func(val uint32) uint64 { return uint64(val) }
	return getSysctl(unix.SysctlUint32, castFun, key)
}

// treats EINVAL as ErrNotCollectable because some keys (e.g. hw.l3cachesize) return EINVAL
// on hardware that lacks the feature
func getSysctlInt64(key string) utils.Value[uint64] {
	castFun := func(val uint64) uint64 { return val }
	// unix.SysctlUint64 takes extra arguments so we need a wrapper
	sysctlUint64 := func(k string) (uint64, error) { return unix.SysctlUint64(k) }
	return getSysctlOptional(sysctlUint64, castFun, key)
}

func getCPUInfo() *Info {
	cpuInfo := &Info{
		CacheSizeKB:  utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CPUNumaNodes: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
	}

	// use `man 3 sysctl` to see the type returned when using each option

	cpuInfo.VendorID = getSysctlString("machdep.cpu.vendor")
	cpuInfo.ModelName = getSysctlString("machdep.cpu.brand_string")

	cpuInfo.Family = getSysctlInt32String("machdep.cpu.family")
	// Apple Silicon: machdep.cpu.family doesn't exist; fall back to hw.cpufamily
	if cpuInfo.Family.Error() != nil {
		cpuInfo.Family = getSysctlInt32String("hw.cpufamily")
	}
	cpuInfo.Model = getSysctlInt32String("machdep.cpu.model")
	cpuInfo.Stepping = getSysctlInt32String("machdep.cpu.stepping")

	cpuInfo.CPUCores = getSysctlInt32Int64("hw.physicalcpu")
	cpuInfo.CPULogicalProcessors = getSysctlInt32Int64("hw.logicalcpu")

	// hw.packages is uint32; ENOENT on single-die systems maps to ErrNotCollectable via getSysctl
	cpuInfo.CPUPkgs = getSysctlInt32Int64("hw.packages")

	// cache sizes are uint64 bytes; ENOENT on Apple Silicon (no L3) maps to ErrNotCollectable
	cpuInfo.CacheSizeL1Bytes = getSysctlInt64("hw.l1dcachesize")
	cpuInfo.CacheSizeL2Bytes = getSysctlInt64("hw.l2cachesize")
	cpuInfo.CacheSizeL3Bytes = getSysctlInt64("hw.l3cachesize")

	// CacheSizeKB: sum of all available cache levels in KB (mirrors Linux ARM64 behavior)
	var totalCacheBytes uint64
	if l1, err := cpuInfo.CacheSizeL1Bytes.Value(); err == nil {
		totalCacheBytes += l1
	}
	if l2, err := cpuInfo.CacheSizeL2Bytes.Value(); err == nil {
		totalCacheBytes += l2
	}
	if l3, err := cpuInfo.CacheSizeL3Bytes.Value(); err == nil {
		totalCacheBytes += l3
	}
	if totalCacheBytes > 0 {
		cpuInfo.CacheSizeKB = utils.NewValue(totalCacheBytes / 1024)
	}

	// mhz is returned in hz but stored in mhz so we use a specific cast function
	mhzCast := func(value uint64) float64 {
		return float64(value) / 1000000
	}
	// unix.SysctlUint64 takes extra arguments so we have to use a wrapper
	sysctlUint64 := func(key string) (uint64, error) { return unix.SysctlUint64(key) }
	cpuInfo.Mhz = getSysctl(sysctlUint64, mhzCast, "hw.cpufrequency")

	return cpuInfo
}

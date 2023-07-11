// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package cpu

import (
	"fmt"

	"golang.org/x/sys/unix"
)

var cpuMapString = map[string]string{
	"machdep.cpu.vendor":       "vendor_id",
	"machdep.cpu.brand_string": "model_name",
}

var cpuMapInt32 = map[string]string{
	"machdep.cpu.family":   "family",
	"machdep.cpu.model":    "model",
	"machdep.cpu.stepping": "stepping",
	"hw.physicalcpu":       "cpu_cores",
	"hw.logicalcpu":        "cpu_logical_processors",
}

// use `man 3 sysctl` to see the type returned when using each option
func getCPUInfo() (map[string]string, error) {
	cpuInfo := make(map[string]string)

	// type returned by sysctl is string
	for option, key := range cpuMapString {
		if value, err := unix.Sysctl(option); err == nil {
			cpuInfo[key] = value
		}
	}

	// type returned by sysctl is int32
	for option, key := range cpuMapInt32 {
		if value, err := unix.SysctlUint32(option); err == nil {
			rendered := fmt.Sprintf("%d", value)
			cpuInfo[key] = rendered
		}
	}

	// type returned by sysctl is int64
	option := "hw.cpufrequency"
	if value, err := unix.SysctlUint64(option); err == nil {
		// the value is in Hz
		mhz := value / 1000000
		rendered := fmt.Sprintf("%d", mhz)
		cpuInfo["mhz"] = rendered
	}

	return cpuInfo, nil
}

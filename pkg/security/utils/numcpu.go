// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package utils holds utils related files
package utils

import (
	"github.com/shirou/gopsutil/v3/cpu"
)

// NumCPU returns the count of CPUs in the CPU affinity mask of the pid 1 process
func NumCPU() (int, error) {
	cpuInfos, err := cpu.Info()
	if err != nil {
		return 0, err
	}
	var count int32
	for _, inf := range cpuInfos {
		count += inf.Cores
	}
	return int(count), nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Portions of this code are taken from the gopsutil project
// https://github.com/shirou/gopsutil .  This code is licensed under the New BSD License
// copyright WAKAYAMA Shirou, and the gopsutil contributors

package utils

import (
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	hostinfoutils "github.com/DataDog/datadog-agent/pkg/util/hostinfo"
)

// Set the OS to "win32" instead of the runtime.GOOS of "windows" for the in app icon
const osName = "win32"

// osVersion this is a legacy representation of OS version dating back to agent V5 which was in Python. In V5 the
// content of this list changed based on the OS.
type osVersion [2]string

func getSystemStats() *systemStats {
	res, _ := cache.Get[*systemStats](
		systemStatsCacheKey,
		func() (*systemStats, error) {

			cpuInfo := cpu.CollectInfo()
			cores := cpuInfo.CPUCores.ValueOrDefault()
			c32 := int32(cores)
			modelName := cpuInfo.ModelName.ValueOrDefault()

			stats := &systemStats{
				Machine:   runtime.GOARCH,
				Platform:  runtime.GOOS,
				Processor: modelName,
				CPUCores:  c32,
				Pythonv:   python.GetPythonVersion(),
			}

			hostInfo := hostinfoutils.GetInformation()

			// osVersion is a legacy representation of OS version dating back to agent V5 which was in
			// Python2. In V5 the content of this list changed based on the OS:
			//
			// - Macver was the result of `platform.mac_ver()`
			// - Nixver was the result of `platform.dist()`
			// - Winver was the result of `platform.win32_ver()`
			// - Fbsdver was never used
			stats.Winver = osVersion{hostInfo.Platform, hostInfo.PlatformVersion}
			return stats, nil
		},
	)
	return res
}

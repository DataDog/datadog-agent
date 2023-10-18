// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package utils

import (
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const osName = runtime.GOOS

type osVersion [3]interface{}

// GetInformation returns an InfoStat object, filled in with various operating system metadata. This returns an empty
// host.InfoStat if gopsutil fails.
func GetInformation() *host.InfoStat {
	info, _ := cache.Get[*host.InfoStat](
		hostInfoCacheKey,
		func() (*host.InfoStat, error) {
			info, err := host.Info()
			if err != nil {
				log.Errorf("failed to retrieve host info: %s", err)
				return &host.InfoStat{}, err
			}
			return info, err
		})
	return info
}

func getSystemStats() *systemStats {
	res, _ := cache.Get[*systemStats](
		systemStatsCacheKey,
		func() (*systemStats, error) {
			var CPUModel string
			var CPUCores int32
			if cpuInfo, err := cpu.Info(); err != nil {
				log.Errorf("failed to retrieve cpu info: %s", err)
			} else {
				CPUModel = cpuInfo[0].ModelName
				CPUCores = cpuInfo[0].Cores
			}

			stats := &systemStats{
				Machine:   runtime.GOARCH,
				Platform:  runtime.GOOS,
				Processor: CPUModel,
				CPUCores:  CPUCores,
				Pythonv:   python.GetPythonVersion(),
			}

			hostInfo := GetInformation()

			// osVersion is a legacy representation of OS version dating back to agent V5 which was in
			// Python2. In V5 the content of this list changed based on the OS:
			//
			// - Macver was the result of `platform.mac_ver()`
			// - Nixver was the result of `platform.dist()`
			// - Winver was the result of `platform.win32_ver()`
			// - Fbsdver was never used
			stats.Macver = osVersion{"", "", ""}
			stats.Nixver = osVersion{"", "", ""}
			stats.Fbsdver = osVersion{"", "", ""}
			stats.Winver = osVersion{"", "", ""}

			if runtime.GOOS == "darwin" {
				stats.Macver = osVersion{hostInfo.PlatformVersion, [3]string{"", "", ""}, runtime.GOARCH}
			} else {
				stats.Nixver = osVersion{hostInfo.Platform, hostInfo.PlatformVersion, ""}
			}

			hostVersion := strings.Trim(hostInfo.Platform+" "+hostInfo.PlatformVersion, " ")
			inventories.SetHostMetadata(inventories.HostOSVersion, hostVersion)
			return stats, nil
		},
	)
	return res
}

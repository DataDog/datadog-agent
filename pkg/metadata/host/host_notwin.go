// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package host

import (
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"

	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/util/cache"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InitHostMetadata initializes necessary CPU info
func InitHostMetadata() error {
	var err error
	_, err = cpu.Info()

	return err

}

func getSystemStats() *systemStats {
	var stats *systemStats
	key := buildKey("systemStats")
	if x, found := cache.Cache.Get(key); found {
		stats = x.(*systemStats)
	} else {
		cpuInfo := getCPUInfo()
		hostInfo := getHostInfo()

		stats = &systemStats{
			Machine:   runtime.GOARCH,
			Platform:  runtime.GOOS,
			Processor: cpuInfo.ModelName,
			CPUCores:  cpuInfo.Cores,
			Pythonv:   strings.Split(GetPythonVersion(), " ")[0],
		}

		// fill the platform dependent bits of info
		fillOsVersion(stats, hostInfo)
		cache.Cache.Set(key, stats, cache.NoExpiration)
		inventories.SetHostMetadata(inventories.HostOSVersion, getOSVersion(hostInfo))
	}

	return stats
}

// getCPUInfo returns InfoStat for the first CPU gopsutil found
func getCPUInfo() *cpu.InfoStat {
	key := buildKey("cpuInfo")
	if x, found := cache.Cache.Get(key); found {
		return x.(*cpu.InfoStat)
	}

	i, err := cpu.Info()
	if err != nil {
		// don't cache and return zero value
		log.Errorf("failed to retrieve cpu info: %s", err)
		return &cpu.InfoStat{}
	}
	info := &i[0]
	cache.Cache.Set(key, info, cache.NoExpiration)
	return info
}

func getHostInfo() *host.InfoStat {
	key := buildKey("hostInfo")
	if x, found := cache.Cache.Get(key); found {
		return x.(*host.InfoStat)
	}

	info, err := host.Info()
	if err != nil {
		// don't cache and return zero value
		log.Errorf("failed to retrieve host info: %s", err)
		return &host.InfoStat{}
	}
	cache.Cache.Set(key, info, cache.NoExpiration)
	return info
}

// GetStatusInformation just returns an InfoStat object, we need some additional information that's not
func GetStatusInformation() *host.InfoStat {
	return getHostInfo()
}

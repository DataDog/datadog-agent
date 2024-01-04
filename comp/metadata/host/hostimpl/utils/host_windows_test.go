// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	"github.com/DataDog/datadog-agent/pkg/gohai/platform"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func TestGetHostInfo(t *testing.T) {
	defer cache.Cache.Delete(hostInfoCacheKey)

	info := GetInformation()
	pi := platform.CollectInfo()

	osHostname, _ := os.Hostname()
	assert.Equal(t, osHostname, info.Hostname)
	assert.NotNil(t, info.Uptime)
	assert.NotNil(t, info.BootTime)
	assert.NotZero(t, info.Procs)
	assert.Equal(t, runtime.GOOS, info.OS)
	assert.Equal(t, runtime.GOARCH, info.KernelArch)

	osValue, _ := pi.OS.Value()
	assert.Equal(t, osValue, info.Platform)
	assert.Equal(t, osValue, info.PlatformFamily)

	platformVersion, _ := winutil.GetWindowsBuildString()
	assert.Equal(t, platformVersion, info.PlatformVersion)
	assert.NotNil(t, info.HostID)
}

func TestGetHostInfoCache(t *testing.T) {
	defer cache.Cache.Delete(hostInfoCacheKey)

	fakeInfo := &InfoStat{Hostname: "hostname from cache"}
	cache.Cache.Set(hostInfoCacheKey, fakeInfo, cache.NoExpiration)

	assert.Equal(t, fakeInfo, GetInformation())
}

func TestGetSystemStats(t *testing.T) {
	defer cache.Cache.Delete(systemStatsCacheKey)

	cpuInfo := cpu.CollectInfo()
	ss := getSystemStats()

	assert.Equal(t, runtime.GOARCH, ss.Machine)
	assert.Equal(t, runtime.GOOS, ss.Platform)
	assert.Equal(t, cpuInfo.ModelName.ValueOrDefault(), ss.Processor)
	assert.Equal(t, int32(cpuInfo.CPUCores.ValueOrDefault()), ss.CPUCores)
	assert.Equal(t, python.GetPythonVersion(), ss.Pythonv)

	hostInfo := GetInformation()
	assert.Equal(t, osVersion{hostInfo.Platform, hostInfo.PlatformVersion}, ss.Winver)
}

func TestGetSystemStatsCache(t *testing.T) {
	defer cache.Cache.Delete(systemStatsCacheKey)

	fakeStats := &systemStats{Machine: "test data"}
	cache.Cache.Set(systemStatsCacheKey, fakeStats, cache.NoExpiration)

	assert.Equal(t, fakeStats, getSystemStats())
}

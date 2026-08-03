// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	hostinfoutils "github.com/DataDog/datadog-agent/pkg/util/hostinfo"
)

func TestGetSystemStats(t *testing.T) {
	defer cache.Cache.Delete(systemStatsCacheKey)

	cpuInfo := cpu.CollectInfo()
	ss := getSystemStats()

	assert.Equal(t, runtime.GOARCH, ss.Machine)
	assert.Equal(t, runtime.GOOS, ss.Platform)
	assert.Equal(t, cpuInfo.ModelName.ValueOrDefault(), ss.Processor)
	assert.Equal(t, int32(cpuInfo.CPUCores.ValueOrDefault()), ss.CPUCores)
	assert.Equal(t, python.GetPythonVersion(), ss.Pythonv)

	hostInfo := hostinfoutils.GetInformation()
	assert.Equal(t, osVersion{hostInfo.Platform, hostInfo.PlatformVersion}, ss.Winver)
}

func TestGetSystemStatsCache(t *testing.T) {
	defer cache.Cache.Delete(systemStatsCacheKey)

	fakeStats := &systemStats{Machine: "test data"}
	cache.Cache.Set(systemStatsCacheKey, fakeStats, cache.NoExpiration)

	assert.Equal(t, fakeStats, getSystemStats())
}

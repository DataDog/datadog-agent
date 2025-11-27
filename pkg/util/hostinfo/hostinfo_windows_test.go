// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//
// Package hostinfo helps collect relevant host information
//
//
// # Compatibility
//
// This module is exported and can be used outside of the datadog-agent
// repository, but is not designed as a general-purpose logging system.  Its
// API may change incompatibly.

package hostinfo

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

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

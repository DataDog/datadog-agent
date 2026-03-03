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

//go:build !windows

package hostinfo

import (
	"testing"

	"github.com/shirou/gopsutil/v4/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestGetHostInfo(t *testing.T) {
	defer cache.Cache.Delete(hostInfoCacheKey)

	info := GetInformation()
	expected, err := host.Info()
	require.NoError(t, err)
	// boot time can be computed dynamically using uptime on some platforms, in which case
	// there can be an off-by-one error
	assert.InDelta(t, expected.BootTime, info.BootTime, 1.5)
	assert.Equal(t, expected.HostID, info.HostID)
	assert.Equal(t, expected.Hostname, info.Hostname)
	assert.Equal(t, expected.KernelArch, info.KernelArch)
	assert.Equal(t, expected.KernelVersion, info.KernelVersion)
	assert.Equal(t, expected.OS, info.OS)
	assert.Equal(t, expected.Platform, info.Platform)
	assert.Equal(t, expected.PlatformFamily, info.PlatformFamily)
	assert.Equal(t, expected.PlatformVersion, info.PlatformVersion)
	// can't use assert.Equal since the fields Uptime and Procs can change
	assert.NotZero(t, info.Procs)
	assert.NotNil(t, info.Uptime)
	assert.Equal(t, expected.VirtualizationRole, info.VirtualizationRole)
	assert.Equal(t, expected.VirtualizationSystem, info.VirtualizationSystem)
}

func TestGetHostInfoCache(t *testing.T) {
	defer cache.Cache.Delete(hostInfoCacheKey)

	fakeInfo := &host.InfoStat{HostID: "test data"}
	cache.Cache.Set(hostInfoCacheKey, fakeInfo, cache.NoExpiration)

	assert.Equal(t, fakeInfo, GetInformation())
}

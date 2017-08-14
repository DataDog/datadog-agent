// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package host

import (
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPayload(t *testing.T) {
	p := GetPayload("myhostname")
	assert.NotEmpty(t, p.Os)
	assert.NotEmpty(t, p.PythonVersion)
	assert.Equal(t, "myhostname", p.InternalHostname)
	assert.NotEmpty(t, p.UUID)
	assert.NotNil(t, p.SystemStats)
	assert.NotNil(t, p.Meta)
	assert.NotNil(t, p.HostTags)
}

func TestGetSystemStats(t *testing.T) {
	assert.NotNil(t, getSystemStats())
	fakeStats := &systemStats{Machine: "fooMachine"}
	key := buildKey("systemStats")
	util.Cache.Set(key, fakeStats, util.NoExpiration)
	s := getSystemStats()
	assert.NotNil(t, s)
	assert.Equal(t, fakeStats.Machine, s.Machine)
}

func TestGetPythonVersion(t *testing.T) {
	require.Equal(t, "n/a", getPythonVersion())
	key := path.Join(util.AgentCachePrefix, "pythonVersion")
	util.Cache.Set(key, "Python 2.8", util.NoExpiration)
	require.Equal(t, "Python 2.8", getPythonVersion())
}

func TestGetCPUInfo(t *testing.T) {
	assert.NotNil(t, getCPUInfo())
	fakeInfo := &cpu.InfoStat{Cores: 42}
	key := buildKey("cpuInfo")
	util.Cache.Set(key, fakeInfo, util.NoExpiration)
	info := getCPUInfo()
	assert.Equal(t, int32(42), info.Cores)
}

func TestGetHostInfo(t *testing.T) {
	assert.NotNil(t, getHostInfo())
	fakeInfo := &host.InfoStat{HostID: "FOOBAR"}
	key := buildKey("hostInfo")
	util.Cache.Set(key, fakeInfo, util.NoExpiration)
	info := getHostInfo()
	assert.Equal(t, "FOOBAR", info.HostID)
}

func TestGetMeta(t *testing.T) {
	meta := getMeta()
	assert.NotEmpty(t, meta.SocketHostname)
	assert.NotEmpty(t, meta.Timezones)
	assert.NotEmpty(t, meta.SocketFqdn)
}

func TestGetHostTags(t *testing.T) {
	config.Datadog.Set("tags", []string{"tag1:value1", "tag2", "tag3"})
	defer config.Datadog.Set("tags", nil)

	hostTags := getHostTags()
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, hostTags.System, []string{"tag1:value1", "tag2", "tag3"})
}

func TestGetEmptyHostTags(t *testing.T) {
	// getHostTags should never return a nil value under System even when there are no host tags
	hostTags := getHostTags()
	assert.NotNil(t, hostTags.System)
	assert.Equal(t, hostTags.System, []string{})
}

func TestBuildKey(t *testing.T) {
	assert.Equal(t, "metadata/host/foo", buildKey("foo"))
}

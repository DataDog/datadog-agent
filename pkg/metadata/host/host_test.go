// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build !windows

package host

import (
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/metadata/host/container"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPayload(t *testing.T) {
	p := GetPayload(util.HostnameData{Hostname: "myhostname", Provider: ""})
	assert.NotEmpty(t, p.Os)
	assert.NotEmpty(t, p.AgentFlavor)
	assert.NotEmpty(t, p.PythonVersion)
	assert.NotNil(t, p.SystemStats)
	assert.NotNil(t, p.Meta)
	assert.NotNil(t, p.HostTags)
	assert.NotNil(t, p.InstallMethod)
}

func TestGetSystemStats(t *testing.T) {
	assert.NotNil(t, getSystemStats())
	fakeStats := &systemStats{Machine: "fooMachine"}
	key := buildKey("systemStats")
	cache.Cache.Set(key, fakeStats, cache.NoExpiration)
	s := getSystemStats()
	assert.NotNil(t, s)
	assert.Equal(t, fakeStats.Machine, s.Machine)
}

func TestGetPythonVersion(t *testing.T) {
	require.Equal(t, "n/a", GetPythonVersion())
	key := cache.BuildAgentKey("pythonVersion")
	cache.Cache.Set(key, "Python 2.8", cache.NoExpiration)
	require.Equal(t, "Python 2.8", GetPythonVersion())
}

func TestGetCPUInfo(t *testing.T) {
	assert.NotNil(t, getCPUInfo())
	fakeInfo := &cpu.InfoStat{Cores: 42}
	key := buildKey("cpuInfo")
	cache.Cache.Set(key, fakeInfo, cache.NoExpiration)
	info := getCPUInfo()
	assert.Equal(t, int32(42), info.Cores)
}

func TestGetHostInfo(t *testing.T) {
	assert.NotNil(t, getHostInfo())
	fakeInfo := &host.InfoStat{HostID: "FOOBAR"}
	key := buildKey("hostInfo")
	cache.Cache.Set(key, fakeInfo, cache.NoExpiration)
	info := getHostInfo()
	assert.Equal(t, "FOOBAR", info.HostID)
}

func TestGetMeta(t *testing.T) {
	meta := getMeta(util.HostnameData{})
	assert.NotEmpty(t, meta.SocketHostname)
	assert.NotEmpty(t, meta.Timezones)
	assert.NotEmpty(t, meta.SocketFqdn)
}

func TestBuildKey(t *testing.T) {
	assert.Equal(t, "metadata/host/foo", buildKey("foo"))
}

func TestGetContainerMeta(t *testing.T) {
	// reset catalog
	container.DefaultCatalog = make(container.Catalog)
	container.RegisterMetadataProvider("provider1", func() (map[string]string, error) { return map[string]string{"foo": "bar"}, nil })
	container.RegisterMetadataProvider("provider2", func() (map[string]string, error) { return map[string]string{"fizz": "buzz"}, nil })
	container.RegisterMetadataProvider("provider3", func() (map[string]string, error) { return map[string]string{"fizz": "buzz"}, nil })

	meta := getContainerMeta(50 * time.Millisecond)
	assert.Equal(t, map[string]string{"foo": "bar", "fizz": "buzz"}, meta)
}

func TestGetContainerMetaTimeout(t *testing.T) {
	// reset catalog
	container.DefaultCatalog = make(container.Catalog)
	container.RegisterMetadataProvider("provider1", func() (map[string]string, error) { return map[string]string{"foo": "bar"}, nil })
	container.RegisterMetadataProvider("provider2", func() (map[string]string, error) {
		time.Sleep(time.Second)
		return map[string]string{"fizz": "buzz"}, nil
	})

	meta := getContainerMeta(50 * time.Millisecond)
	assert.Equal(t, map[string]string{"foo": "bar"}, meta)
}

func TestGetLogsMeta(t *testing.T) {
	// No transport
	logs.CurrentTransport = ""
	meta := getLogsMeta()
	assert.Equal(t, &LogsMeta{Transport: ""}, meta)
	// TCP transport
	logs.CurrentTransport = logs.TransportTCP
	meta = getLogsMeta()
	assert.Equal(t, &LogsMeta{Transport: "TCP"}, meta)
	// HTTP transport
	logs.CurrentTransport = logs.TransportHTTP
	meta = getLogsMeta()
	assert.Equal(t, &LogsMeta{Transport: "HTTP"}, meta)
}

func TestGetInstallMethod(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_install_method")
	assert.Nil(t, err)
	defer os.RemoveAll(dir)

	installInfoPath := path.Join(dir, "install_info")

	// ------------- Without file, the install is considered private
	installMethod := getInstallMethod(installInfoPath)
	require.Equal(t, "undefined", installMethod.ToolVersion)
	assert.Nil(t, installMethod.Tool)
	assert.Nil(t, installMethod.InstallerVersion)

	// ------------- with a correct file
	var installInfoContent = `
---
install_method:
  tool_version: chef-15
  tool: chef
  installer_version: datadog-cookbook-4.2.1
`
	assert.Nil(t, ioutil.WriteFile(installInfoPath, []byte(installInfoContent), 0666))

	// the install is considered coming from chef (example)
	installMethod = getInstallMethod(installInfoPath)
	require.Equal(t, "chef-15", installMethod.ToolVersion)
	assert.NotNil(t, installMethod.Tool)
	require.Equal(t, "chef", *installMethod.Tool)
	assert.NotNil(t, installMethod.InstallerVersion)
	require.Equal(t, "datadog-cookbook-4.2.1", *installMethod.InstallerVersion)

	// ------------- with an incorrect file
	installInfoContent = `
---
install_methodlol:
  name: chef-15
  version: datadog-cookbook-4.2.1
`
	assert.Nil(t, ioutil.WriteFile(installInfoPath, []byte(installInfoContent), 0666))

	// the parsing does not occur and the install is kept undefined
	installMethod = getInstallMethod(installInfoPath)
	require.Equal(t, "undefined", installMethod.ToolVersion)
	assert.Nil(t, installMethod.Tool)
	assert.Nil(t, installMethod.InstallerVersion)
}

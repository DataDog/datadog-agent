// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package diskv2_test

import (
	"bufio"
	"bytes"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/diskv2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
)

func setupPlatformMocks() {
	diskv2.NetAddConnection = func(_localName, _remoteName, _password, _username string) error {
		return nil
	}
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenAllIOCountersMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "MonotonicCount", "system.disk.read_time", float64(300), "", []string{"device:/dev/sda1", "device_name:/dev/sda1"})
	m.AssertMetric(t, "MonotonicCount", "system.disk.write_time", float64(450), "", []string{"device:/dev/sda1", "device_name:/dev/sda1"})
	m.AssertMetric(t, "Rate", "system.disk.read_time_pct", float64(30), "", []string{"device:/dev/sda1", "device_name:/dev/sda1"})
	m.AssertMetric(t, "Rate", "system.disk.write_time_pct", float64(45), "", []string{"device:/dev/sda1", "device_name:/dev/sda1"})
	m.AssertMetric(t, "MonotonicCount", "system.disk.read_time", float64(500), "", []string{"device:/dev/sda2", "device_name:/dev/sda2"})
	m.AssertMetric(t, "MonotonicCount", "system.disk.write_time", float64(150), "", []string{"device:/dev/sda2", "device_name:/dev/sda2"})
	m.AssertMetric(t, "Rate", "system.disk.read_time_pct", float64(50), "", []string{"device:/dev/sda2", "device_name:/dev/sda2"})
	m.AssertMetric(t, "Rate", "system.disk.write_time_pct", float64(15), "", []string{"device:/dev/sda2", "device_name:/dev/sda2"})
}

func TestGivenADiskCheckWithCreateMountsConfigured_WhenCheckIsConfigured_ThenMountsAreCreated(t *testing.T) {
	setupDefaultMocks()
	var netAddConnectionCalls [][]string
	diskv2.NetAddConnection = func(localName, remoteName, password, username string) error {
		netAddConnectionCalls = append(netAddConnectionCalls, []string{localName, remoteName, password, username})
		return nil
	}
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
create_mounts:
- mountpoint: "p:"
  host: smbserver
  share: space
- mountpoint: "s:"
  user: auser
  password: "somepassword"
  host: smbserver
  share: space
  type: smb
- mountpoint: "n:"
  host: nfsserver
  share: /mnt/nfs_share
  type: nfs
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	assert.Equal(t, 3, len(netAddConnectionCalls))
	expectedNetAddConnectionCalls := [][]string{
		{`\\smbserver\space`, "p:", "", ""},
		{`\\smbserver\space`, "s:", "somepassword", "auser"},
		{`nfsserver:/mnt/nfs_share`, "n:", "", ""},
	}
	for i, mountCall := range netAddConnectionCalls {
		assert.Equal(t, expectedNetAddConnectionCalls[i], mountCall)
	}
}

func TestGivenADiskCheckWithCreateMountsConfiguredWithoutHost_WhenCheckIsConfigured_ThenMountsAreNotCreated(t *testing.T) {
	setupDefaultMocks()
	var netAddConnectionCalls [][]string
	diskv2.NetAddConnection = func(localName, remoteName, password, username string) error {
		netAddConnectionCalls = append(netAddConnectionCalls, []string{localName, remoteName, password, username})
		return nil
	}
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
create_mounts:
- mountpoint: "n:"
  share: /mnt/nfs_share
  type: nfs
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	err = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.Nil(t, err)
	assert.Equal(t, 0, len(netAddConnectionCalls))
	w.Flush()
	assert.Contains(t, b.String(), "Invalid configuration. Drive mount requires remote machine and share point")
}

func TestGivenADiskCheckWithCreateMountsConfigured_WhenCheckRunsAndIOCountersSystemCallReturnsError_ThenErrorMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskv2.NetAddConnection = func(_localName, _remoteName, _password, _username string) error {
		return errors.New("error calling NetAddConnection")
	}
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
create_mounts:
- mountpoint: "p:"
  host: smbserver
  share: space
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err = diskCheck.Run()

	assert.Nil(t, err)
	w.Flush()
	assert.Contains(t, b.String(), `Failed to mount p: on \\smbserver\space`)
}

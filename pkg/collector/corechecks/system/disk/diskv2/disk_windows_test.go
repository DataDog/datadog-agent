// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package diskv2_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"testing"

	gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/diskv2"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func setupPlatformMocks() {
	diskv2.NetAddConnection = func(_localName, _remoteName, _password, _username string) error {
		return nil
	}
}

func createWindowsCheck(t *testing.T) check.Check {
	cfg := configmock.New(t)
	cfg.Set("disk_check.use_core_loader", true, configmodel.SourceAgentRuntime)

	diskCheckOpt := diskv2.Factory()
	diskCheckFunc, _ := diskCheckOpt.Get()
	diskCheck := diskCheckFunc()
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskv2.WithDiskIOCounters(diskCheck, func(...string) (map[string]gopsutil_disk.IOCountersStat, error) {
		return map[string]gopsutil_disk.IOCountersStat{
			"\\\\?\\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\\": {
				Name:       "sda1",
				ReadCount:  100,
				WriteCount: 200,
				ReadBytes:  1048576,
				WriteBytes: 2097152,
				ReadTime:   300,
				WriteTime:  450,
			},
		}, nil
	}), func(_ string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:              "D:\\",
			Fstype:            "NTFS",
			Total:             100000000000, // 100 GB
			Free:              30000000000,  // 30 GB
			Used:              70000000000,  // 70 GB
			UsedPercent:       70.0,
			InodesTotal:       1000000,
			InodesUsed:        500000,
			InodesFree:        500000,
			InodesUsedPercent: 50.0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     `\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`,
				Mountpoint: "D:\\",
				Fstype:     "NTFS",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	})
	return diskCheck
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenAllUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithLowercaseDeviceTagConfigured_WhenCheckRuns_ThenLowercaseDevicesAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("lowercase_device_tag: true"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{`device:\\?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{`device:\\?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{`device:\\?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetricTaggedWith(t, "MonotonicCount", "system.disk.read_time", []string{`device:\\?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetricTaggedWith(t, "MonotonicCount", "system.disk.write_time", []string{`device:\\?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetricTaggedWith(t, "Rate", "system.disk.read_time_pct", []string{`device:\\?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetricTaggedWith(t, "Rate", "system.disk.write_time_pct", []string{`device:\\?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithIncludeAllDevicesTrueConfigured_WhenCheckRuns_ThenAllUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("include_all_devices: true"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithIncludeAllDevicesFalseConfigured_WhenCheckRuns_ThenOnlyPhysicalDevicesUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("include_all_devices: false"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRunsAndPartitionsSystemReturnsEmptyDevice_ThenNoUsageMetricsAreReportedForThatPartition(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "",
				Mountpoint: "D:\\",
				Fstype:     "NTFS",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "\\\\?\\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\\",
				Mountpoint: "E:\\",
				Fstype:     "NTFS",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{"device:", "device_name:."})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{"device:", "device_name:."})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{"device:", "device_name:."})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithDeviceIncludeConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseDevicesAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_include:
  - \\?\Vol.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithDeviceWhiteListConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseDevicesAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_whitelist:
  - \\?\Vol.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithFileSystemGlobalExcludeNotConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithIso9660FileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "cdrom",
				Mountpoint: "/",
				Fstype:     "iso9660",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "\\\\?\\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\\",
				Mountpoint: "E:\\",
				Fstype:     "NTFS",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{"device:cdrom", "device_name:cdrom"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{"device:cdrom", "device_name:cdrom"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{"device:cdrom", "device_name:cdrom"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithFileSystemGlobalExcludeNotConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithTracefsFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "trace",
				Mountpoint: "/",
				Fstype:     "tracefs",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "\\\\?\\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\\",
				Mountpoint: "E:\\",
				Fstype:     "NTFS",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{"device:trace", "device_name:trace"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{"device:trace", "device_name:trace"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{"device:trace", "device_name:trace"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenAllIOCountersMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "MonotonicCount", "system.disk.read_time", float64(300), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "MonotonicCount", "system.disk.write_time", float64(450), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Rate", "system.disk.read_time_pct", float64(30), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Rate", "system.disk.write_time_pct", float64(45), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithCreateMountsConfigured_WhenCheckIsConfigured_ThenMountsAreCreated(t *testing.T) {
	setupDefaultMocks()
	var netAddConnectionCalls [][]string
	diskv2.NetAddConnection = func(localName, remoteName, password, username string) error {
		netAddConnectionCalls = append(netAddConnectionCalls, []string{localName, remoteName, password, username})
		return nil
	}
	diskCheck := createWindowsCheck(t)
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
		{"p:", `\\smbserver\space`, "", ""},
		{"s:", `\\smbserver\space`, "somepassword", "auser"},
		{"n:", `nfsserver:/mnt/nfs_share`, "", ""},
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
	diskCheck := createWindowsCheck(t)
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
	diskCheck := createWindowsCheck(t)
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

func TestGivenADiskCheckWithDeviceTagReConfigured_WhenCheckRuns_ThenUsageMetricsAreReportedWithTheseTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_tag_re:
  \\\\\?\\Vol.*: role:primary
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`, "role:primary"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`, "role:primary"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`, "role:primary"})
}

func TestGivenADiskCheckWithFileSystemGlobalExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
file_system_global_exclude:
  - ntfs.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithFileSystemGlobalBlackListConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
file_system_global_blacklist:
  - ntfs.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithExcludedFileSystemsConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
excluded_filesystems:
  - ntfs.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithFileSystemExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_exclude:
  - ntfs.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithFileSystemBlackListConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_blacklist:
  - ntfs.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithFileSystemIncludeConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseFileSystemsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_include:
  - ntfs.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithFileSystemWhiteListConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseFileSystemsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_whitelist:
  - ntfs.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithMountPointGlobalExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
mount_point_global_exclude:
  - D:\\
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithMountPointGlobalBlackListConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
mount_point_global_blacklist:
  - D:\\
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithMountPointExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
mount_point_exclude:
  - D:\\
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithMountPointBlackListConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
mount_point_blacklist:
  - D:\\
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithExcludedMountPointReConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`excluded_mountpoint_re: D:\\`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithUseMountConfigured_WhenCheckRuns_ThenUsageMetricsAreReportedWithMountPointTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("use_mount: true"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{`device:D:\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{`device:D:\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{`device:D:\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`})
}

func TestGivenADiskCheckWithServiceCheckRwTrueConfigured_WhenCheckRuns_ThenReadWriteServiceCheckReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createWindowsCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
service_check_rw: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertServiceCheck(t, "disk.read_write", servicecheck.ServiceCheckOK, "", []string{`device:\\?\Volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}\`, `device_name:?\volume{a1b2c3d4-e5f6-7890-abcd-ef1234567890}`}, "")
}

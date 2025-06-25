// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diskv2_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/diskv2"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var partitionsTrue = []gopsutil_disk.PartitionStat{
	{
		Device:     "/dev/sda1",
		Mountpoint: "/",
		Fstype:     "ext4",
		Opts:       []string{"rw", "relatime"},
	},
	{
		Device:     "/dev/sda2",
		Mountpoint: "/home",
		Fstype:     "ext4",
		Opts:       []string{"rw", "relatime"},
	},
	{
		Device:     "/dev/sda6",
		Mountpoint: "/home/backup",
		Fstype:     "ext4",
		Opts:       []string{"rw", "relatime"},
	},
	{
		Device:     "tmpfs",
		Mountpoint: "/run",
		Fstype:     "tmpfs",
		Opts:       []string{"nosuid", "nodev", "relatime"},
	},
	{
		Device:     "shm",
		Mountpoint: "/dev/shm",
		Fstype:     "tmpfs",
		Opts:       []string{"ro", "nosuid", "nodev"},
	},
}
var partitionsFalse = []gopsutil_disk.PartitionStat{
	{
		Device:     "/dev/sda1",
		Mountpoint: "/",
		Fstype:     "ext4",
		Opts:       []string{"rw", "relatime"},
	},
	{
		Device:     "/dev/sda2",
		Mountpoint: "/home",
		Fstype:     "ext4",
		Opts:       []string{"rw", "relatime"},
	},
}
var usageData = map[string]*gopsutil_disk.UsageStat{
	"/": {
		Path:              "/",
		Fstype:            "ext4",
		Total:             100000000000, // 100 GB
		Free:              30000000000,  // 30 GB
		Used:              70000000000,  // 70 GB
		UsedPercent:       70.0,
		InodesTotal:       1000000,
		InodesUsed:        500000,
		InodesFree:        500000,
		InodesUsedPercent: 50.0,
	},
	"/home": {
		Path:              "/home",
		Fstype:            "ext4",
		Total:             50000000000, // 50 GB
		Free:              20000000000, // 20 GB
		Used:              30000000000, // 30 GB
		UsedPercent:       60.0,
		InodesTotal:       500000,
		InodesUsed:        200000,
		InodesFree:        300000,
		InodesUsedPercent: 40.0,
	},
	"/home/backup": {
		Path:              "/home/backup",
		Fstype:            "ext4",
		Total:             20000000000, // 20 GB
		Free:              10000000000, // 10 GB
		Used:              10000000000, // 10 GB
		UsedPercent:       60.0,
		InodesTotal:       500000,
		InodesUsed:        200000,
		InodesFree:        300000,
		InodesUsedPercent: 40.0,
	},
	"/run": {
		Path:              "/run",
		Fstype:            "tmpfs",
		Total:             2000000000, // 2 GB
		Free:              1500000000, // 1.5 GB
		Used:              500000000,  // 0.5 GB
		UsedPercent:       25.0,
		InodesTotal:       10000,
		InodesUsed:        5000,
		InodesFree:        5000,
		InodesUsedPercent: 50.0,
	},
	"/dev/shm": {
		Path:              "/dev/shm",
		Fstype:            "tmpfs",
		Total:             8000000000, // 8 GB
		Free:              7000000000, // 7 GB
		Used:              1000000000, // 1 GB
		UsedPercent:       12.5,
		InodesTotal:       20000,
		InodesUsed:        1000,
		InodesFree:        19000,
		InodesUsedPercent: 5.0,
	},
}
var ioCountersData = map[string]gopsutil_disk.IOCountersStat{
	"/dev/sda1": {
		Name:       "/dev/sda1",
		ReadCount:  100,
		WriteCount: 200,
		ReadBytes:  1048576,
		WriteBytes: 2097152,
		ReadTime:   300,
		WriteTime:  450,
	},
	"/dev/sda2": {
		Name:       "/dev/sda2",
		ReadCount:  50,
		WriteCount: 75,
		ReadBytes:  524288,
		WriteBytes: 1048576,
		ReadTime:   500,
		WriteTime:  150,
	},
}

func setupDefaultMocks() {
	setupPlatformMocks()
}

func createDiskCheck(t *testing.T) check.Check {
	cfg := configmock.New(t)
	cfg.Set("disk_check.use_core_loader", true, configmodel.SourceAgentRuntime)

	diskCheckOpt := diskv2.Factory()
	diskCheckFunc, _ := diskCheckOpt.Get()
	diskCheck := diskCheckFunc()
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskv2.WithDiskIOCounters(diskCheck, func(...string) (map[string]gopsutil_disk.IOCountersStat, error) {
		return ioCountersData, nil
	}), func(mountpoint string) (*gopsutil_disk.UsageStat, error) {
		return usageData[mountpoint], nil
	}), func(_ context.Context, all bool) ([]gopsutil_disk.PartitionStat, error) {
		if all {
			return partitionsTrue, nil
		}
		return partitionsFalse, nil
	})
	return diskCheck
}

type signalClock struct {
	clock.Clock
	afterCalled chan time.Time
}

func (sc *signalClock) After(d time.Duration) <-chan time.Time {
	ch := sc.Clock.After(d)
	// Signal that After has been called
	select {
	case sc.afterCalled <- time.Now():
	default:
	}
	return ch
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, []byte(`min_collection_interval: "string_value"`), nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckAndStoppedSender(t *testing.T) {
	stoppedSenderError := errors.New("demultiplexer is stopped")
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	m.GetSenderManager().(*aggregator.AgentDemultiplexer).Stop(false)
	err := diskCheck.Run()

	assert.Equal(t, stoppedSenderError, err)
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRunsAndPartitionsSystemCallReturnsError_ThenErrorIsReturnedAndNoUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return nil, errors.New("error calling disk.DiskPartitions")
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.NotNil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRunsAndUsageSystemCallReturnsError_ThenNoUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskUsage(diskCheck, func(_ string) (*gopsutil_disk.UsageStat, error) {
		return nil, errors.New("error calling diskUsage")
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err = diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))

	w.Flush()
	assert.Contains(t, b.String(), "Unable to get disk metrics for /: error calling diskUsage. You can exclude this mountpoint in the settings if it is invalid.")
	assert.Contains(t, b.String(), "Unable to get disk metrics for /home: error calling diskUsage. You can exclude this mountpoint in the settings if it is invalid.")
	assert.Contains(t, b.String(), "Unable to get disk metrics for /run: error calling diskUsage. You can exclude this mountpoint in the settings if it is invalid.")
	assert.Contains(t, b.String(), "Unable to get disk metrics for /dev/shm: error calling diskUsage. You can exclude this mountpoint in the settings if it is invalid.")

}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRunsAndIOCountersSystemCallReturnsError_ThenErrorIsReturnedAndNoUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskIOCounters(diskCheck, func(...string) (map[string]gopsutil_disk.IOCountersStat, error) {
		return nil, errors.New("error calling diskIOCounters")
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.NotNil(t, err)
	m.AssertNotCalled(t, "MonotonicCount", "system.disk.read_time", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "MonotonicCount", "system.disk.write_time", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Rate", "system.disk.read_time_pct", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Rate", "system.disk.write_time_pct", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

func TestGivenADiskCheckWithFileSystemGlobalBlackListConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
file_system_global_blacklist:
  - ext4
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`file_system_global_blacklist` is deprecated and will be removed in a future release. Please use `file_system_global_exclude` instead.")
}

func TestGivenADiskCheckWithDeviceGlobalBlackListConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
device_global_blacklist:
  - shm
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`device_global_blacklist` is deprecated and will be removed in a future release. Please use `device_global_exclude` instead.")
}

func TestGivenADiskCheckWithMountpointGlobalBlackListConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
mount_point_global_blacklist:
  - /dev/shm
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`mount_point_global_blacklist` is deprecated and will be removed in a future release. Please use `mount_point_global_exclude` instead.")
}

func TestGivenADiskCheckWithFileSystemWhiteListConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_whitelist:
  - ext4
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`file_system_whitelist` is deprecated and will be removed in a future release. Please use `file_system_include` instead.")
}

func TestGivenADiskCheckWithFileSystemBlackListConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_blacklist:
  - ext4
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`file_system_blacklist` is deprecated and will be removed in a future release. Please use `file_system_exclude` instead.")
}

func TestGivenADiskCheckWithDeviceWhiteListConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_whitelist:
  - ext4
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`device_whitelist` is deprecated and will be removed in a future release. Please use `device_include` instead.")
}

func TestGivenADiskCheckWithDeviceBlackListConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_blacklist:
  - ext4
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`device_blacklist` is deprecated and will be removed in a future release. Please use `device_exclude` instead.")
}

func TestGivenADiskCheckWithMountPointWhiteListConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
mount_point_whitelist:
  - ext4
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`mount_point_whitelist` is deprecated and will be removed in a future release. Please use `mount_point_include` instead.")
}

func TestGivenADiskCheckWithMountPointBlackListConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
mount_point_blacklist:
  - ext4
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`mount_point_blacklist` is deprecated and will be removed in a future release. Please use `mount_point_exclude` instead.")
}

func TestGivenADiskCheckWithExcludedMountPointReConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
excluded_mountpoint_re:
  - ext4
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`excluded_mountpoint_re` is deprecated and will be removed in a future release. Please use `mount_point_exclude` instead.")
}

func TestGivenADiskCheckWithExcludedFileSystemsConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
excluded_filesystems:
  - ext4
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`excluded_filesystems` is deprecated and will be removed in a future release. Please use `file_system_exclude` instead.")
}

func TestGivenADiskCheckWithExcludedDisksConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
excluded_disks:
  - ext4
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`excluded_disks` is deprecated and will be removed in a future release. Please use `device_exclude` instead.")
}

func TestGivenADiskCheckWithExcludedDisksReConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
excluded_disk_re:
  - ext4
`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")

	_ = diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	w.Flush()
	assert.Contains(t, b.String(), "`excluded_disk_re` is deprecated and will be removed in a future release. Please use `device_exclude` instead.")
}

func TestGivenADiskCheckWithDeviceGlobalExcludeAndDeviceExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseDevices(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
device_global_exclude:
  - /dev/sda1
`))
	config := integration.Data([]byte(`
device_exclude:
  - /dev/sda2
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, initConfig, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithDeviceGlobalBlackListAndDeviceExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseDevices(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
device_global_blacklist:
  - /dev/sda1
`))
	config := integration.Data([]byte(`
device_exclude:
  - /dev/sda2
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, initConfig, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithDeviceGlobalExcludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
device_global_exclude:
  - /dev/sda(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithDeviceExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseDevices(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_exclude:
  - /dev/sda.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithDeviceBlackListConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseDevices(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_blacklist:
  - /dev/sda.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithExcludedDisksConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseDevices(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
excluded_disks:
  - /dev/sda.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithExcludedDisksConfiguredWithDa2_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithSda2Devices(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "sda2",
				Mountpoint: "/home",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
excluded_disks:
  - da2
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:sda2", "device_name:sda2"})
}

func TestGivenADiskCheckWithDeviceExcludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_exclude:
  - /dev/sda(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithExcludedDiskReConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseDevices(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("excluded_disk_re: /dev/sda.*"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithExcludedDiskReIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("excluded_disk_re: /dev/sda(.*"))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithDeviceIncludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_include:
  - /dev/sda(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithFileSystemGlobalExcludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
file_system_global_exclude:
  - tmp(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithFileSystemExcludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_exclude:
  - tmp(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithFileSystemIncludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_include:
  - ext(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithMountPointGlobalExcludeNotConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithBinfmt_miscMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(_ string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:              "/host/proc/sys/fs/binfmt_misc",
			Fstype:            "ext4",
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
				Device:     "first",
				Mountpoint: "/host/proc/sys/fs/binfmt_misc",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "second",
				Mountpoint: "/proc/sys/fs/binfmt_misc",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:first", "device_name:first"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:first", "device_name:first"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:first", "device_name:first"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:second", "device_name:second"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:second", "device_name:second"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:second", "device_name:second"})
}

func TestGivenADiskCheckWithMountPointGlobalExcludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
mount_point_global_exclude:
  - /dev/shm(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithMountPointExcludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
mount_point_exclude:
  - /dev/(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithExcludedMountPointReIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("excluded_mountpoint_re: /dev/(.*"))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithMountPointIncludeConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseMountPointsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
mount_point_include:
  - /dev/.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithMountPointWhiteListConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseMountPointsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
mount_point_whitelist:
  - /dev/.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithMountPointIncludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
mount_point_include:
  - /dev/(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRunsAndUsageSystemCallReturnsAPartitionWithZeroTotal_ThenNoUsageMetricsAreReportedForThatPartition(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(_ string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:              "/dev/shm",
			Fstype:            "tmpfs",
			Total:             0,
			Free:              0,
			Used:              0,
			UsedPercent:       0,
			InodesTotal:       0,
			InodesUsed:        0,
			InodesFree:        0,
			InodesUsedPercent: 0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "shm",
				Mountpoint: "/dev/shm",
				Fstype:     "tmpfs",
				Opts:       []string{"rw", "nosuid", "nodev"},
			}}, nil
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

func TestGivenADiskCheckWithMinDiskSizeConfiguredTo1MiBConfig_WhenCheckRunsAndUsageSystemCallReturnsAPartitionWith1024Total_ThenNoUsageMetricsAreReportedForThatPartition(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(_ string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:              "/dev/shm",
			Fstype:            "tmpfs",
			Total:             1024,
			Free:              1024,
			Used:              0,
			UsedPercent:       0,
			InodesTotal:       0,
			InodesUsed:        0,
			InodesFree:        0,
			InodesUsedPercent: 0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "shm",
				Mountpoint: "/dev/shm",
				Fstype:     "tmpfs",
				Opts:       []string{"rw", "nosuid", "nodev"},
			}}, nil
	})

	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`min_disk_size: 1`))
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.InfoLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "info")

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err = diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))

	w.Flush()
	assert.Contains(t, b.String(), "Excluding partition: [device: shm] [mountpoint: /dev/shm] [fstype: tmpfs] with total disk size 1024 bytes")
}

func TestGivenADiskCheckWithMinDiskSizeConfiguredTo1MiBConfig_WhenCheckRunsAndUsageSystemCallReturnsAPartitionWith1048576Total_ThenUsageMetricsAreReportedForThatPartition(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(_ string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:              "/dev/shm",
			Fstype:            "tmpfs",
			Total:             1048576,
			Free:              1048576,
			Used:              0,
			UsedPercent:       0,
			InodesTotal:       0,
			InodesUsed:        0,
			InodesFree:        0,
			InodesUsedPercent: 0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "shm",
				Mountpoint: "/dev/shm",
				Fstype:     "tmpfs",
				Opts:       []string{"rw", "nosuid", "nodev"},
			}}, nil
	})

	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`min_disk_size: 1`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1024), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenUsageMetricsAreNotReportedWithFileSystemTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"tmpfs", "filesystem:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"tmpfs", "filesystem:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"tmpfs", "filesystem:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"tmpfs", "filesystem:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"tmpfs", "filesystem:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"tmpfs", "filesystem:tmpfs"})
}

func TestGivenADiskCheckWithTagByFileSystemFalseConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedWithFileSystemTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("tag_by_filesystem: false"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"tmpfs", "filesystem:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"tmpfs", "filesystem:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"tmpfs", "filesystem:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"tmpfs", "filesystem:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"tmpfs", "filesystem:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"tmpfs", "filesystem:tmpfs"})
}

func TestGivenADiskCheckWithTagByFileSystemTrueConfigured_WhenCheckRuns_ThenUsageMetricsAreReportedWithFileSystemTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("tag_by_filesystem: true"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"ext4", "filesystem:ext4"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"ext4", "filesystem:ext4"})
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenUsageMetricsAreNotReportedWithMountPointTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/", "device_name:sda1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/", "device_name:sda1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/", "device_name:sda1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/home", "device_name:sda2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/home", "device_name:sda2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/home", "device_name:sda2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/run", "device_name:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/run", "device_name:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/run", "device_name:tmpfs"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/shm", "device_name:shm"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/shm", "device_name:shm"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/shm", "device_name:shm"})
}

func TestGivenADiskCheckWithDeviceTagReIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_tag_re:
  /dev/sda.*: role:primary
  tmp.(*: role:tmp
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithUseLsblkAndBlkidCacheFileConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
use_lsblk: true
blkid_cache_file: /run/blkid/blkid.tab
`))
	expectedError := "only one of 'use_lsblk' and 'blkid_cache_file' can be set at the same time"

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.EqualError(t, err, expectedError)
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenReadWriteServiceCheckNotReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "ServiceCheck", "disk.read_write", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
}

func TestGivenADiskCheckWithServiceCheckRwFalseConfigured_WhenCheckRuns_ThenReadWriteServiceCheckNotReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
service_check_rw: false
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "ServiceCheck", "disk.read_write", mock.AnythingOfType("servicecheck.ServiceCheckStatus"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"), mock.AnythingOfType("string"))
}

func TestGivenADiskCheckWithDefaultConfig_WhenUsagePartitionTimeout_ThenUsageMetricsNotReported(t *testing.T) {
	setupDefaultMocks()
	mockClock := clock.NewMock()
	afterCalled := make(chan time.Time, 1)
	// Wrap your mockClock with the signaling clock
	testClock := &signalClock{
		Clock:       mockClock,
		afterCalled: afterCalled,
	}
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithClock(diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(_ string) (*gopsutil_disk.UsageStat, error) {
		// Sleep 10s (longer than default timeout)
		time.Sleep(10 * time.Second)
		return &gopsutil_disk.UsageStat{
			Path:              "/dev/shm",
			Fstype:            "tmpfs",
			Total:             1024,
			Free:              1024,
			Used:              0,
			UsedPercent:       0,
			InodesTotal:       0,
			InodesUsed:        0,
			InodesFree:        0,
			InodesUsedPercent: 0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "shm",
				Mountpoint: "/dev/shm",
				Fstype:     "tmpfs",
				Opts:       []string{"rw", "nosuid", "nodev"},
			}}, nil
	}), testClock)

	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	done := make(chan error, 1)
	go func() {
		done <- diskCheck.Run()
	}()
	// Explicitly wait until diskCheck.Run calls mockClock.After()
	<-afterCalled
	// Move the clock forward longer than default timeout (5s)
	mockClock.Add(5 * time.Second)
	err := <-done

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.disk.utilized", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.disk.in_use", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

func TestDiskCheckWithoutCoreLoader(t *testing.T) {
	flavor.SetTestFlavor(t, flavor.DefaultAgent)

	cfg := configmock.New(t)
	cfg.Set("disk_check.use_core_loader", false, configmodel.SourceAgentRuntime)

	diskFactory := diskv2.Factory()
	diskCheckFunc, ok := diskFactory.Get()
	require.True(t, ok)
	diskCheck := diskCheckFunc()

	mock := mocksender.NewMockSender(diskCheck.ID())
	err := diskCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	require.ErrorIs(t, err, check.ErrSkipCheckInstance)
}

func TestDiskCheckNonDefaultFlavor(t *testing.T) {
	for _, fl := range []string{flavor.IotAgent, flavor.ClusterAgent} {
		t.Run(fl, func(t *testing.T) {
			flavor.SetTestFlavor(t, fl)

			cfg := configmock.New(t)
			cfg.Set("disk_check.use_core_loader", false, configmodel.SourceAgentRuntime)

			diskFactory := diskv2.Factory()
			diskCheckFunc, ok := diskFactory.Get()
			require.True(t, ok)
			diskCheck := diskCheckFunc()

			mock := mocksender.NewMockSender(diskCheck.ID())
			err := diskCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
			require.NoError(t, err)
		})
	}
}

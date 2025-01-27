// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package disk

import (
	"bufio"
	"bytes"
	"errors"

	"testing"

	"github.com/shirou/gopsutil/v4/disk"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func setupDefaultMocks() {
	partitionsTrue := []disk.PartitionStat{
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
			Device:     "tmpfs",
			Mountpoint: "/run",
			Fstype:     "tmpfs",
			Opts:       []string{"rw", "nosuid", "nodev", "relatime"},
		},
		{
			Device:     "shm",
			Mountpoint: "/dev/shm",
			Fstype:     "tmpfs",
			Opts:       []string{"rw", "nosuid", "nodev"},
		},
	}
	partitionsFalse := []disk.PartitionStat{
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
	diskPartitions = func(all bool) ([]disk.PartitionStat, error) {
		if all {
			return partitionsTrue, nil
		}
		return partitionsFalse, nil
	}
	usageData := map[string]*disk.UsageStat{
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
	diskUsage = func(mountpoint string) (*disk.UsageStat, error) {
		return usageData[mountpoint], nil
	}
}

func createCheck() check.Check {
	diskCheckOpt := Factory()
	diskCheckFunc, _ := diskCheckOpt.Get()
	diskCheck := diskCheckFunc()
	return diskCheck
}
func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenAllUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithFileSystemGlobalBlackListConfigured_WhenCheckIsConfigured_ThenWarningMessagedIsLogged(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRunsAndPartitionsSystemCallReturnsError_ThenErrorIsReturnedAndNoUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskPartitions = func(_ bool) ([]disk.PartitionStat, error) {
		return nil, errors.New("error calling diskPartitions")
	}
	diskCheck := createCheck()
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
	diskUsage = func(_ string) (*disk.UsageStat, error) {
		return nil, errors.New("error calling diskUsage")
	}
	diskCheck := createCheck()
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
	assert.Contains(t, b.String(), "Unable to get disk metrics of / mount point")
	assert.Contains(t, b.String(), "Unable to get disk metrics of /home mount point")
	assert.Contains(t, b.String(), "Unable to get disk metrics of /run mount point")
	assert.Contains(t, b.String(), "Unable to get disk metrics of /dev/shm mount point")

}

func TestGivenADiskCheckWithIncludeAllDevicesTrueConfigured_WhenCheckRuns_ThenAllUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("include_all_devices: true"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithIncludeAllDevicesFalseConfigured_WhenCheckRuns_ThenOnlyPhysicalDevicesUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("include_all_devices: false"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRunsAndPartitionsSystemReturnsEmptyDevice_ThenNoUsageMetricsAreReportedForThatPartition(t *testing.T) {
	setupDefaultMocks()
	diskPartitions = func(_ bool) ([]disk.PartitionStat, error) {
		return []disk.PartitionStat{
			{
				Device:     "",
				Mountpoint: "/",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "/dev/sda2",
				Mountpoint: "/home",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	}
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:", "device_name:."})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:", "device_name:."})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:", "device_name:."})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
}

func TestGivenADiskCheckWithAllPartitionsFalseConfigured_WhenCheckRunsAndPartitionsSystemReturnsEmptyDevice_ThenNoUsageMetricsAreReportedForThatPartition(t *testing.T) {
	setupDefaultMocks()
	diskPartitions = func(_ bool) ([]disk.PartitionStat, error) {
		return []disk.PartitionStat{
			{
				Device:     "",
				Mountpoint: "/",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "/dev/sda2",
				Mountpoint: "/home",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	}
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("all_partitions: false"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:", "device_name:."})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:", "device_name:."})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:", "device_name:."})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
}

func TestGivenADiskCheckWithAllPartitionsTrueConfigured_WhenCheckRunsAndPartitionsSystemReturnsEmptyDevice_ThenUsageMetricsAreReportedForThatPartition(t *testing.T) {
	setupDefaultMocks()
	diskPartitions = func(_ bool) ([]disk.PartitionStat, error) {
		return []disk.PartitionStat{
			{
				Device:     "",
				Mountpoint: "/",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "/dev/sda2",
				Mountpoint: "/home",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	}
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("all_partitions: true"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:", "device_name:."})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:", "device_name:."})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:", "device_name:."})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
}

func TestGivenADiskCheckWithDeviceGlobalExcludeAndDeviceExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseDevices(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
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
	diskCheck := createCheck()
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
	diskCheck := createCheck()
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
	diskCheck := createCheck()
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
	diskCheck := createCheck()
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
	diskCheck := createCheck()
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

func TestGivenADiskCheckWithDeviceExcludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
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
	diskCheck := createCheck()
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
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("excluded_disk_re: /dev/sda(.*"))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithDeviceIncludeConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseDevicesAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_include:
  - /dev/sda.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithDeviceWhiteListConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseDevicesAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_whitelist:
  - /dev/sda.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithDeviceIncludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_include:
  - /dev/sda(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithFileSystemGlobalExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
file_system_global_exclude:
  - tmpfs
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithFileSystemGlobalBlackListConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
file_system_global_blacklist:
  - tmpfs
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithFileSystemGlobalExcludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
file_system_global_exclude:
  - tmp(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithFileSystemExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_exclude:
  - tmp.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithFileSystemBlackListConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_blacklist:
  - tmp.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithExcludedFileSystemsConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
excluded_filesystems:
  - tmp.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithFileSystemExcludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_exclude:
  - tmp(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithFileSystemIncludeConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseFileSystemsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_include:
  - ext.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithFileSystemWhiteListConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseFileSystemsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_whitelist:
  - ext.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithFileSystemIncludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
file_system_include:
  - ext(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithMountPointGlobalExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
mount_point_global_exclude:
  - /dev/shm
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithMountPointGlobalBlackListConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
mount_point_global_blacklist:
  - /dev/shm
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithMountPointGlobalExcludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createCheck()
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
mount_point_global_exclude:
  - /dev/shm(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, initConfig, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithMountPointExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
mount_point_exclude:
  - /dev/.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithMountPointBlackListConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
mount_point_blacklist:
  - /dev/.*
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithMountPointExcludeIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
mount_point_exclude:
  - /dev/(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithExcludedMountPointReConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("excluded_mountpoint_re: /dev/.*"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(1953125), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(488281.25), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(1464843.75), "", []string{"device:tmpfs", "device_name:tmpfs"})
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(7812500), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(976562.5), "", []string{"device:shm", "device_name:shm"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(6835937.5), "", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithExcludedMountPointReIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("excluded_mountpoint_re: /dev/(.*"))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithMountPointIncludeConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseMountPointsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
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
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	diskPartitions = func(_ bool) ([]disk.PartitionStat, error) {
		return []disk.PartitionStat{
			{
				Device:     "shm",
				Mountpoint: "/dev/shm",
				Fstype:     "tmpfs",
				Opts:       []string{"rw", "nosuid", "nodev"},
			}}, nil
	}
	diskUsage = func(_ string) (*disk.UsageStat, error) {
		return &disk.UsageStat{
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
	}

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

func TestGivenADiskCheckWithMinDiskSizeConfiguredTo100Config_WhenCheckRunsAndUsageSystemCallReturnsAPartitionWith10Total_ThenNoUsageMetricsAreReportedForThatPartition(t *testing.T) {
	setupDefaultMocks()
	diskCheck := new(Check)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	diskPartitions = func(_ bool) ([]disk.PartitionStat, error) {
		return []disk.PartitionStat{
			{
				Device:     "shm",
				Mountpoint: "/dev/shm",
				Fstype:     "tmpfs",
				Opts:       []string{"rw", "nosuid", "nodev"},
			}}, nil
	}
	diskUsage = func(_ string) (*disk.UsageStat, error) {
		return &disk.UsageStat{
			Path:              "/dev/shm",
			Fstype:            "tmpfs",
			Total:             10,
			Free:              10,
			Used:              0,
			UsedPercent:       0,
			InodesTotal:       0,
			InodesUsed:        0,
			InodesFree:        0,
			InodesUsedPercent: 0,
		}, nil
	}
	config := integration.Data([]byte(`min_disk_size: 100`))
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
	assert.Contains(t, b.String(), "Excluding partition: [device: shm] [mountpoint: /dev/shm] [fstype: tmpfs] with total disk size 10")
}

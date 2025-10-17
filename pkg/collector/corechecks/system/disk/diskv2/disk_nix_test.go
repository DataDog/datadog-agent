// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package diskv2_test

import (
	"context"
	"errors"
	"os"
	"testing"

	gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/diskv2"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

const LsblkData = string(`
sda1 MYLABEL1
sda2 MYLABEL2

sda3
`)
const blkidCacheData = string(`
<device DEVNO="0x0801" LABEL="MYLABEL" UUID="1234-5678" TYPE="ext4">/dev/sda1</device>
<device DEVNO="0x0802" LABEL="BACKUP" UUID="8765-4321" TYPE="ext4">/dev/sda2</device>
<device DEVNO="0x0811" LABEL="USB_DRIVE" UUID="abcd-efgh" TYPE="vfat">/dev/sdb1</device>
<device DEVNO="0x0812" LABEL="DATA_DISK" UUID="ijkl-mnop" TYPE="ntfs">/dev/sdb2</device>
`)
const blkidData = string(`
/dev/sda1: UUID="abc-123" LABEL="MYLABEL1"
/dev/sda2: UUID=\"def-456\" LABEL="MYLABEL2"
/dev/sda3: UUID=\"def-789\"
/dev/sda6: UUID="abc-321" LABEL=

/dev/sda4:
/dev/sda5
`)

func setupPlatformMocks() {
	diskv2.LsblkCommand = func() (string, error) {
		return LsblkData, nil
	}
	diskv2.BlkidCacheCommand = func(_blkidCacheFile string) (string, error) {
		return blkidCacheData, nil
	}
	diskv2.BlkidCommand = func() (string, error) {
		return blkidData, nil
	}
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenAllUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
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

func TestGivenADiskCheckWithLowercaseDeviceTagConfigured_WhenCheckRuns_ThenLowercaseDevicesAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskIOCounters(diskCheck, func(...string) (map[string]gopsutil_disk.IOCountersStat, error) {
		return map[string]gopsutil_disk.IOCountersStat{
			"/dev/SDA1": {
				Name:       "sda1",
				ReadCount:  100,
				WriteCount: 200,
				ReadBytes:  1048576,
				WriteBytes: 2097152,
				ReadTime:   300,
				WriteTime:  450,
			},
			"/dev/SDA2": {
				Name:       "sda2",
				ReadCount:  50,
				WriteCount: 75,
				ReadBytes:  524288,
				WriteBytes: 1048576,
				ReadTime:   500,
				WriteTime:  150,
			},
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "/dev/SDA1",
				Mountpoint: "/home",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	})

	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("lowercase_device_tag: true"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:SDA1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:SDA1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:SDA1"})
	m.AssertMetricTaggedWith(t, "MonotonicCount", "system.disk.read_time", []string{"device:/dev/sda1", "device_name:SDA1"})
	m.AssertMetricTaggedWith(t, "MonotonicCount", "system.disk.write_time", []string{"device:/dev/sda1", "device_name:SDA1"})
	m.AssertMetricTaggedWith(t, "Rate", "system.disk.read_time_pct", []string{"device:/dev/sda1", "device_name:SDA1"})
	m.AssertMetricTaggedWith(t, "Rate", "system.disk.write_time_pct", []string{"device:/dev/sda1", "device_name:SDA1"})
}

func TestGivenADiskCheckWithIncludeAllDevicesTrueConfigured_WhenCheckRuns_ThenAllUsageMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
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
	diskCheck := createDiskCheck(t)
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
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
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
	})

	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", []string{"device:", "device_name:."})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", mock.AnythingOfType("float64"), "", []string{"device:", "device_name:."})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", mock.AnythingOfType("float64"), "", []string{"device:", "device_name:."})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
}

func TestGivenADiskCheckWithAllPartitionsFalseConfigured_WhenCheckRunsAndPartitionsSystemReturnsEmptyDevice_ThenNoUsageMetricsAreReportedForThatPartition(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
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
	})

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
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
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
	})

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

func TestGivenADiskCheckWithDeviceIncludeConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseDevicesAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
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
	diskCheck := createDiskCheck(t)
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

func TestGivenADiskCheckWithFileSystemGlobalExcludeNotConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithIso9660FileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "cdrom",
				Mountpoint: "/",
				Fstype:     "iso9660",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "/dev/sda2",
				Mountpoint: "/home",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:cdrom", "device_name:cdrom"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:cdrom", "device_name:cdrom"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:cdrom", "device_name:cdrom"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
}

func TestGivenADiskCheckWithFileSystemGlobalExcludeNotConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithTracefsFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "trace",
				Mountpoint: "/",
				Fstype:     "tracefs",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "/dev/sda2",
				Mountpoint: "/home",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertNotCalled(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:trace", "device_name:trace"})
	m.AssertNotCalled(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:trace", "device_name:trace"})
	m.AssertNotCalled(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:trace", "device_name:trace"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
}

func TestGivenADiskCheckWithFileSystemGlobalExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
file_system_global_exclude:
  - tmp.*
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
	diskCheck := createDiskCheck(t)
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

func TestGivenADiskCheckWithExcludedFileSystemsConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
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

func TestGivenADiskCheckWithExcludedFileSystemsIncorrectlyConfigured_WhenCheckIsConfigured_ThenErrorIsReturned(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	initConfig := integration.Data([]byte(`
excluded_filesystems:
  - tmpfs(.*
`))

	err := diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, initConfig, nil, "test")

	assert.NotNil(t, err)
}

func TestGivenADiskCheckWithExcludedFileSystemsConfiguredWithTmpfs_WhenCheckRuns_ThenUsageMetricsAreReportedForPartitionsWithDevTmpfsFileSystem(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "devtmpfs",
				Mountpoint: "/",
				Fstype:     "devtmpfs",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "/dev/sda2",
				Mountpoint: "/home",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
excluded_filesystems:
  - tmpfs
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:devtmpfs", "device_name:devtmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:devtmpfs", "device_name:devtmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:devtmpfs", "device_name:devtmpfs"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
}

func TestGivenADiskCheckWithFileSystemExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseFileSystems(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
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
	diskCheck := createDiskCheck(t)
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

func TestGivenADiskCheckWithFileSystemIncludeConfigured_WhenCheckRuns_ThenOnlyUsageMetricsForPartitionsWithThoseFileSystemsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
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
	diskCheck := createDiskCheck(t)
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

func TestGivenADiskCheckWithDeviceTagReConfigured_WhenCheckRuns_ThenUsageMetricsAreReportedWithTheseTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
device_tag_re:
  /dev/sda.*: role:primary
  tmp.*: role:tmp
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1", "role:primary"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:sda1", "role:primary"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:sda1", "role:primary"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda2", "device_name:sda2", "role:primary"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda2", "device_name:sda2", "role:primary"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda2", "device_name:sda2", "role:primary"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:tmpfs", "device_name:tmpfs", "role:tmp"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:tmpfs", "device_name:tmpfs", "role:tmp"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:tmpfs", "device_name:tmpfs", "role:tmp"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:shm", "device_name:shm"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:shm", "device_name:shm"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:shm", "device_name:shm"})
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenAllIOCountersMetricsAreReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetric(t, "MonotonicCount", "system.disk.read_time", float64(300), "", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetric(t, "MonotonicCount", "system.disk.write_time", float64(450), "", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetric(t, "Rate", "system.disk.read_time_pct", float64(30), "", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetric(t, "Rate", "system.disk.write_time_pct", float64(45), "", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetric(t, "MonotonicCount", "system.disk.read_time", float64(500), "", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetric(t, "MonotonicCount", "system.disk.write_time", float64(150), "", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetric(t, "Rate", "system.disk.read_time_pct", float64(50), "", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetric(t, "Rate", "system.disk.write_time_pct", float64(15), "", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithTagByLabelConfiguredFalse_WhenCheckRuns_ThenBlkidLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
tag_by_label: false
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenBlkidLabelsAreReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenEmptyBlkidLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda6", "device_name:sda6", "label:", "device_label:"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda6", "device_name:sda6", "label:", "device_label:"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda6", "device_name:sda6", "label:", "device_label:"})
}

func TestGivenADiskCheckWithTagByLabelConfiguredTrue_WhenCheckRuns_ThenBlkidLabelsAreReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
tag_by_label: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithTagByLabelConfiguredTrue_WhenCheckRunsAndBlkidReturnsError_ThenBlkidLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskv2.BlkidCommand = func() (string, error) {
		return "", errors.New("error calling blkid")
	}
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
tag_by_label: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithUseLsblkConfiguredTrue_WhenCheckRuns_ThenLsblkLabelsAreReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
use_lsblk: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithTagByLabelConfiguredTrueAndUseLsblk_WhenCheckRuns_ThenLsblkLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
tag_by_label: true
use_lsblk: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda2", "device_name:sda2", "label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithTagByLabelConfiguredFalseAndUseLsblk_WhenCheckRuns_ThenLsblkLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
tag_by_label: false
use_lsblk: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithUseLsblkConfiguredTrue_WhenLsblkReturnsError_ThenLsblkLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskv2.LsblkCommand = func() (string, error) {
		return "", errors.New("error calling lsblk")
	}
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
use_lsblk: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL1", "device_label:MYLABEL1"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL2", "device_label:MYLABEL2"})
}

func TestGivenADiskCheckWithBlkidCacheFileConfigured_WhenCheckRuns_ThenBlkidCacheFileLabelsAreReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	var actualBlkidCacheFile string
	diskv2.BlkidCacheCommand = func(blkidCacheFile string) (string, error) {
		actualBlkidCacheFile = blkidCacheFile
		return blkidCacheData, nil
	}
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
blkid_cache_file: /run/blkid/blkid.tab
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	assert.Equal(t, "/run/blkid/blkid.tab", actualBlkidCacheFile)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda2", "device_name:sda2", "label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda2", "device_name:sda2", "label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda2", "device_name:sda2", "label:BACKUP", "device_label:BACKUP"})
}

func TestGivenADiskCheckWithBlkidCacheFileConfigured_WhenBlkidCacheReturnsError_ThenBlkidCacheLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	var actualBlkidCacheFile string
	diskv2.BlkidCacheCommand = func(blkidCacheFile string) (string, error) {
		actualBlkidCacheFile = blkidCacheFile
		return "", errors.New("error calling blkid cache")
	}
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
blkid_cache_file: /run/blkid/blkid.tab
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	assert.Equal(t, "/run/blkid/blkid.tab", actualBlkidCacheFile)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:BACKUP", "device_label:BACKUP"})
}

func TestGivenADiskCheckWithBlkidCacheFileConfigured_WhenBlkidCacheHasWrongLines_ThenBlkidCacheLabelsAreNotReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	var actualBlkidCacheFile string
	diskv2.BlkidCacheCommand = func(blkidCacheFile string) (string, error) {
		actualBlkidCacheFile = blkidCacheFile
		return string(`
<device DEVNO="0x0801" LABEL="MYLABEL" UUID="1234-5678" TYPE="ext4">/dev/sda1</device>

<device DEVNO="0x0802" LABEL="BACKUP" UUID="8765-4321" TYPE="ext4">/dev/sda2</device
<device DEVNO="0x0811" LABEL="USB_DRIVE" UUID="abcd-efgh" TYPE="vfat">/dev/sdb1</device>
<device DEVNO="0x0812" LABEL="DATA_DISK" UUID="ijkl-mnop" TYPE="ntfs">/dev/sdb2</device>
`), nil
	}
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
blkid_cache_file: /run/blkid/blkid.tab
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	assert.Equal(t, "/run/blkid/blkid.tab", actualBlkidCacheFile)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda1", "device_name:sda1", "label:MYLABEL", "device_label:MYLABEL"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"label:BACKUP", "device_label:BACKUP"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"label:BACKUP", "device_label:BACKUP"})
}

func TestGivenADiskCheckWithMountPointGlobalExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
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
	diskCheck := createDiskCheck(t)
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

func TestGivenADiskCheckWithMountPointExcludeConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
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
	diskCheck := createDiskCheck(t)
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

func TestGivenADiskCheckWithExcludedMountPointReConfigured_WhenCheckRuns_ThenUsageMetricsAreNotReportedForPartitionsWithThoseMountPoints(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
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

func TestGivenADiskCheckWithUseMountConfigured_WhenCheckRuns_ThenUsageMetricsAreReportedWithMountPointTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte("use_mount: true"))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/", "device_name:sda1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/", "device_name:sda1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/", "device_name:sda1"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/home", "device_name:sda2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/home", "device_name:sda2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/home", "device_name:sda2"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/run", "device_name:tmpfs"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/run", "device_name:tmpfs"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/run", "device_name:tmpfs"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/shm", "device_name:shm"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/shm", "device_name:shm"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/shm", "device_name:shm"})
}

func TestGivenADiskCheckWithServiceCheckRwTrueConfigured_WhenCheckRuns_ThenReadWriteServiceCheckReported(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
service_check_rw: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertServiceCheck(t, "disk.read_write", servicecheck.ServiceCheckOK, "", []string{"device:/dev/sda1", "device_name:sda1"}, "")
	m.AssertServiceCheck(t, "disk.read_write", servicecheck.ServiceCheckOK, "", []string{"device:/dev/sda2", "device_name:sda2"}, "")
	m.AssertServiceCheck(t, "disk.read_write", servicecheck.ServiceCheckUnknown, "", []string{"device:tmpfs", "device_name:tmpfs"}, "")
	m.AssertServiceCheck(t, "disk.read_write", servicecheck.ServiceCheckCritical, "", []string{"device:shm", "device_name:shm"}, "")
}

type symlinkFs struct {
	afero.Fs
	links map[string]string
}

func newSymlinkFs(base afero.Fs) *symlinkFs {
	return &symlinkFs{
		Fs:    base,
		links: make(map[string]string),
	}
}

// SymlinkIfPossible records the link in our map.
func (s *symlinkFs) SymlinkIfPossible(oldname, newname string) error {
	s.links[newname] = oldname
	return nil
}

// ReadlinkIfPossible satisfies afero.Symlinker.
func (s *symlinkFs) ReadlinkIfPossible(name string) (string, error) {
	if target, ok := s.links[name]; ok {
		return target, nil
	}
	return "", os.ErrInvalid
}

func TestResolveRootDeviceFlagTrue(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "/dev/sda1",
				Mountpoint: "/home",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	})
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
resolve_root_device: true
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1"})
}

func TestResolveRootDeviceFlagFalse(t *testing.T) {
	fakeStatFn := func(_ string) (diskv2.StatT, error) {
		return diskv2.StatT{Major: 8, Minor: 1}, nil
	}
	base := afero.NewMemMapFs()
	fs := newSymlinkFs(base)
	_ = fs.MkdirAll("/sys/dev/block/8:1", 0755)
	_ = fs.SymlinkIfPossible("/dev/sda1", "/sys/dev/block/8:1")
	// ensure the /dev directory exists
	assert.NoError(t, fs.MkdirAll("/dev", 0755))
	// create an empty file at /dev/sda1
	assert.NoError(t, afero.WriteFile(fs, "/dev/sda1", []byte{}, 0644))
	err := afero.WriteFile(fs, "/sys/dev/block/8:1/uevent", []byte(
		`MAJOR=8
MINOR=1
DEVNAME=sda1
DEVPATH=/devices/pci0000:00/0000:00:17.0/ata1/host0/target0:0:0/0:0:0:0/block/sda/sda1
SUBSYSTEM=block
UEVENT_SEQNUM=42
`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/proc/self/mountinfo", []byte(
		`36 35 8:1 / / rw,relatime - ext4 /dev/root rw,errors=continue
50 25 0:31 / /mnt/strange\\040name rw,relatime - ext4 /dev/sdd1 rw
`),
		0644)
	assert.Nil(t, err)
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithFs(diskv2.WithStat(diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "/dev/sda1",
				Mountpoint: "/home",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			}}, nil
	}), fakeStatFn), fs)
	m := mocksender.NewMockSender(diskCheck.ID())
	m.SetupAcceptAll()
	config := integration.Data([]byte(`
resolve_root_device: false
`))

	diskCheck.Configure(m.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")
	err = diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1"})
}

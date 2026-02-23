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
	"slices"
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
	m := configureCheck(t, diskCheck, nil, nil)
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

	config := integration.Data([]byte("lowercase_device_tag: true"))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte("include_all_devices: true"))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte("include_all_devices: false"))
	m := configureCheck(t, diskCheck, config, nil)
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

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRunsAndPartitionsSystemCallReturnsPartialResultsWithError_ThenMetricsAreReportedForReturnedPartitions(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	// Simulate partial success: return some partitions along with an error
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(mountpoint string) (*gopsutil_disk.UsageStat, error) {
		return usageData[mountpoint], nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "/dev/sda1",
				Mountpoint: "/",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			},
		}, errors.New("error reading some partitions")
	})
	m := configureCheck(t, diskCheck, nil, nil)
	err := diskCheck.Run()

	// No error returned, and metrics are reported for the partitions that were returned
	assert.Nil(t, err)
	// usageData["/"] has Total=100GB, Used=70GB, Free=30GB - metrics are in kB (divided by 1024)
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
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

	m := configureCheck(t, diskCheck, nil, nil)
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

	config := integration.Data([]byte("all_partitions: false"))
	m := configureCheck(t, diskCheck, config, nil)
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

	config := integration.Data([]byte("all_partitions: true"))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
device_include:
  - /dev/sda.*
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
device_whitelist:
  - /dev/sda.*
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	m := configureCheck(t, diskCheck, nil, nil)
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
	m := configureCheck(t, diskCheck, nil, nil)
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
	initConfig := integration.Data([]byte(`
file_system_global_exclude:
  - tmp.*
`))
	m := configureCheck(t, diskCheck, nil, initConfig)
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
	initConfig := integration.Data([]byte(`
file_system_global_blacklist:
  - tmpfs
`))
	m := configureCheck(t, diskCheck, nil, initConfig)
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
	config := integration.Data([]byte(`
excluded_filesystems:
  - tmp.*
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	initConfig := integration.Data([]byte(`
excluded_filesystems:
  - tmpfs(.*
`))

	err := diskCheck.Configure(senderManager, integration.FakeConfigHash, initConfig, nil, "test")

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
	config := integration.Data([]byte(`
excluded_filesystems:
  - tmpfs
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
file_system_exclude:
  - tmp.*
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
file_system_blacklist:
  - tmp.*
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
file_system_include:
  - ext.*
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
file_system_whitelist:
  - ext.*
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
device_tag_re:
  /dev/sda.*: role:primary
  tmp.*: role:tmp
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	m := configureCheck(t, diskCheck, nil, nil)
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
	config := integration.Data([]byte(`
tag_by_label: false
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	m := configureCheck(t, diskCheck, nil, nil)
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
	m := configureCheck(t, diskCheck, nil, nil)
	err := diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda6", "device_name:sda6", "label:", "device_label:"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/sda6", "device_name:sda6", "label:", "device_label:"})
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/sda6", "device_name:sda6", "label:", "device_label:"})
}

func TestGivenADiskCheckWithTagByLabelConfiguredTrue_WhenCheckRuns_ThenBlkidLabelsAreReportedAsTags(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	config := integration.Data([]byte(`
tag_by_label: true
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
tag_by_label: true
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
use_lsblk: true
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
tag_by_label: true
use_lsblk: true
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
tag_by_label: false
use_lsblk: true
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
use_lsblk: true
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
blkid_cache_file: /run/blkid/blkid.tab
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
blkid_cache_file: /run/blkid/blkid.tab
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
blkid_cache_file: /run/blkid/blkid.tab
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	initConfig := integration.Data([]byte(`
mount_point_global_exclude:
  - /dev/shm
`))
	m := configureCheck(t, diskCheck, nil, initConfig)
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
	initConfig := integration.Data([]byte(`
mount_point_global_blacklist:
  - /dev/shm
`))
	m := configureCheck(t, diskCheck, nil, initConfig)
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
	config := integration.Data([]byte(`
mount_point_exclude:
  - /dev/.*
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
mount_point_blacklist:
  - /dev/.*
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte("excluded_mountpoint_re: /dev/.*"))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte("use_mount: true"))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
service_check_rw: true
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
resolve_root_device: true
`))
	m := configureCheck(t, diskCheck, config, nil)
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
	config := integration.Data([]byte(`
resolve_root_device: false
`))
	m := configureCheck(t, diskCheck, config, nil)
	err = diskCheck.Run()

	assert.Nil(t, err)
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1"})
}

// TestDeviceMapperResolution tests that device-mapper devices (dm-X) are resolved
// to their friendly /dev/mapper/* names for tagging (matching Python psutil behavior)
func TestDeviceMapperResolution_WhenGopsutilReturnsDmDevice_ThenMetricsHaveFriendlyMapperName(t *testing.T) {
	fakeStatFn := func(_ string) (diskv2.StatT, error) {
		return diskv2.StatT{Major: 252, Minor: 0}, nil
	}
	base := afero.NewMemMapFs()
	fs := newSymlinkFs(base)
	// Setup /dev/mapper/ocivolume-root -> ../dm-0 symlink
	assert.NoError(t, fs.MkdirAll("/dev/mapper", 0755))
	assert.NoError(t, fs.MkdirAll("/dev", 0755))
	// The symlink is relative (as it typically is in Linux)
	assert.NoError(t, fs.SymlinkIfPossible("../dm-0", "/dev/mapper/ocivolume-root"))
	// Also create the dm-0 device file
	assert.NoError(t, afero.WriteFile(fs, "/dev/dm-0", []byte{}, 0644))
	// Create mountinfo with /dev/mapper/ocivolume-root
	err := afero.WriteFile(fs, "/proc/self/mountinfo", []byte(
		`103 1 252:0 / / rw,relatime shared:1 - xfs /dev/mapper/ocivolume-root rw,attr2,inode64,logbufs=8,logbsize=32k,noquota
104 1 252:1 / /oled rw,relatime shared:2 - xfs /dev/mapper/ocivolume-oled rw,attr2,inode64,logbufs=8,logbsize=32k,noquota
`),
		0644)
	assert.Nil(t, err)
	// Setup symlink for ocivolume-oled as well
	assert.NoError(t, fs.SymlinkIfPossible("../dm-1", "/dev/mapper/ocivolume-oled"))
	assert.NoError(t, afero.WriteFile(fs, "/dev/dm-1", []byte{}, 0644))

	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	// gopsutil returns /dev/dm-0 (after resolving the symlink internally)
	diskCheck = diskv2.WithGOOS(diskv2.WithFs(diskv2.WithStat(diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(mountpoint string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:        mountpoint,
			Fstype:      "xfs",
			Total:       100000000000,
			Free:        30000000000,
			Used:        70000000000,
			UsedPercent: 70.0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				// gopsutil resolves /dev/mapper/ocivolume-root to /dev/dm-0
				Device:     "/dev/dm-0",
				Mountpoint: "/",
				Fstype:     "xfs",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "/dev/dm-1",
				Mountpoint: "/oled",
				Fstype:     "xfs",
				Opts:       []string{"rw", "relatime"},
			},
		}, nil
	}), fakeStatFn), fs), "linux")

	m := configureCheck(t, diskCheck, nil, nil)
	err = diskCheck.Run()

	assert.Nil(t, err)
	// Metrics should have the friendly /dev/mapper/ocivolume-root name, not /dev/dm-0
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/mapper/ocivolume-root", "device_name:ocivolume-root"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.used", []string{"device:/dev/mapper/ocivolume-root", "device_name:ocivolume-root"})
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.free", []string{"device:/dev/mapper/ocivolume-root", "device_name:ocivolume-root"})
	// Second device-mapper device should also be resolved
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/mapper/ocivolume-oled", "device_name:ocivolume-oled"})
}

// TestDeviceMapperResolution_WhenSymlinkResolutionFails_ThenMetricsHaveOriginalDmName
// tests that when we can't resolve the symlink, we fall back to the original device name
func TestDeviceMapperResolution_WhenSymlinkResolutionFails_ThenMetricsHaveOriginalDmName(t *testing.T) {
	fakeStatFn := func(_ string) (diskv2.StatT, error) {
		return diskv2.StatT{Major: 252, Minor: 0}, nil
	}
	base := afero.NewMemMapFs()
	fs := newSymlinkFs(base)
	assert.NoError(t, fs.MkdirAll("/dev/mapper", 0755))
	// Create mountinfo with /dev/mapper/ocivolume-root but NO symlink
	// (symlink resolution will fail)
	err := afero.WriteFile(fs, "/proc/self/mountinfo", []byte(
		`103 1 252:0 / / rw,relatime shared:1 - xfs /dev/mapper/ocivolume-root rw,attr2,inode64,logbufs=8,logbsize=32k,noquota
`),
		0644)
	assert.Nil(t, err)

	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithGOOS(diskv2.WithFs(diskv2.WithStat(diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(mountpoint string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:        mountpoint,
			Fstype:      "xfs",
			Total:       100000000000,
			Free:        30000000000,
			Used:        70000000000,
			UsedPercent: 70.0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "/dev/dm-0",
				Mountpoint: "/",
				Fstype:     "xfs",
				Opts:       []string{"rw", "relatime"},
			},
		}, nil
	}), fakeStatFn), fs), "linux")

	m := configureCheck(t, diskCheck, nil, nil)
	err = diskCheck.Run()

	assert.Nil(t, err)
	// When symlink resolution fails, metrics should have the original dm-X name
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/dm-0", "device_name:dm-0"})
}

// TestDeviceMapperResolution_WithLowercaseDeviceTag verifies that lowercase_device_tag
// works correctly with device-mapper resolution
func TestDeviceMapperResolution_WithLowercaseDeviceTag(t *testing.T) {
	fakeStatFn := func(_ string) (diskv2.StatT, error) {
		return diskv2.StatT{Major: 252, Minor: 0}, nil
	}
	base := afero.NewMemMapFs()
	fs := newSymlinkFs(base)
	assert.NoError(t, fs.MkdirAll("/dev/mapper", 0755))
	assert.NoError(t, fs.MkdirAll("/dev", 0755))
	assert.NoError(t, fs.SymlinkIfPossible("../dm-0", "/dev/mapper/OCIVolume-Root"))
	assert.NoError(t, afero.WriteFile(fs, "/dev/dm-0", []byte{}, 0644))
	err := afero.WriteFile(fs, "/proc/self/mountinfo", []byte(
		`103 1 252:0 / / rw,relatime shared:1 - xfs /dev/mapper/OCIVolume-Root rw,attr2,inode64
`),
		0644)
	assert.Nil(t, err)

	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithGOOS(diskv2.WithFs(diskv2.WithStat(diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(mountpoint string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:        mountpoint,
			Fstype:      "xfs",
			Total:       100000000000,
			Free:        30000000000,
			Used:        70000000000,
			UsedPercent: 70.0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "/dev/dm-0",
				Mountpoint: "/",
				Fstype:     "xfs",
				Opts:       []string{"rw", "relatime"},
			},
		}, nil
	}), fakeStatFn), fs), "linux")

	config := integration.Data([]byte("lowercase_device_tag: true"))
	m := configureCheck(t, diskCheck, config, nil)
	err = diskCheck.Run()

	assert.Nil(t, err)
	// Device tag should be lowercase, device_name should preserve original case
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/mapper/ocivolume-root", "device_name:OCIVolume-Root"})
}

// TestDeviceMapperResolution_WithMixedDeviceTypes verifies that regular devices
// and device-mapper devices work together correctly
func TestDeviceMapperResolution_WithMixedDeviceTypes(t *testing.T) {
	fakeStatFn := func(_ string) (diskv2.StatT, error) {
		return diskv2.StatT{Major: 8, Minor: 1}, nil
	}
	base := afero.NewMemMapFs()
	fs := newSymlinkFs(base)
	assert.NoError(t, fs.MkdirAll("/dev/mapper", 0755))
	assert.NoError(t, fs.MkdirAll("/dev", 0755))
	assert.NoError(t, fs.SymlinkIfPossible("../dm-0", "/dev/mapper/vg-lv"))
	assert.NoError(t, afero.WriteFile(fs, "/dev/dm-0", []byte{}, 0644))
	assert.NoError(t, afero.WriteFile(fs, "/dev/sda1", []byte{}, 0644))
	err := afero.WriteFile(fs, "/proc/self/mountinfo", []byte(
		`103 1 252:0 / / rw,relatime shared:1 - xfs /dev/mapper/vg-lv rw
104 1 8:1 / /boot rw,relatime shared:2 - ext4 /dev/sda1 rw
`),
		0644)
	assert.Nil(t, err)

	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithGOOS(diskv2.WithFs(diskv2.WithStat(diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(mountpoint string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:        mountpoint,
			Fstype:      "xfs",
			Total:       100000000000,
			Free:        30000000000,
			Used:        70000000000,
			UsedPercent: 70.0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "/dev/dm-0", // gopsutil resolves mapper symlink
				Mountpoint: "/",
				Fstype:     "xfs",
				Opts:       []string{"rw", "relatime"},
			},
			{
				Device:     "/dev/sda1", // regular device stays as-is
				Mountpoint: "/boot",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			},
		}, nil
	}), fakeStatFn), fs), "linux")

	m := configureCheck(t, diskCheck, nil, nil)
	err = diskCheck.Run()

	assert.Nil(t, err)
	// Device-mapper device should be resolved to friendly name
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/mapper/vg-lv", "device_name:vg-lv"})
	// Regular device should keep its original name
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1"})
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRuns_ThenInodeMetricsAreReported(t *testing.T) {
	// This tests that inode metrics (system.fs.inodes.*) are sent on Linux
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(path string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:              path,
			Fstype:            "ext4",
			Total:             100000000000,
			Free:              30000000000,
			Used:              70000000000,
			UsedPercent:       70.0,
			InodesTotal:       1000000,
			InodesUsed:        400000,
			InodesFree:        600000,
			InodesUsedPercent: 40.0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "/dev/sda1",
				Mountpoint: "/",
				Fstype:     "ext4",
				Opts:       []string{"rw"},
			},
		}, nil
	})
	m := configureCheck(t, diskCheck, nil, nil)
	err := diskCheck.Run()

	assert.Nil(t, err)
	// Verify inode metrics are sent
	m.AssertMetric(t, "Gauge", "system.fs.inodes.total", float64(1000000), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.fs.inodes.used", float64(400000), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.fs.inodes.free", float64(600000), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.fs.inodes.utilized", float64(40.0), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.fs.inodes.in_use", float64(0.4), "", []string{"device:/dev/sda1", "device_name:sda1"})
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRunsAndInodesIsZero_ThenInodeMetricsAreNotReported(t *testing.T) {
	// When InodesTotal is 0, inode metrics should not be sent (e.g., some filesystems don't support inodes)
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(path string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:              path,
			Fstype:            "tmpfs",
			Total:             100000000,
			Free:              50000000,
			Used:              50000000,
			UsedPercent:       50.0,
			InodesTotal:       0, // No inodes
			InodesUsed:        0,
			InodesFree:        0,
			InodesUsedPercent: 0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "tmpfs",
				Mountpoint: "/tmp",
				Fstype:     "tmpfs",
				Opts:       []string{"rw"},
			},
		}, nil
	})
	m := configureCheck(t, diskCheck, nil, nil)
	err := diskCheck.Run()

	assert.Nil(t, err)
	// Disk metrics should still be reported
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:tmpfs", "device_name:tmpfs"})
	// Inode metrics should NOT be reported when InodesTotal is 0
	m.AssertNotCalled(t, "Gauge", "system.fs.inodes.total", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.fs.inodes.used", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.fs.inodes.free", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.fs.inodes.utilized", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "Gauge", "system.fs.inodes.in_use", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRunsAndPartitionsSystemReturnsNoneDevice_ThenNoUsageMetricsAreReportedForThatPartition(t *testing.T) {
	// The code treats device == "none" the same as empty device (excluded by default unless all_partitions: true)
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskCheck, func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "none", // Special "none" device (e.g., some virtual filesystems)
				Mountpoint: "/sys/fs/cgroup",
				Fstype:     "cgroup2",
				Opts:       []string{"rw", "nosuid", "nodev", "noexec"},
			},
			{
				Device:     "/dev/sda1",
				Mountpoint: "/",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			},
		}, nil
	})

	m := configureCheck(t, diskCheck, nil, nil)
	err := diskCheck.Run()

	assert.Nil(t, err)
	// Partition with device "none" should be excluded
	m.AssertNotCalled(t, "Gauge", "system.disk.total", mock.AnythingOfType("float64"), "", mock.MatchedBy(func(tags []string) bool {
		return slices.Contains(tags, "device:none")
	}))
	// Regular partition should still be reported
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1", "device_name:sda1"})
}

func TestGivenADiskCheckWithAllPartitionsTrueConfigured_WhenCheckRunsAndPartitionsSystemReturnsNoneDevice_ThenUsageMetricsAreReportedForThatPartition(t *testing.T) {
	// With all_partitions: true, even "none" devices should be included
	// Note: We need a non-zero Total size, otherwise it's filtered out by the min_disk_size logic
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(path string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:        path,
			Fstype:      "tmpfs",
			Total:       100000000, // Non-zero to avoid min_disk_size filtering
			Free:        50000000,
			Used:        50000000,
			UsedPercent: 50.0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "none",
				Mountpoint: "/run/user/1000",
				Fstype:     "tmpfs",
				Opts:       []string{"rw"},
			},
		}, nil
	})

	config := integration.Data([]byte("all_partitions: true"))
	m := configureCheck(t, diskCheck, config, nil)
	err := diskCheck.Run()

	assert.Nil(t, err)
	// With all_partitions: true, the "none" device should be included
	// The device tag keeps the original value "none"
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:none", "device_name:none"})
}

func TestGivenADiskCheckWithResolveRootDeviceFalse_WhenMountinfoDoesNotExist_ThenFallbackToMountsFileAndDevRootNotResolved(t *testing.T) {
	// Tests the fallback from /proc/self/mountinfo to /proc/self/mounts
	// When mountinfo doesn't exist, the code should fall back to mounts file.
	// KEY DIFFERENCE: When using mounts file (not mountinfo), the /dev/root
	// resolution logic is SKIPPED, so /dev/root stays as-is in the device tag.
	fakeStatFn := func(_ string) (diskv2.StatT, error) {
		return diskv2.StatT{Major: 8, Minor: 1}, nil
	}
	base := afero.NewMemMapFs()
	fs := newSymlinkFs(base)

	// Create /proc/self directory but NO mountinfo file
	assert.NoError(t, fs.MkdirAll("/proc/self", 0755))
	// Also create /proc/1 without mountinfo to ensure fallback happens
	assert.NoError(t, fs.MkdirAll("/proc/1", 0755))

	// Create mounts file (simpler format than mountinfo)
	// Format: device mountpoint fstype options dump pass
	err := afero.WriteFile(fs, "/proc/self/mounts", []byte(
		`/dev/root / ext4 rw,relatime 0 0
`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/proc/1/mounts", []byte(
		`/dev/root / ext4 rw,relatime 0 0
`),
		0644)
	assert.Nil(t, err)

	// Setup uevent file that would normally be used to resolve /dev/root
	// (but it WON'T be used when falling back to mounts)
	assert.NoError(t, fs.MkdirAll("/sys/dev/block/8:1", 0755))
	err = afero.WriteFile(fs, "/sys/dev/block/8:1/uevent", []byte(
		`MAJOR=8
MINOR=1
DEVNAME=sda1
`),
		0644)
	assert.Nil(t, err)

	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	// gopsutil returns /dev/root (as it would on some systems)
	diskCheck = diskv2.WithGOOS(diskv2.WithFs(diskv2.WithStat(diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(mountpoint string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:        mountpoint,
			Fstype:      "ext4",
			Total:       100000000000,
			Free:        30000000000,
			Used:        70000000000,
			UsedPercent: 70.0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "/dev/root", // This would be resolved to /dev/sda1 with mountinfo
				Mountpoint: "/",
				Fstype:     "ext4",
				Opts:       []string{"rw", "relatime"},
			},
		}, nil
	}), fakeStatFn), fs), "linux")

	config := integration.Data([]byte(`
resolve_root_device: false
`))
	m := configureCheck(t, diskCheck, config, nil)
	err = diskCheck.Run()

	// Check should succeed even when falling back to mounts file
	assert.Nil(t, err)
	// KEY ASSERTION: /dev/root is NOT resolved to /dev/sda1 because we're using
	// the mounts fallback (not mountinfo). This proves the fallback happened!
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/root", "device_name:root"})
	// Verify it's NOT using the resolved name
	m.AssertMetricNotTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/sda1"})
}

func TestGivenADiskCheckWithResolveRootDeviceFalse_WhenProc1MountinfoFails_ThenFallbackToProcSelfMountinfo(t *testing.T) {
	// Tests the fallback from /proc/1/mountinfo to /proc/self/mountinfo
	// When /proc/1/mountinfo doesn't exist but /proc/self/mountinfo does,
	// the code should use /proc/self/mountinfo for device-mapper resolution
	fakeStatFn := func(_ string) (diskv2.StatT, error) {
		return diskv2.StatT{Major: 252, Minor: 0}, nil
	}
	base := afero.NewMemMapFs()
	fs := newSymlinkFs(base)

	// Create /proc/1 directory but NO mountinfo file there
	assert.NoError(t, fs.MkdirAll("/proc/1", 0755))
	// Create /proc/self/mountinfo (the fallback)
	assert.NoError(t, fs.MkdirAll("/proc/self", 0755))
	err := afero.WriteFile(fs, "/proc/self/mountinfo", []byte(
		`103 1 252:0 / / rw,relatime shared:1 - xfs /dev/mapper/vg-root rw
`),
		0644)
	assert.Nil(t, err)

	// Setup device-mapper symlink
	assert.NoError(t, fs.MkdirAll("/dev/mapper", 0755))
	assert.NoError(t, fs.SymlinkIfPossible("../dm-0", "/dev/mapper/vg-root"))
	assert.NoError(t, afero.WriteFile(fs, "/dev/dm-0", []byte{}, 0644))

	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	// gopsutil returns /dev/dm-0 (resolved symlink)
	diskCheck = diskv2.WithGOOS(diskv2.WithFs(diskv2.WithStat(diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(mountpoint string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:        mountpoint,
			Fstype:      "xfs",
			Total:       100000000000,
			Free:        30000000000,
			Used:        70000000000,
			UsedPercent: 70.0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "/dev/dm-0", // gopsutil resolves symlink
				Mountpoint: "/",
				Fstype:     "xfs",
				Opts:       []string{"rw", "relatime"},
			},
		}, nil
	}), fakeStatFn), fs), "linux")

	config := integration.Data([]byte(`
resolve_root_device: false
`))
	m := configureCheck(t, diskCheck, config, nil)
	err = diskCheck.Run()

	assert.Nil(t, err)
	// KEY ASSERTION: Device-mapper resolution worked using /proc/self/mountinfo fallback
	// /dev/dm-0 should be resolved to friendly /dev/mapper/vg-root name
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/mapper/vg-root", "device_name:vg-root"})
}

func TestGivenADiskCheckWithProcMountinfoPathConfigured_WhenCheckRuns_ThenCustomPathIsUsed(t *testing.T) {
	// Tests that proc_mountinfo_path configuration overrides the default path
	// This is useful when running in containers where host's /proc is mounted elsewhere
	fakeStatFn := func(_ string) (diskv2.StatT, error) {
		return diskv2.StatT{Major: 252, Minor: 0}, nil
	}
	base := afero.NewMemMapFs()
	fs := newSymlinkFs(base)

	// Create custom mountinfo path (simulating container environment)
	assert.NoError(t, fs.MkdirAll("/host/proc/1", 0755))
	err := afero.WriteFile(fs, "/host/proc/1/mountinfo", []byte(
		`103 1 252:0 / / rw,relatime shared:1 - xfs /dev/mapper/host-root rw
`),
		0644)
	assert.Nil(t, err)

	// Setup device-mapper symlink
	assert.NoError(t, fs.MkdirAll("/dev/mapper", 0755))
	assert.NoError(t, fs.SymlinkIfPossible("../dm-0", "/dev/mapper/host-root"))
	assert.NoError(t, afero.WriteFile(fs, "/dev/dm-0", []byte{}, 0644))

	// Do NOT create /proc/1/mountinfo or /proc/self/mountinfo
	// The check should use the custom path instead

	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithGOOS(diskv2.WithFs(diskv2.WithStat(diskv2.WithDiskPartitionsWithContext(diskv2.WithDiskUsage(diskCheck, func(mountpoint string) (*gopsutil_disk.UsageStat, error) {
		return &gopsutil_disk.UsageStat{
			Path:        mountpoint,
			Fstype:      "xfs",
			Total:       100000000000,
			Free:        30000000000,
			Used:        70000000000,
			UsedPercent: 70.0,
		}, nil
	}), func(_ context.Context, _ bool) ([]gopsutil_disk.PartitionStat, error) {
		return []gopsutil_disk.PartitionStat{
			{
				Device:     "/dev/dm-0",
				Mountpoint: "/",
				Fstype:     "xfs",
				Opts:       []string{"rw", "relatime"},
			},
		}, nil
	}), fakeStatFn), fs), "linux")

	// Configure custom proc_mountinfo_path
	config := integration.Data([]byte(`
resolve_root_device: false
proc_mountinfo_path: /host/proc/1/mountinfo
`))
	m := configureCheck(t, diskCheck, config, nil)
	err = diskCheck.Run()

	assert.Nil(t, err)
	// KEY ASSERTION: Device-mapper resolution worked using custom mountinfo path
	// /dev/dm-0 should be resolved to friendly /dev/mapper/host-root name
	m.AssertMetricTaggedWith(t, "Gauge", "system.disk.total", []string{"device:/dev/mapper/host-root", "device_name:host-root"})
}

func TestGivenADiskCheckWithDefaultConfig_WhenCheckRunsAndIOCountersSystemCallReturnsError_ThenPartitionMetricsAreStillCommitted(t *testing.T) {
	setupDefaultMocks()
	diskCheck := createDiskCheck(t)
	diskCheck = diskv2.WithDiskIOCounters(diskCheck, func(...string) (map[string]gopsutil_disk.IOCountersStat, error) {
		return nil, errors.New("incorrect function")
	})
	m := configureCheck(t, diskCheck, nil, nil)
	err := diskCheck.Run()

	// Check should succeed — partition metrics must be committed despite IOCounters failure
	assert.Nil(t, err)
	// Partition/usage metrics should still be reported
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(97656250), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(68359375), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(29296875), "", []string{"device:/dev/sda1", "device_name:sda1"})
	m.AssertMetric(t, "Gauge", "system.disk.total", float64(48828125), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.used", float64(29296875), "", []string{"device:/dev/sda2", "device_name:sda2"})
	m.AssertMetric(t, "Gauge", "system.disk.free", float64(19531250), "", []string{"device:/dev/sda2", "device_name:sda2"})
	// IO metrics should NOT be reported
	m.AssertNotCalled(t, "MonotonicCount", "system.disk.read_time", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
	m.AssertNotCalled(t, "MonotonicCount", "system.disk.write_time", mock.AnythingOfType("float64"), mock.AnythingOfType("string"), mock.AnythingOfType("[]string"))
}

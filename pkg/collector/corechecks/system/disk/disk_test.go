// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package disk

import (
	"regexp"
	"testing"

	"github.com/shirou/gopsutil/v3/disk"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
)

var (
	diskSamples = []disk.PartitionStat{
		{
			Device:     "/dev/sda2",
			Mountpoint: "/",
			Fstype:     "ext4",
			Opts:       []string{"rw,relatime,errors=remount-ro,data=ordered"},
		},
		{
			Device:     "/dev/sda1",
			Mountpoint: "/boot/efi",
			Fstype:     "vfat",
			Opts:       []string{"rw,relatime,fmask=0077,dmask=0077,codepage=437,iocharset=iso8859-1,shortname=mixed,errors=remount-ro"},
		},
	}
	diskUsageSamples = map[string]*disk.UsageStat{
		"/boot/efi": {
			Path:              "/boot/efi",
			Fstype:            "msdos",
			Total:             535805952,
			Free:              530948096,
			Used:              4857856,
			UsedPercent:       0.9066446503378894,
			InodesTotal:       0,
			InodesUsed:        0,
			InodesFree:        0,
			InodesUsedPercent: 0,
		},
		"/": {
			Path:              "/",
			Fstype:            "ext2/ext3",
			Total:             52045545472,
			Free:              39085445120,
			Used:              10285920256,
			UsedPercent:       19.763305702182958,
			InodesTotal:       3244032,
			InodesUsed:        290872,
			InodesFree:        2953160,
			InodesUsedPercent: 8.9663727114899,
		},
	}
	diskIoSamples = map[string]disk.IOCountersStat{
		"sda": {
			ReadCount:        443071,
			MergedReadCount:  104744,
			WriteCount:       10412454,
			MergedWriteCount: 310860,
			ReadBytes:        849293 * SectorSize,
			WriteBytes:       1406995 * SectorSize,
			ReadTime:         19699308,
			WriteTime:        418600,
			IopsInProgress:   0,
			IoTime:           343324,
			WeightedIO:       727464,
			Name:             "sda",
			SerialNumber:     "123456789WD",
		},
	}
)

func diskSampler(all bool) ([]disk.PartitionStat, error) {
	return diskSamples, nil
}

func diskUsageSampler(mountpoint string) (*disk.UsageStat, error) {
	return diskUsageSamples[mountpoint], nil
}

func diskIoSampler(names ...string) (map[string]disk.IOCountersStat, error) {
	return diskIoSamples, nil
}

func TestDiskCheck(t *testing.T) {
	diskPartitions = diskSampler
	diskUsage = diskUsageSampler
	ioCounters = diskIoSampler
	diskCheck := new(Check)
	mock := mocksender.NewMockSender(diskCheck.ID())
	diskCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	expectedMonoCounts := 2
	expectedRates := 2
	expectedGauges := 16

	mock.On("Gauge", "system.disk.total", 523248.0, "", []string{"device:/dev/sda1", "device_name:sda1"}).Return().Times(1)
	mock.On("Gauge", "system.disk.used", 4744.0, "", []string{"device:/dev/sda1", "device_name:sda1"}).Return().Times(1)
	mock.On("Gauge", "system.disk.free", 518504.0, "", []string{"device:/dev/sda1", "device_name:sda1"}).Return().Times(1)
	mock.On("Gauge", "system.disk.in_use", 0.009066446503378894, "", []string{"device:/dev/sda1", "device_name:sda1"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.total", 0.0, "", []string{"device:/dev/sda1", "device_name:sda1"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.used", 0.0, "", []string{"device:/dev/sda1", "device_name:sda1"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.free", 0.0, "", []string{"device:/dev/sda1", "device_name:sda1"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.in_use", 0.0, "", []string{"device:/dev/sda1", "device_name:sda1"}).Return().Times(1)

	mock.On("Gauge", "system.disk.total", 50825728.0, "", []string{"device:/dev/sda2", "device_name:sda2"}).Return().Times(1)
	mock.On("Gauge", "system.disk.used", 10044844.0, "", []string{"device:/dev/sda2", "device_name:sda2"}).Return().Times(1)
	mock.On("Gauge", "system.disk.free", 38169380.0, "", []string{"device:/dev/sda2", "device_name:sda2"}).Return().Times(1)
	mock.On("Gauge", "system.disk.in_use", 0.19763305702182958, "", []string{"device:/dev/sda2", "device_name:sda2"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.total", 3244032.0, "", []string{"device:/dev/sda2", "device_name:sda2"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.used", 290872.0, "", []string{"device:/dev/sda2", "device_name:sda2"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.free", 2953160.0, "", []string{"device:/dev/sda2", "device_name:sda2"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.in_use", 0.08966372711489899, "", []string{"device:/dev/sda2", "device_name:sda2"}).Return().Times(1)

	mock.On("MonotonicCount", "system.disk.read_time", 19699308.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("MonotonicCount", "system.disk.write_time", 418600.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)

	mock.On("Rate", "system.disk.read_time_pct", 1969930.8, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Rate", "system.disk.write_time_pct", 41860.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)
	diskCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "MonotonicCount", expectedMonoCounts)
	mock.AssertNumberOfCalls(t, "Gauge", expectedGauges)
	mock.AssertNumberOfCalls(t, "Rate", expectedRates)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestDiskCheckExcludedDiskFilsystem(t *testing.T) {
	diskPartitions = diskSampler
	diskUsage = diskUsageSampler
	ioCounters = diskIoSampler
	diskCheck := new(Check)
	mock := mocksender.NewMockSender(diskCheck.ID())
	diskCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")
	diskCheck.cfg.excludedFilesystems = []string{"vfat"}
	diskCheck.cfg.excludedDisks = []string{"/dev/sda2"}

	expectedMonoCounts := 2
	expectedGauges := 0
	expectedRates := 2

	mock.On("MonotonicCount", "system.disk.read_time", 19699308.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("MonotonicCount", "system.disk.write_time", 418600.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)

	mock.On("Rate", "system.disk.read_time_pct", 1969930.8, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Rate", "system.disk.write_time_pct", 41860.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)
	diskCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "MonotonicCount", expectedMonoCounts)
	mock.AssertNumberOfCalls(t, "Gauge", expectedGauges)
	mock.AssertNumberOfCalls(t, "Rate", expectedRates)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestDiskCheckExcludedRe(t *testing.T) {
	diskPartitions = diskSampler
	diskUsage = diskUsageSampler
	ioCounters = diskIoSampler
	diskCheck := new(Check)
	mock := mocksender.NewMockSender(diskCheck.ID())
	diskCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test")

	diskCheck.cfg.excludedMountpointRe = regexp.MustCompile("/boot/efi")
	diskCheck.cfg.excludedDiskRe = regexp.MustCompile("/dev/sda2")

	expectedMonoCounts := 2
	expectedGauges := 0
	expectedRates := 2

	mock.On("MonotonicCount", "system.disk.read_time", 19699308.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("MonotonicCount", "system.disk.write_time", 418600.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)

	mock.On("Rate", "system.disk.read_time_pct", 1969930.8, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Rate", "system.disk.write_time_pct", 41860.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)
	diskCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "MonotonicCount", expectedMonoCounts)
	mock.AssertNumberOfCalls(t, "Gauge", expectedGauges)
	mock.AssertNumberOfCalls(t, "Rate", expectedRates)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestDiskCheckTags(t *testing.T) {
	diskPartitions = diskSampler
	diskUsage = diskUsageSampler
	ioCounters = diskIoSampler
	diskCheck := new(Check)

	config := integration.Data([]byte("use_mount: true\ntag_by_filesystem: true\nall_partitions: true\ndevice_tag_re:\n  /boot/efi: role:esp\n  /dev/sda2: device_type:sata,disk_size:large"))

	mock := mocksender.NewMockSender(diskCheck.ID())
	diskCheck.Configure(mock.GetSenderManager(), integration.FakeConfigHash, config, nil, "test")

	expectedMonoCounts := 2
	expectedGauges := 16
	expectedRates := 2

	mock.On("Gauge", "system.disk.total", 523248.0, "", []string{"vfat", "filesystem:vfat", "device:/boot/efi", "device_name:sda1", "role:esp"}).Return().Times(1)
	mock.On("Gauge", "system.disk.used", 4744.0, "", []string{"vfat", "filesystem:vfat", "device:/boot/efi", "device_name:sda1", "role:esp"}).Return().Times(1)
	mock.On("Gauge", "system.disk.free", 518504.0, "", []string{"vfat", "filesystem:vfat", "device:/boot/efi", "device_name:sda1", "role:esp"}).Return().Times(1)
	mock.On("Gauge", "system.disk.in_use", 0.009066446503378894, "", []string{"vfat", "filesystem:vfat", "device:/boot/efi", "device_name:sda1", "role:esp"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.total", 0.0, "", []string{"vfat", "filesystem:vfat", "device:/boot/efi", "device_name:sda1", "role:esp"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.used", 0.0, "", []string{"vfat", "filesystem:vfat", "device:/boot/efi", "device_name:sda1", "role:esp"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.free", 0.0, "", []string{"vfat", "filesystem:vfat", "device:/boot/efi", "device_name:sda1", "role:esp"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.in_use", 0.0, "", []string{"vfat", "filesystem:vfat", "device:/boot/efi", "device_name:sda1", "role:esp"}).Return().Times(1)

	mock.On("Gauge", "system.disk.total", 50825728.0, "", []string{"ext4", "filesystem:ext4", "device:/", "device_name:sda2", "device_type:sata", "disk_size:large"}).Return().Times(1)
	mock.On("Gauge", "system.disk.used", 10044844.0, "", []string{"ext4", "filesystem:ext4", "device:/", "device_name:sda2", "device_type:sata", "disk_size:large"}).Return().Times(1)
	mock.On("Gauge", "system.disk.free", 38169380.0, "", []string{"ext4", "filesystem:ext4", "device:/", "device_name:sda2", "device_type:sata", "disk_size:large"}).Return().Times(1)
	mock.On("Gauge", "system.disk.in_use", 0.19763305702182958, "", []string{"ext4", "filesystem:ext4", "device:/", "device_name:sda2", "device_type:sata", "disk_size:large"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.total", 3244032.0, "", []string{"ext4", "filesystem:ext4", "device:/", "device_name:sda2", "device_type:sata", "disk_size:large"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.used", 290872.0, "", []string{"ext4", "filesystem:ext4", "device:/", "device_name:sda2", "device_type:sata", "disk_size:large"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.free", 2953160.0, "", []string{"ext4", "filesystem:ext4", "device:/", "device_name:sda2", "device_type:sata", "disk_size:large"}).Return().Times(1)
	mock.On("Gauge", "system.fs.inodes.in_use", 0.08966372711489899, "", []string{"ext4", "filesystem:ext4", "device:/", "device_name:sda2", "device_type:sata", "disk_size:large"}).Return().Times(1)

	mock.On("MonotonicCount", "system.disk.read_time", 19699308.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("MonotonicCount", "system.disk.write_time", 418600.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)

	mock.On("Rate", "system.disk.read_time_pct", 1969930.8, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)
	mock.On("Rate", "system.disk.write_time_pct", 41860.0, "", []string{"device:sda", "device_name:sda"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)
	diskCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "MonotonicCount", expectedMonoCounts)
	mock.AssertNumberOfCalls(t, "Gauge", expectedGauges)
	mock.AssertNumberOfCalls(t, "Rate", expectedRates)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

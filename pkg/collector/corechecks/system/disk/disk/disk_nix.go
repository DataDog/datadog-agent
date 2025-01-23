// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package disk

import (
	"fmt"
	"path/filepath"

	"github.com/shirou/gopsutil/v4/disk"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// for testing
var (
	diskPartitions = disk.Partitions
	diskUsage      = disk.Usage

	ioCounters = disk.IOCounters
)

// Check stores disk-specific additional fields
type Check struct {
	core.CheckBase
	cfg *diskConfig
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	err = c.collectPartitionMetrics(sender)
	if err != nil {
		return err
	}
	err = c.collectDiskMetrics(sender)
	if err != nil {
		return err
	}
	sender.Commit()

	return nil
}

func (c *Check) collectPartitionMetrics(sender sender.Sender) error {
	partitions, err := diskPartitions(c.cfg.allDevices)
	if err != nil {
		return err
	}

	for _, partition := range partitions {
		log.Debugf("Checking device %s", partition.Device)

		if c.excludePartition(partition) {
			log.Debugf("Excluding partition: [device: %s] [mountpoint: %s] [fstype: %s]", partition.Device, partition.Mountpoint, partition.Fstype)
			continue
		}

		// Get disk metrics here to be able to exclude on total usage
		usage, err := diskUsage(partition.Mountpoint)
		if err != nil {
			log.Warnf("Unable to get disk metrics of %s mount point: %s", partition.Mountpoint, err)
			continue
		}

		// Exclude disks with total disk size 0
		if usage.Total == 0 {
			continue
		}

		tags := make([]string, 0, 2)

		if c.cfg.tagByFilesystem {
			tags = append(tags, partition.Fstype, fmt.Sprintf("filesystem:%s", partition.Fstype))
		}
		var deviceName string
		if c.cfg.useMount {
			deviceName = partition.Mountpoint
		} else {
			deviceName = partition.Device
		}
		tags = append(tags, fmt.Sprintf("device:%s", deviceName))
		tags = append(tags, fmt.Sprintf("device_name:%s", filepath.Base(partition.Device)))

		tags = c.applyDeviceTags(partition.Device, partition.Mountpoint, tags)

		c.sendPartitionMetrics(sender, usage, tags)
	}

	return nil
}

func (c *Check) excludePartition(partition disk.PartitionStat) bool {
	device := partition.Device
	if device == "" || device == "none" {
		device = ""
		if !c.cfg.allPartitions {
			return true
		}
	}
	exclude := c.excludeDevice(device) || c.excludeFileSystem(partition.Fstype) || c.excludeMountPoint(partition.Mountpoint) || !c.includeDevice(device) || !c.includeFileSystem(partition.Fstype)
	return exclude
}

func (c *Check) excludeDevice(device string) bool {
	if device == "" || (len(c.cfg.excludedDevices) == 0) {
		return false
	}
	return sliceMatchesExpression(c.cfg.excludedDevices, device)
}

func (c *Check) includeDevice(device string) bool {
	if device == "" || len(c.cfg.includedDevices) == 0 {
		return true
	}
	return sliceMatchesExpression(c.cfg.includedDevices, device)
}

func (c *Check) excludeFileSystem(fileSystem string) bool {
	if len(c.cfg.excludedFilesystems) == 0 {
		return false
	}
	return stringSliceContain(c.cfg.excludedFilesystems, fileSystem)
}

func (c *Check) includeFileSystem(fileSystem string) bool {
	if len(c.cfg.includedFilesystems) == 0 {
		return true
	}
	return stringSliceContain(c.cfg.includedFilesystems, fileSystem)
}

func (c *Check) excludeMountPoint(mountPoint string) bool {
	if c.cfg.excludedMountpointRe == nil {
		return false
	}
	return c.cfg.excludedMountpointRe.MatchString(mountPoint)
}

func (c *Check) collectDiskMetrics(sender sender.Sender) error {
	iomap, err := ioCounters()
	if err != nil {
		return err
	}
	for deviceName, ioCounter := range iomap {

		tags := []string{}
		tags = append(tags, fmt.Sprintf("device:%s", deviceName))
		tags = append(tags, fmt.Sprintf("device_name:%s", deviceName))

		tags = c.applyDeviceTags(deviceName, "", tags)

		c.sendDiskMetrics(sender, ioCounter, tags)
	}

	return nil
}

func (c *Check) sendPartitionMetrics(sender sender.Sender, usage *disk.UsageStat, tags []string) {
	// Disk metrics
	// For legacy reasons,  the standard unit it kB
	sender.Gauge(fmt.Sprintf(diskMetric, "total"), float64(usage.Total)/1024, "", tags)
	sender.Gauge(fmt.Sprintf(diskMetric, "used"), float64(usage.Used)/1024, "", tags)
	sender.Gauge(fmt.Sprintf(diskMetric, "free"), float64(usage.Free)/1024, "", tags)
	// FIXME(8.x): use percent, a lot more logical than in_use
	sender.Gauge(fmt.Sprintf(diskMetric, "in_use"), usage.UsedPercent/100, "", tags)

	// Inodes metrics
	sender.Gauge(fmt.Sprintf(inodeMetric, "total"), float64(usage.InodesTotal), "", tags)
	sender.Gauge(fmt.Sprintf(inodeMetric, "used"), float64(usage.InodesUsed), "", tags)
	sender.Gauge(fmt.Sprintf(inodeMetric, "free"), float64(usage.InodesFree), "", tags)
	// FIXME(8.x): use percent, a lot more logical than in_use
	sender.Gauge(fmt.Sprintf(inodeMetric, "in_use"), usage.InodesUsedPercent/100, "", tags)
}

func (c *Check) sendDiskMetrics(sender sender.Sender, ioCounter disk.IOCountersStat, tags []string) {
	sender.MonotonicCount(fmt.Sprintf(diskMetric, "read_time"), float64(ioCounter.ReadTime), "", tags)
	sender.MonotonicCount(fmt.Sprintf(diskMetric, "write_time"), float64(ioCounter.WriteTime), "", tags)
	// FIXME(8.x): These older metrics are kept here for backwards compatibility, but they are wrong: the value is not a percentage
	sender.Rate(fmt.Sprintf(diskMetric, "read_time_pct"), float64(ioCounter.ReadTime)*100/1000, "", tags)
	sender.Rate(fmt.Sprintf(diskMetric, "write_time_pct"), float64(ioCounter.WriteTime)*100/1000, "", tags)
}

// Configure the disk check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, initConfig, data, source)
	if err != nil {
		return err
	}
	return c.instanceConfigure(data)
}

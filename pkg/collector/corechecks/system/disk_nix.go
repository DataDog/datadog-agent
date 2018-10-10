// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package system

import (
	"fmt"

	"github.com/shirou/gopsutil/disk"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// for testing
var (
	diskPartitions = disk.Partitions
	diskUsage      = disk.Usage
)

// DiskCheck stores disk-specific additional fields
type DiskCheck struct {
	core.CheckBase
	cfg *diskConfig
}

// Run executes the check
func (c *DiskCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
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

func (c *DiskCheck) collectPartitionMetrics(sender aggregator.Sender) error {
	partitions, err := diskPartitions(true)
	if err != nil {
		return err
	}

	for _, partition := range partitions {
		if c.excludeDisk(partition.Mountpoint, partition.Device, partition.Fstype) {
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

		tags := make([]string, len(c.cfg.customeTags), len(c.cfg.customeTags)+2)
		copy(tags, c.cfg.customeTags)

		if c.cfg.tagByFilesystem {
			tags = append(tags, fmt.Sprintf("filesystem:%s", partition.Fstype))
		}
		var deviceName string
		if c.cfg.useMount {
			deviceName = partition.Mountpoint
		} else {
			deviceName = partition.Device
		}
		tags = append(tags, fmt.Sprintf("device:%s", deviceName))

		tags = c.applyDeviceTags(partition.Device, partition.Mountpoint, tags)

		c.sendPartitionMetrics(sender, usage, tags)
	}

	return nil
}

func (c *DiskCheck) collectDiskMetrics(sender aggregator.Sender) error {
	iomap, err := ioCounters()
	if err != nil {
		return err
	}
	for deviceName, ioCounter := range iomap {

		tags := make([]string, len(c.cfg.customeTags), len(c.cfg.customeTags)+1)
		copy(tags, c.cfg.customeTags)
		tags = append(tags, fmt.Sprintf("device:%s", deviceName))

		tags = c.applyDeviceTags(deviceName, "", tags)

		c.sendDiskMetrics(sender, ioCounter, tags)
	}

	return nil
}

func (c *DiskCheck) sendPartitionMetrics(sender aggregator.Sender, usage *disk.UsageStat, tags []string) {
	// Disk metrics
	// For legacy reasons,  the standard unit it kB
	sender.Gauge(fmt.Sprintf(diskMetric, "total"), float64(usage.Total)/1024, "", tags)
	sender.Gauge(fmt.Sprintf(diskMetric, "used"), float64(usage.Used)/1024, "", tags)
	sender.Gauge(fmt.Sprintf(diskMetric, "free"), float64(usage.Free)/1024, "", tags)
	// FIXME: 6.x, use percent, a lot more logical than in_use
	sender.Gauge(fmt.Sprintf(diskMetric, "in_use"), usage.UsedPercent/100, "", tags)

	// Inodes metrics
	sender.Gauge(fmt.Sprintf(inodeMetric, "total"), float64(usage.InodesTotal), "", tags)
	sender.Gauge(fmt.Sprintf(inodeMetric, "used"), float64(usage.InodesUsed), "", tags)
	sender.Gauge(fmt.Sprintf(inodeMetric, "free"), float64(usage.InodesFree), "", tags)
	// FIXME: 6.x, use percent, a lot more logical than in_use
	sender.Gauge(fmt.Sprintf(inodeMetric, "in_use"), usage.InodesUsedPercent/100, "", tags)

}

func (c *DiskCheck) sendDiskMetrics(sender aggregator.Sender, ioCounter disk.IOCountersStat, tags []string) {

	// /1000 as psutil returns the value in ms
	// Rate computes a rate of change between to consecutive check run.
	// For cumulated time values like read and write times this a ratio between 0 and 1, we want it as a percentage so we *100 in advance
	sender.Rate(fmt.Sprintf(diskMetric, "read_time_pct"), float64(ioCounter.ReadTime)*100/1000, "", tags)
	sender.Rate(fmt.Sprintf(diskMetric, "write_time_pct"), float64(ioCounter.WriteTime)*100/1000, "", tags)
}

// Configure the disk check
func (c *DiskCheck) Configure(data integration.Data, initConfig integration.Data) error {
	err := c.CommonConfigure(data)
	if err != nil {
		return err
	}
	return c.instanceConfigure(data)
}

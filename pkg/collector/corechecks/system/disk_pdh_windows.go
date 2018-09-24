// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package system

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
	"github.com/shirou/gopsutil/disk"
)

// DiskCheck stores disk-specific additional fields
type DiskCheck struct {
	core.CheckBase
	cfg      *diskConfig
	counters map[string]*pdhutil.PdhCounterSet
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
	partitions, err := disk.Partitions(true)
	if err != nil {
		return err
	}

	for _, partition := range partitions {
		if c.excludeDisk(partition.Mountpoint, partition.Device, partition.Fstype) {
			continue
		}

		// Get disk metrics here to be able to exclude on total usage
		diskUsage, err := disk.Usage(partition.Mountpoint)
		if err != nil {
			log.Warnf("Unable to get disk metrics of %s mount point: %s", partition.Mountpoint, err)
			continue
		}

		// Exclude disks with total disk size 0
		if diskUsage.Total == 0 {
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

		// apply device/mountpoint specific tags
		for re, deviceTags := range c.cfg.deviceTagRe {
			if re != nil && (re.MatchString(partition.Device) || re.MatchString(partition.Mountpoint)) {
				for _, tag := range deviceTags {
					tags = append(tags, tag)
				}
			}
		}

		c.sendPartitionMetrics(sender, diskUsage, tags)
	}

	return nil
}

func (c *DiskCheck) Configure(data integration.Data, initConfig integration.Data) error {
	err := c.commonConfigure(data)
	if err != nil {
		return err
	}

	counternames = []string{
		"% Free Space",
		"Free Megabytes",
		"% Disk Read Time",
		"% Disk Write Time"
	}

	c.counters = make(map[string]*pdhutil.PdhCounterSet)

	for name := range counternames {
		c.counters[name], err = pdhutil.GetCounterSet("LogicalDisk", name, "", isDrive)
		if err != nil {
			return err
		}
	}

	return nil
}

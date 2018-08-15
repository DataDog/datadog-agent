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
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

// DiskCheck stores disk-specific additional fields
type DiskCheck struct {
	core.CheckBase
	cfg     *diskConfig
	counter *pdhutil.PdhCounterSet
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
	sender.Commit()

	return nil
}

func (c *DiskCheck) collectPartitionMetrics(sender aggregator.Sender) error {
	drives, err := c.counter.GetAllValues()
	if err != nil {
		return err
	}
	for drive, metrics := range drives {
		fmt.Println("drive", drive)
		fstype := winutil.GetDriveFsType(drive)
		if c.excludeDisk(drive, drive, fstype) {
			continue
		}

		tags := make([]string, len(c.cfg.customeTags), len(c.cfg.customeTags)+2)
		copy(tags, c.cfg.customeTags)

		if c.cfg.tagByFilesystem {
			tags = append(tags, fmt.Sprintf("filesystem:%s", fstype))
		}

		tags = append(tags, fmt.Sprintf("device:%s", drive))

		// apply device/mountpoint specific tags
		for re, deviceTags := range c.cfg.deviceTagRe {
			if re != nil && re.MatchString(drive) {
				for _, tag := range deviceTags {
					tags = append(tags, tag)
				}
			}
		}
		//metrics := make()
		//c.sendPartitionMetrics(sender, diskUsage, tags)
	}

	return nil
}

func (c *DiskCheck) Configure(data integration.Data, initConfig integration.Data) error {
	err := c.commonConfigure(data)
	if err != nil {
		return err
	}

	counternames := []string{
		"% Free Space",
		"Free Megabytes",
		"% Disk Read Time",
		"% Disk Write Time",
	}

	c.counter, err = pdhutil.GetCounterSet("LogicalDisk", counternames, "", isDrive)
	if err != nil {
		return err
	}

	return nil
}

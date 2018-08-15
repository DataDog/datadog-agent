// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package system

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	err = c.collectMetrics(sender)
	if err != nil {
		return err
	}
	sender.Commit()

	return nil
}

func (c *DiskCheck) collectMetrics(sender aggregator.Sender) error {
	drives, err := c.counter.GetAllValues()
	if err != nil {
		return err
	}
	for drive, metrics := range drives {
		driveSlash := drive + `\`
		fstype, err := winutil.GetDriveFsType(driveSlash)
		if err != nil {
			log.Warnf("Unable to get filesystem type of drive: %s", drive)
		}
		drive = strings.ToLower(drive)
		if c.excludeDisk(driveSlash, driveSlash, fstype) {
			continue
		}

		tags := make([]string, len(c.cfg.customeTags), len(c.cfg.customeTags)+2)
		copy(tags, c.cfg.customeTags)

		if c.cfg.tagByFilesystem && fstype != "" {
			tags = append(tags, fmt.Sprintf("filesystem:%s", fstype))
		}

		tags = append(tags, fmt.Sprintf("device:%s", drive))

		tags = c.applyDeviceTags(drive, "", tags)

		c.sendMetrics(sender, metrics, tags)
	}

	return nil
}

func (c *DiskCheck) sendMetrics(sender aggregator.Sender, metrics map[string]float64, tags []string) {
	// Disk metrics
	// For legacy reasons,  the standard unit it kB
	freePercent := metrics["% Free Space"]
	free := metrics["Free Megabytes"] * 1024
	readTimePercent := metrics["% Disk Read Time"]
	writeTimePercent := metrics["% Disk Write Time"]
	total := free / freePercent * 100

	sender.Gauge(fmt.Sprintf(diskMetric, "total"), total, "", tags)
	sender.Gauge(fmt.Sprintf(diskMetric, "used"), total-free, "", tags)
	sender.Gauge(fmt.Sprintf(diskMetric, "free"), free, "", tags)
	// FIXME: 6.x, use percent, a lot more logical than in_use
	sender.Gauge(fmt.Sprintf(diskMetric, "in_use"), (100-freePercent)/100, "", tags)

	// /1000 as psutil returns the value in ms
	// Rate computes a rate of change between to consecutive check run.
	// For cumulated time values like read and write times this a ratio between 0 and 1, we want it as a percentage so we *100 in advance
	sender.Gauge(fmt.Sprintf(diskMetric, "read_time_pct"), readTimePercent, "", tags)
	sender.Gauge(fmt.Sprintf(diskMetric, "write_time_pct"), writeTimePercent, "", tags)

}

// Configure the disk check
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

// set case-insensitive flag for windows
func formatRegexp(regexp string) string {
	return "(?i)" + regexp
}

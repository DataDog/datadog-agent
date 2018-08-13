// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package system

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/shirou/gopsutil/disk"
	yaml "gopkg.in/yaml.v2"
)

const (
	diskCheckName = "disk"
	diskMetric    = "system.disk.%s"
	inodeMetric   = "system.fs.inode.%s"
)

// DiskCheck stores disk-specific additional fields
type DiskCheck struct {
	core.CheckBase
	useMount             bool
	excludedFilesystems  []string
	excludedDisks        []string
	excludedDiskRe       *regexp.Regexp
	tagByFilesystem      bool
	excludedMountpointRe *regexp.Regexp
	allPartitions        bool
	deviceTagRe          map[*regexp.Regexp][]string
	customeTags          []string
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
		if c.excludeDisk(partition) {
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

		tags := make([]string, len(c.customeTags), len(c.customeTags)+2)
		copy(tags, c.customeTags)

		if c.tagByFilesystem {
			tags = append(tags, fmt.Sprintf("filesystem:%s", partition.Fstype))
		}
		var deviceName string
		if c.useMount {
			deviceName = partition.Mountpoint
		} else {
			deviceName = partition.Device
		}
		tags = append(tags, fmt.Sprintf("device:%s", deviceName))

		// apply device/mountpoint specific tags
		for re, deviceTags := range c.deviceTagRe {
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

func (c *DiskCheck) collectDiskMetrics(sender aggregator.Sender) error {
	ioCounters, err := disk.IOCounters()
	if err != nil {
		return err
	}
	for deviceName, ioCounter := range ioCounters {

		tags := make([]string, len(c.customeTags)+1)
		copy(tags, c.customeTags)
		tags = append(tags, fmt.Sprintf("device:%s", deviceName))

		// apply device specific tags
		for re, deviceTags := range c.deviceTagRe {
			if re != nil && re.MatchString(deviceName) {
				for _, tag := range deviceTags {
					tags = append(tags, tag)
				}
			}
		}

		c.sendDiskMetrics(sender, ioCounter, tags)
	}

	return nil
}

func (c *DiskCheck) sendPartitionMetrics(sender aggregator.Sender, diskUsage *disk.UsageStat, tags []string) {
	// Disk metrics
	// For legacy reasons,  the standard unit it kB
	sender.Gauge(fmt.Sprintf(diskMetric, "total"), float64(diskUsage.Total)/1024, "", tags)
	sender.Gauge(fmt.Sprintf(diskMetric, "used"), float64(diskUsage.Used)/1024, "", tags)
	sender.Gauge(fmt.Sprintf(diskMetric, "free"), float64(diskUsage.Free)/1024, "", tags)
	sender.Gauge(fmt.Sprintf(diskMetric, "in_use"), diskUsage.UsedPercent/100, "", tags)
	// Use percent, a lot more logical than in_use
	sender.Gauge(fmt.Sprintf(diskMetric, "used.percent"), diskUsage.UsedPercent, "", tags)

	// Inodes metrics
	sender.Gauge(fmt.Sprintf(inodeMetric, "total"), float64(diskUsage.InodesTotal), "", tags)
	sender.Gauge(fmt.Sprintf(inodeMetric, "used"), float64(diskUsage.InodesUsed), "", tags)
	sender.Gauge(fmt.Sprintf(inodeMetric, "free"), float64(diskUsage.InodesFree), "", tags)
	sender.Gauge(fmt.Sprintf(inodeMetric, "in_use"), diskUsage.InodesUsedPercent/100, "", tags)
	// Use percent, a lot more logical than in_use
	sender.Gauge(fmt.Sprintf(inodeMetric, "used.percent"), diskUsage.InodesUsedPercent, "", tags)

}

func (c *DiskCheck) sendDiskMetrics(sender aggregator.Sender, ioCounter disk.IOCountersStat, tags []string) {

	// /1000 as psutil returns the value in ms
	// Rate computes a rate of change between to consecutive check run.
	// For cumulated time values like read and write times this a ratio between 0 and 1, we want it as a percentage so we *100 in advance
	sender.Rate(fmt.Sprintf(diskMetric, "read_time_pct"), float64(ioCounter.ReadTime)*100/1000, "", tags)
	sender.Rate(fmt.Sprintf(diskMetric, "write_time_pct"), float64(ioCounter.WriteTime)*100/1000, "", tags)
}

func (c *DiskCheck) excludeDisk(disk disk.PartitionStat) bool {

	// Hack for NFS secure mounts
	// Secure mounts might look like this: '/mypath (deleted)', we should
	// ignore all the bits not part of the mountpoint name. Take also into
	// account a space might be in the mountpoint.
	disk.Mountpoint = strings.Split(disk.Mountpoint, " ")[0]

	nameEmpty := disk.Device == "" || disk.Device == "none"

	// allow empty names if `all_partitions` is `yes` so we can evaluate mountpoints
	if nameEmpty {
		if !c.allPartitions {
			return true
		}
	} else {
		// I don't why I we do this only if the device name is not empty
		// This is useful only when `all_partitions` is true and `exclude_disk_re` matches empty strings or `excluded_devices` contains the device

		// device is listed in `excluded_disks`
		if stringSliceContain(c.excludedDisks, disk.Device) {
			return true
		}

		// device name matches `excluded_disk_re`
		if c.excludedDiskRe != nil && c.excludedDiskRe.MatchString(disk.Device) {
			return true
		}
	}

	// fs is listed in `excluded_filesystems`
	if stringSliceContain(c.excludedFilesystems, disk.Fstype) {
		return true
	}

	// device mountpoint matches `excluded_mountpoint_re`
	if c.excludedMountpointRe != nil && c.excludedMountpointRe.MatchString(disk.Mountpoint) {
		return true
	}

	// all good, don't exclude the disk
	return false
}

// Configure the disk check
func (c *DiskCheck) Configure(data integration.Data, initConfig integration.Data) error {
	conf := make(map[interface{}]interface{})

	err := yaml.Unmarshal([]byte(data), &conf)
	if err != nil {
		return err
	}

	useMount, found := conf["use_mount"]
	if useMount, ok := useMount.(bool); found && ok {
		c.useMount = useMount
	}

	excludedFilesystems, found := conf["excluded_filesystems"]
	if excludedFilesystems, ok := excludedFilesystems.([]string); found && ok {
		c.excludedFilesystems = excludedFilesystems
	}

	// Force exclusion of CDROM (iso9660) from disk check
	c.excludedFilesystems = append(c.excludedFilesystems, "iso9660")

	excludedDisks, found := conf["excluded_disks"]
	if excludedDisks, ok := excludedDisks.([]string); found && ok {
		c.excludedDisks = excludedDisks
	}

	excludedDiskRe, found := conf["excluded_disk_re"]
	if excludedDiskRe, ok := excludedDiskRe.(string); found && ok {
		c.excludedDiskRe, err = regexp.Compile(excludedDiskRe)
		if err != nil {
			return err
		}
	}

	tagByFilesystem, found := conf["tag_by_filesystem"]
	if tagByFilesystem, ok := tagByFilesystem.(bool); found && ok {
		c.tagByFilesystem = tagByFilesystem
	}

	excludedMountpointRe, found := conf["excluded_mountpoint_re"]
	if excludedMountpointRe, ok := excludedMountpointRe.(string); found && ok {
		c.excludedMountpointRe, err = regexp.Compile(excludedMountpointRe)
		if err != nil {
			return err
		}
	}

	allPartitions, found := conf["all_partitions"]
	if allPartitions, ok := allPartitions.(bool); found && ok {
		c.tagByFilesystem = allPartitions
	}

	deviceTagRe, found := conf["device_tag_re"]
	if deviceTagRe, ok := deviceTagRe.(map[string]string); found && ok {
		for reString, tags := range deviceTagRe {
			re, err := regexp.Compile(reString)
			if err != nil {
				return err
			}
			c.deviceTagRe[re] = strings.Split(tags, ",")
		}
	}

	tags, found := conf["tags"]
	if tags, ok := tags.([]string); found && ok {
		c.customeTags = tags
	}

	return nil
}

func stringSliceContain(slice []string, x string) bool {
	for _, e := range slice {
		if e == x {
			return true
		}
	}
	return false
}

func diskFactory() check.Check {
	return &DiskCheck{
		CheckBase: core.NewCheckBase(diskCheckName),
	}
}

func init() {
	core.RegisterCheck(diskCheckName, diskFactory)
}

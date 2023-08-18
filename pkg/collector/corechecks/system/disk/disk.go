// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

package disk

import (
	"regexp"
	"strings"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const (
	checkName   = "disk"
	diskMetric  = "system.disk.%s"
	inodeMetric = "system.fs.inodes.%s"
)

type diskConfig struct {
	useMount             bool
	excludedFilesystems  []string
	excludedDisks        []string
	excludedDiskRe       *regexp.Regexp
	tagByFilesystem      bool
	excludedMountpointRe *regexp.Regexp
	allPartitions        bool
	deviceTagRe          map[*regexp.Regexp][]string
}

func (c *Check) excludeDisk(mountpoint, device, fstype string) bool {

	// Hack for NFS secure mounts
	// Secure mounts might look like this: '/mypath (deleted)', we should
	// ignore all the bits not part of the mountpoint name. Take also into
	// account a space might be in the mountpoint.
	mountpoint = strings.Split(mountpoint, " ")[0]

	nameEmpty := device == "" || device == "none"

	// allow empty names if `all_partitions` is `yes` so we can evaluate mountpoints
	if nameEmpty {
		if !c.cfg.allPartitions {
			return true
		}
	} else {
		// I don't why I we do this only if the device name is not empty
		// This is useful only when `all_partitions` is true and `exclude_disk_re` matches empty strings or `excluded_devices` contains the device

		// device is listed in `excluded_disks`
		if stringSliceContain(c.cfg.excludedDisks, device) {
			return true
		}

		// device name matches `excluded_disk_re`
		if c.cfg.excludedDiskRe != nil && c.cfg.excludedDiskRe.MatchString(device) {
			return true
		}
	}

	// fs is listed in `excluded_filesystems`
	if stringSliceContain(c.cfg.excludedFilesystems, fstype) {
		return true
	}

	// device mountpoint matches `excluded_mountpoint_re`
	if c.cfg.excludedMountpointRe != nil && c.cfg.excludedMountpointRe.MatchString(mountpoint) {
		return true
	}

	// all good, don't exclude the disk
	return false
}

func (c *Check) instanceConfigure(data integration.Data) error {
	conf := make(map[interface{}]interface{})
	c.cfg = &diskConfig{}
	err := yaml.Unmarshal([]byte(data), &conf)
	if err != nil {
		return err
	}

	useMount, found := conf["use_mount"]
	if useMount, ok := useMount.(bool); found && ok {
		c.cfg.useMount = useMount
	}

	excludedFilesystems, found := conf["excluded_filesystems"]
	if excludedFilesystems, ok := excludedFilesystems.([]string); found && ok {
		c.cfg.excludedFilesystems = excludedFilesystems
	}

	// Force exclusion of CDROM (iso9660) from disk check
	c.cfg.excludedFilesystems = append(c.cfg.excludedFilesystems, "iso9660")

	excludedDisks, found := conf["excluded_disks"]
	if excludedDisks, ok := excludedDisks.([]string); found && ok {
		c.cfg.excludedDisks = excludedDisks
	}

	excludedDiskRe, found := conf["excluded_disk_re"]
	if excludedDiskRe, ok := excludedDiskRe.(string); found && ok {
		c.cfg.excludedDiskRe, err = regexp.Compile(excludedDiskRe)
		if err != nil {
			return err
		}
	}

	tagByFilesystem, found := conf["tag_by_filesystem"]
	if tagByFilesystem, ok := tagByFilesystem.(bool); found && ok {
		c.cfg.tagByFilesystem = tagByFilesystem
	}

	excludedMountpointRe, found := conf["excluded_mountpoint_re"]
	if excludedMountpointRe, ok := excludedMountpointRe.(string); found && ok {
		c.cfg.excludedMountpointRe, err = regexp.Compile(excludedMountpointRe)
		if err != nil {
			return err
		}
	}

	allPartitions, found := conf["all_partitions"]
	if allPartitions, ok := allPartitions.(bool); found && ok {
		c.cfg.allPartitions = allPartitions
	}

	deviceTagRe, found := conf["device_tag_re"]
	if deviceTagRe, ok := deviceTagRe.(map[interface{}]interface{}); found && ok {
		c.cfg.deviceTagRe = make(map[*regexp.Regexp][]string)
		for reString, tags := range deviceTagRe {
			if reString, ok := reString.(string); ok {
				if tags, ok := tags.(string); ok {
					re, err := regexp.Compile(reString)
					if err != nil {
						return err
					}
					c.cfg.deviceTagRe[re] = strings.Split(tags, ",")
				}
			}
		}
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

func (c *Check) applyDeviceTags(device, mountpoint string, tags []string) []string {
	// apply device/mountpoint specific tags
	for re, deviceTags := range c.cfg.deviceTagRe {
		if re == nil {
			continue
		}
		if re.MatchString(device) || (mountpoint != "" && re.MatchString(mountpoint)) {
			for _, tag := range deviceTags {
				tags = append(tags, tag)
			}
		}
	}
	return tags
}

func diskFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(checkName),
	}
}

func init() {
	core.RegisterCheck(checkName, diskFactory)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package system

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	yaml "gopkg.in/yaml.v2"
)

const diskCheckName = "disk"

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

func (c *DiskCheck) Run() error {
	return nil
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

func diskFactory() check.Check {
	return &DiskCheck{
		CheckBase: core.NewCheckBase(diskCheckName),
	}
}

func init() {
	core.RegisterCheck(diskCheckName, diskFactory)
}

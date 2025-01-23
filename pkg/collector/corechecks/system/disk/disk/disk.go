// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

//nolint:revive // TODO(PLINT) Fix revive linter
package disk

import (
	"regexp"
	"strings"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName   = "disk"
	diskMetric  = "system.disk.%s"
	inodeMetric = "system.fs.inodes.%s"
)

type diskConfig struct {
	useMount             bool
	includedDevices      []regexp.Regexp
	excludedDevices      []regexp.Regexp
	includedFilesystems  []regexp.Regexp
	excludedFilesystems  []regexp.Regexp
	tagByFilesystem      bool
	excludedMountpointRe *regexp.Regexp
	allPartitions        bool
	deviceTagRe          map[*regexp.Regexp][]string
	allDevices           bool
}

func NewDiskConfig() *diskConfig {
	return &diskConfig{
		useMount:             false,
		includedDevices:      []regexp.Regexp{},
		excludedDevices:      []regexp.Regexp{},
		includedFilesystems:  []regexp.Regexp{},
		excludedFilesystems:  []regexp.Regexp{},
		tagByFilesystem:      false,
		excludedMountpointRe: nil,
		allPartitions:        false,
		deviceTagRe:          make(map[*regexp.Regexp][]string),
		allDevices:           true,
	}
}

// func (c *Check) excludeDisk(mountpoint, device, fstype string) bool {
// 	nameEmpty := device == "" || device == "none"

// 	// allow empty names if `all_partitions` is `yes` so we can evaluate mountpoints
// 	if nameEmpty {
// 		if !c.cfg.allPartitions {
// 			return true
// 		}
// 	} else {
// 		// I don't why I we do this only if the device name is not empty
// 		// This is useful only when `all_partitions` is true and `exclude_disk_re` matches empty strings or `excluded_devices` contains the device

// 		// device is listed in `excluded_disks`
// 		if stringSliceContain(c.cfg.excludedDevices, device) {
// 			return true
// 		}

// 		// device name matches `excluded_disk_re`
// 		if c.cfg.excludedDeviceRe != nil && c.cfg.excludedDeviceRe.MatchString(device) {
// 			return true
// 		}
// 	}

// 	// fs is listed in `excluded_filesystems`
// 	if stringSliceContain(c.cfg.excludedFilesystems, fstype) {
// 		return true
// 	}

// 	// Hack for NFS secure mounts
// 	// Secure mounts might look like this: '/mypath (deleted)', we should
// 	// ignore all the bits not part of the mountpoint name. Take also into
// 	// account a space might be in the mountpoint.
// 	mountpoint = strings.Split(mountpoint, " ")[0]
// 	// device mountpoint matches `excluded_mountpoint_re`
// 	if c.cfg.excludedMountpointRe != nil && c.cfg.excludedMountpointRe.MatchString(mountpoint) {
// 		return true
// 	}

// 	// all good, don't exclude the disk
// 	return false
// }

func (c *Check) instanceConfigure(data integration.Data) error {
	conf := make(map[interface{}]interface{})
	c.cfg = NewDiskConfig()
	err := yaml.Unmarshal([]byte(data), &conf)
	if err != nil {
		return err
	}

	useMount, found := conf["use_mount"]
	if useMount, ok := useMount.(bool); found && ok {
		c.cfg.useMount = useMount
	}

	includeAllDevices, found := conf["include_all_devices"]
	if includeAllDevices, ok := includeAllDevices.(bool); found && ok {
		c.cfg.allDevices = includeAllDevices
	}

	err = c.configureExcludeDevice(conf)
	if err != nil {
		return err
	}
	err = c.configureIncludeDevice(conf)
	if err != nil {
		return err
	}
	err = c.configureExcludeFileSystem(conf)
	if err != nil {
		return err
	}
	err = c.configureIncludeFileSystem(conf)
	if err != nil {
		return err
	}
	err = c.configureExcludeMountPoint(conf)
	if err != nil {
		return err
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

func (c *Check) configureExcludeDevice(conf map[interface{}]interface{}) error {
	for _, key := range []string{"device_exclude", "device_blacklist", "excluded_disks"} {
		if deviceExclude, ok := conf[key].([]interface{}); ok {
			for _, val := range deviceExclude {
				if strVal, ok := val.(string); ok {
					regexp, err := regexp.Compile(strVal)
					if err != nil {
						return err
					}
					c.cfg.excludedDevices = append(c.cfg.excludedDevices, *regexp)
				}
			}
		}
	}
	excludedDiskRe, found := conf["excluded_disk_re"] //Maintained for backwards compatibility. It would now be easier to add regular expressions to the 'device_exclude' list key
	if excludedDiskRe, ok := excludedDiskRe.(string); found && ok {
		var err error
		regexp, err := regexp.Compile(excludedDiskRe)
		if err != nil {
			return err
		}
		c.cfg.excludedDevices = append(c.cfg.excludedDevices, *regexp)
	}
	return nil
}

func (c *Check) configureIncludeDevice(conf map[interface{}]interface{}) error {
	for _, key := range []string{"device_include", "device_whitelist"} {
		if deviceInclude, ok := conf[key].([]interface{}); ok {
			for _, val := range deviceInclude {
				if strVal, ok := val.(string); ok {
					regexp, err := regexp.Compile(strVal)
					if err != nil {
						return err
					}
					c.cfg.includedDevices = append(c.cfg.includedDevices, *regexp)
				}
			}
		}
	}
	return nil
}

func (c *Check) configureExcludeFileSystem(conf map[interface{}]interface{}) error {
	for _, key := range []string{"file_system_exclude", "file_system_blacklist", "excluded_filesystems"} {
		if fileSystemExclude, ok := conf[key].([]interface{}); ok {
			for _, val := range fileSystemExclude {
				if strVal, ok := val.(string); ok {
					regexp, err := regexp.Compile(strVal)
					if err != nil {
						return err
					}
					c.cfg.excludedFilesystems = append(c.cfg.excludedFilesystems, *regexp)
				}
			}
		}
	}
	// Force exclusion of CDROM (iso9660) from disk check
	regexp, err := regexp.Compile("iso9660")
	if err != nil {
		return err
	}
	c.cfg.excludedFilesystems = append(c.cfg.excludedFilesystems, *regexp)
	return nil
}

func (c *Check) configureIncludeFileSystem(conf map[interface{}]interface{}) error {
	for _, key := range []string{"file_system_include", "file_system_whitelist"} {
		if fileSystemInclude, ok := conf[key].([]interface{}); ok {
			for _, val := range fileSystemInclude {
				if strVal, ok := val.(string); ok {
					regexp, err := regexp.Compile(strVal)
					if err != nil {
						return err
					}
					c.cfg.includedFilesystems = append(c.cfg.includedFilesystems, *regexp)
				}
			}
		}
	}
	return nil
}

func (c *Check) configureExcludeMountPoint(conf map[interface{}]interface{}) error {
	excludedMountPointRe, found := conf["excluded_mountpoint_re"]
	if excludedMountPointRe, ok := excludedMountPointRe.(string); found && ok {
		var err error
		c.cfg.excludedMountpointRe, err = regexp.Compile(excludedMountPointRe)
		if err != nil {
			return err
		}
	}
	return nil
}

// func stringSliceContain(slice []string, x string) bool {
// 	for _, e := range slice {
// 		if e == x {
// 			return true
// 		}
// 	}
// 	return false
// }

func sliceMatchesExpression(slice []regexp.Regexp, expression string) bool {
	for _, regexp := range slice {
		if regexp.MatchString(expression) {
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
			tags = append(tags, deviceTags...)
		}
	}
	return tags
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
	}
}

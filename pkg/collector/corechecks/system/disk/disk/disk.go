// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build !windows

//nolint:revive // TODO(PLINT) Fix revive linter
package disk

import (
	"errors"
	"regexp"
	"strings"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName   = "disk"
	diskMetric  = "system.disk.%s"
	inodeMetric = "system.fs.inodes.%s"
)

type diskConfig struct {
	useMount            bool
	includedDevices     []regexp.Regexp
	excludedDevices     []regexp.Regexp
	includedFilesystems []regexp.Regexp
	excludedFilesystems []regexp.Regexp
	includedMountpoints []regexp.Regexp
	excludedMountpoints []regexp.Regexp
	tagByFilesystem     bool
	allPartitions       bool
	deviceTagRe         map[*regexp.Regexp][]string
	allDevices          bool
	minDiskSize         uint64
	tagByLabel          bool
	useLsblk            bool
	blkidCacheFile      string
}

func NewDiskConfig() *diskConfig {
	return &diskConfig{
		useMount:            false,
		includedDevices:     []regexp.Regexp{},
		excludedDevices:     []regexp.Regexp{},
		includedFilesystems: []regexp.Regexp{},
		excludedFilesystems: []regexp.Regexp{},
		includedMountpoints: []regexp.Regexp{},
		excludedMountpoints: []regexp.Regexp{},
		tagByFilesystem:     false,
		allPartitions:       false,
		deviceTagRe:         make(map[*regexp.Regexp][]string),
		allDevices:          true,
		minDiskSize:         0,
		tagByLabel:          true,
		useLsblk:            false,
		blkidCacheFile:      "",
	}
}

func (c *Check) diskConfigure(data integration.Data, initConfig integration.Data) error {
	unmarshalledInstanceConfig := make(map[interface{}]interface{})
	err := yaml.Unmarshal([]byte(data), &unmarshalledInstanceConfig)
	if err != nil {
		return err
	}
	unmarshalledInitConfig := make(map[interface{}]interface{})
	err = yaml.Unmarshal([]byte(initConfig), &unmarshalledInitConfig)
	if err != nil {
		return err
	}

	deprecationsInitConf := map[string]string{
		"file_system_global_blacklist": "file_system_global_exclude",
		"device_global_blacklist":      "device_global_exclude",
		"mount_point_global_blacklist": "mount_point_global_exclude",
	}
	for oldKey, newKey := range deprecationsInitConf {
		if _, exists := unmarshalledInitConfig[oldKey]; exists {
			log.Warnf("`%s` is deprecated and will be removed in a future release. Please use `%s` instead.", oldKey, newKey)
		}
	}

	deprecationsInstanceConf := map[string]string{
		"file_system_whitelist":  "file_system_include",
		"file_system_blacklist":  "file_system_exclude",
		"device_whitelist":       "device_include",
		"device_blacklist":       "device_exclude",
		"mount_point_whitelist":  "mount_point_include",
		"mount_point_blacklist":  "mount_point_exclude",
		"excluded_filesystems":   "file_system_exclude",
		"excluded_disks":         "device_exclude",
		"excluded_disk_re":       "device_exclude",
		"excluded_mountpoint_re": "mount_point_exclude",
	}
	for oldKey, newKey := range deprecationsInstanceConf {
		if _, exists := unmarshalledInstanceConfig[oldKey]; exists {
			log.Warnf("`%s` is deprecated and will be removed in a future release. Please use `%s` instead.", oldKey, newKey)
		}
	}

	c.cfg = NewDiskConfig()
	useMount, found := unmarshalledInstanceConfig["use_mount"]
	if useMount, ok := useMount.(bool); found && ok {
		c.cfg.useMount = useMount
	}
	includeAllDevices, found := unmarshalledInstanceConfig["include_all_devices"]
	if includeAllDevices, ok := includeAllDevices.(bool); found && ok {
		c.cfg.allDevices = includeAllDevices
	}
	allPartitions, found := unmarshalledInstanceConfig["all_partitions"]
	if allPartitions, ok := allPartitions.(bool); found && ok {
		c.cfg.allPartitions = allPartitions
	}
	minDiskSize, found := unmarshalledInstanceConfig["min_disk_size"]
	if minDiskSize, ok := minDiskSize.(int); found && ok {
		c.cfg.minDiskSize = uint64(minDiskSize)
	}
	tagByFilesystem, found := unmarshalledInstanceConfig["tag_by_filesystem"]
	if tagByFilesystem, ok := tagByFilesystem.(bool); found && ok {
		c.cfg.tagByFilesystem = tagByFilesystem
	}
	err = c.configureExcludeDevice(unmarshalledInstanceConfig, unmarshalledInitConfig)
	if err != nil {
		return err
	}
	err = c.configureIncludeDevice(unmarshalledInstanceConfig)
	if err != nil {
		return err
	}
	err = c.configureExcludeFileSystem(unmarshalledInstanceConfig, unmarshalledInitConfig)
	if err != nil {
		return err
	}
	err = c.configureIncludeFileSystem(unmarshalledInstanceConfig)
	if err != nil {
		return err
	}
	err = c.configureExcludeMountPoint(unmarshalledInstanceConfig, unmarshalledInitConfig)
	if err != nil {
		return err
	}
	err = c.configureIncludeMountPoint(unmarshalledInstanceConfig)
	if err != nil {
		return err
	}
	deviceTagRe, found := unmarshalledInstanceConfig["device_tag_re"]
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
	tagByLabel, found := unmarshalledInstanceConfig["tag_by_label"]
	if tagByLabel, ok := tagByLabel.(bool); found && ok {
		c.cfg.tagByLabel = tagByLabel
	}
	useLsblk, found := unmarshalledInstanceConfig["use_lsblk"]
	if useLsblk, ok := useLsblk.(bool); found && ok {
		c.cfg.useLsblk = useLsblk
	}
	blkidCacheFile, found := unmarshalledInstanceConfig["blkid_cache_file"]
	if blkidCacheFile, ok := blkidCacheFile.(string); found && ok {
		c.cfg.blkidCacheFile = blkidCacheFile
	}
	if c.cfg.tagByLabel && c.cfg.useLsblk && c.cfg.blkidCacheFile != "" {
		return errors.New("Only one of 'use_lsblk' and 'blkid_cache_file' can be set at the same time.")
	}

	return nil
}

func (c *Check) configureExcludeDevice(instanceConfig map[interface{}]interface{}, initConfig map[interface{}]interface{}) error {
	for _, key := range []string{"device_global_exclude", "device_global_blacklist"} {
		if deviceExclude, ok := initConfig[key].([]interface{}); ok {
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
	for _, key := range []string{"device_exclude", "device_blacklist", "excluded_disks"} {
		if deviceExclude, ok := instanceConfig[key].([]interface{}); ok {
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
	excludedDiskRe, found := instanceConfig["excluded_disk_re"] //Maintained for backwards compatibility. It would now be easier to add regular expressions to the 'device_exclude' list key
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

func (c *Check) configureIncludeDevice(instanceConfig map[interface{}]interface{}) error {
	for _, key := range []string{"device_include", "device_whitelist"} {
		if deviceInclude, ok := instanceConfig[key].([]interface{}); ok {
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

func (c *Check) configureExcludeFileSystem(instanceConfig map[interface{}]interface{}, initConfig map[interface{}]interface{}) error {
	for _, key := range []string{"file_system_global_exclude", "file_system_global_blacklist"} {
		if fileSystemExclude, ok := initConfig[key].([]interface{}); ok {
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
	for _, key := range []string{"file_system_exclude", "file_system_blacklist", "excluded_filesystems"} {
		if fileSystemExclude, ok := instanceConfig[key].([]interface{}); ok {
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

func (c *Check) configureExcludeMountPoint(instanceConfig map[interface{}]interface{}, initConfig map[interface{}]interface{}) error {
	for _, key := range []string{"mount_point_global_exclude", "mount_point_global_blacklist"} {
		if mountPointExclude, ok := initConfig[key].([]interface{}); ok {
			for _, val := range mountPointExclude {
				if strVal, ok := val.(string); ok {
					regexp, err := regexp.Compile(strVal)
					if err != nil {
						return err
					}
					c.cfg.excludedMountpoints = append(c.cfg.excludedMountpoints, *regexp)
				}
			}
		}
	}
	for _, key := range []string{"mount_point_exclude", "mount_point_blacklist"} {
		if mountPointExclude, ok := instanceConfig[key].([]interface{}); ok {
			for _, val := range mountPointExclude {
				if strVal, ok := val.(string); ok {
					regexp, err := regexp.Compile(strVal)
					if err != nil {
						return err
					}
					c.cfg.excludedMountpoints = append(c.cfg.excludedMountpoints, *regexp)
				}
			}
		}
	}
	excludedMountPointRe, found := instanceConfig["excluded_mountpoint_re"] //Maintained for backwards compatibility. It would now be easier to add regular expressions to the 'device_exclude' list key
	if excludedMountPointRe, ok := excludedMountPointRe.(string); found && ok {
		var err error
		regexp, err := regexp.Compile(excludedMountPointRe)
		if err != nil {
			return err
		}
		c.cfg.excludedMountpoints = append(c.cfg.excludedMountpoints, *regexp)
	}
	return nil
}

func (c *Check) configureIncludeMountPoint(conf map[interface{}]interface{}) error {
	for _, key := range []string{"mount_point_include", "mount_point_whitelist"} {
		if mountPointInclude, ok := conf[key].([]interface{}); ok {
			for _, val := range mountPointInclude {
				if strVal, ok := val.(string); ok {
					regexp, err := regexp.Compile(strVal)
					if err != nil {
						return err
					}
					c.cfg.includedMountpoints = append(c.cfg.includedMountpoints, *regexp)
				}
			}
		}
	}
	return nil
}

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
	log.Debugf("Applying device tags for [device: %s] [mountpoint: %s]", device, mountpoint)
	log.Debugf("Before applying device tags: %s", tags)
	for re, deviceTags := range c.cfg.deviceTagRe {
		if re.MatchString(device) || (mountpoint != "" && re.MatchString(mountpoint)) {
			tags = append(tags, deviceTags...)
		}
	}
	log.Debugf("After applying device tags: %s", tags)
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

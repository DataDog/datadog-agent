// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package disk

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/mitchellh/mapstructure"
	yaml "gopkg.in/yaml.v2"

	"regexp"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
)

const (
	CheckName   = "disk"
	diskMetric  = "system.disk.%s"
	inodeMetric = "system.fs.inodes.%s"
)

var (
	DiskPartitions = gopsutil_disk.Partitions
	DiskUsage      = gopsutil_disk.Usage
	DiskIOCounters = gopsutil_disk.IOCounters
)

type Mount struct {
	Host       string
	Share      string
	User       string
	Password   string
	Type       string
	MountPoint string
}

// RemotePath constructs the remote path based on the mount type.
// It converts the Type to uppercase to ensure case-insensitive evaluation.
func (m Mount) RemotePath() (string, string) {
	if strings.TrimSpace(m.Type) == "" {
		m.Type = "SMB"
	}
	// Convert Type to uppercase for case-insensitive comparison
	normalizedType := strings.ToUpper(strings.TrimSpace(m.Type))
	if normalizedType == "NFS" {
		return normalizedType, fmt.Sprintf(`%s:%s`, m.Host, m.Share)
	} else {
		var userAndPassword string
		if len(m.User) > 0 {
			userAndPassword += m.User
		}
		if len(m.Password) > 0 {
			userAndPassword += fmt.Sprintf(":%s", m.Password)
		}
		if len(userAndPassword) > 0 {
			return normalizedType, fmt.Sprintf(`\\%s@%s\%s`, userAndPassword, m.Host, m.Share)
		} else {
			return normalizedType, fmt.Sprintf(`\\%s\%s`, m.Host, m.Share)
		}
	}
}

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
	includeAllDevices   bool
	minDiskSize         uint64
	tagByLabel          bool
	useLsblk            bool
	blkidCacheFile      string
	serviceCheckRw      bool
}

func sliceMatchesExpression(slice []regexp.Regexp, expression string) bool {
	for _, regexp := range slice {
		if regexp.MatchString(expression) {
			return true
		}
	}
	return false
}

type Check struct {
	core.CheckBase
	cfg          *diskConfig
	deviceLabels map[string]string
}

func newDiskConfig() *diskConfig {
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
		includeAllDevices:   true,
		minDiskSize:         0,
		tagByLabel:          true,
		useLsblk:            false,
		blkidCacheFile:      "",
		serviceCheckRw:      false,
	}
}

func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	if c.cfg.tagByLabel {
		err = c.fetchAllDeviceLabels()
		if err != nil {
			log.Debugf("Unable to fetch device labels: %s", err)
		}
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

func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, initConfig, data, source)
	if err != nil {
		return err
	}
	return c.diskConfigure(data, initConfig)
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

	c.cfg = newDiskConfig()
	useMount, found := unmarshalledInstanceConfig["use_mount"]
	if useMount, ok := useMount.(bool); found && ok {
		c.cfg.useMount = useMount
	}
	includeAllDevices, found := unmarshalledInstanceConfig["include_all_devices"]
	if includeAllDevices, ok := includeAllDevices.(bool); found && ok {
		c.cfg.includeAllDevices = includeAllDevices
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
	serviceCheckRw, found := unmarshalledInstanceConfig["service_check_rw"]
	if serviceCheckRw, ok := serviceCheckRw.(bool); found && ok {
		c.cfg.serviceCheckRw = serviceCheckRw
	}

	createMounts, found := unmarshalledInstanceConfig["create_mounts"]
	if createMounts, ok := createMounts.([]interface{}); found && ok {
		for _, createMount := range createMounts {
			var m Mount
			err = mapstructure.Decode(createMount, &m)
			if err != nil {
				log.Debugf("Error decoding: %s\n", err)
				continue
			}
			if len(m.Host) == 0 || len(m.Share) == 0 {
				log.Errorf("Invalid configuration. Drive mount requires remote machine and share point")
				continue
			}
			log.Debugf("Mounting: %s\n", m)
			mountType, remoteName := m.RemotePath()
			log.Debugf("mountType: %s\n", mountType)
			err = NetAddConnection(mountType, m.MountPoint, remoteName, m.Password, m.User)
			if err != nil {
				log.Errorf("Failed to mount %s on %s: %s", m.MountPoint, remoteName, err)
				continue
			}
			log.Debugf("Successfully mounted %s as %s\n", m.MountPoint, remoteName)
		}
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
	excludedDiskRe, found := instanceConfig["excluded_disk_re"]
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
	excludedMountPointRe, found := instanceConfig["excluded_mountpoint_re"]
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

func (c *Check) collectPartitionMetrics(sender sender.Sender) error {
	partitions, err := DiskPartitions(c.cfg.includeAllDevices)
	if err != nil {
		log.Warnf("Unable to get disk partitions: %s", err)
		return err
	}
	log.Debugf("partitions %s", partitions)
	for _, partition := range partitions {
		log.Debugf("Checking partition: [device: %s] [mountpoint: %s] [fstype: %s]", partition.Device, partition.Mountpoint, partition.Fstype)
		if c.excludePartition(partition) {
			log.Debugf("Excluding partition: [device: %s] [mountpoint: %s] [fstype: %s]", partition.Device, partition.Mountpoint, partition.Fstype)
			continue
		}
		usage, err := DiskUsage(partition.Mountpoint)
		if err != nil {
			log.Warnf("Unable to get disk metrics for %s: %s. You can exclude this mountpoint in the settings if it is invalid.", partition.Mountpoint, err)
			continue
		}
		log.Debugf("usage %s", usage)
		// Exclude disks with total disk size 0
		if usage.Total <= c.cfg.minDiskSize {
			log.Debugf("Excluding partition: [device: %s] [mountpoint: %s] [fstype: %s] with total disk size %d", partition.Device, partition.Mountpoint, partition.Fstype, usage.Total)
			if usage.Total > 0 {
				log.Infof("Excluding partition: [device: %s] [mountpoint: %s] [fstype: %s] with total disk size %d", partition.Device, partition.Mountpoint, partition.Fstype, usage.Total)
			}
			continue
		}
		log.Debugf("Passed partition: [device: %s] [mountpoint: %s] [fstype: %s]", partition.Device, partition.Mountpoint, partition.Fstype)
		tags := []string{}
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
		tags = append(tags, c.getDeviceTags(deviceName)...)
		label, ok := c.deviceLabels[partition.Device]
		if ok {
			tags = append(tags, fmt.Sprintf("label:%s", label), fmt.Sprintf("device_label:%s", label))
		}
		c.sendPartitionMetrics(sender, usage, tags)

		if c.cfg.serviceCheckRw {
			checkStatus := servicecheck.ServiceCheckUnknown
			for _, opt := range partition.Opts {
				if opt == "rw" {
					checkStatus = servicecheck.ServiceCheckOK
					break
				} else if opt == "ro" {
					checkStatus = servicecheck.ServiceCheckCritical
					break
				}
			}
			sender.ServiceCheck("disk.read_write", checkStatus, "", tags, "")
		}
	}
	return nil
}

func (c *Check) collectDiskMetrics(sender sender.Sender) error {
	iomap, err := DiskIOCounters()
	if err != nil {
		log.Warnf("Unable to get disk iocounters: %s", err)
		return err
	}
	for deviceName, ioCounters := range iomap {
		log.Debugf("Checking iocounters: [device: %s] [ioCounters: %s]", deviceName, ioCounters)
		tags := []string{}
		tags = append(tags, fmt.Sprintf("device:%s", deviceName))
		tags = append(tags, fmt.Sprintf("device_name:%s", deviceName))
		tags = append(tags, c.getDeviceTags(deviceName)...)
		label, ok := c.deviceLabels[deviceName]
		if ok {
			tags = append(tags, fmt.Sprintf("label:%s", label), fmt.Sprintf("device_label:%s", label))
		}
		c.sendDiskMetrics(sender, ioCounters, tags)
	}

	return nil
}

func (c *Check) sendPartitionMetrics(sender sender.Sender, usage *gopsutil_disk.UsageStat, tags []string) {
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

func (c *Check) sendDiskMetrics(sender sender.Sender, ioCounter gopsutil_disk.IOCountersStat, tags []string) {
	sender.MonotonicCount(fmt.Sprintf(diskMetric, "read_time"), float64(ioCounter.ReadTime), "", tags)
	sender.MonotonicCount(fmt.Sprintf(diskMetric, "write_time"), float64(ioCounter.WriteTime), "", tags)
	// FIXME(8.x): These older metrics are kept here for backwards compatibility, but they are wrong: the value is not a percentage
	sender.Rate(fmt.Sprintf(diskMetric, "read_time_pct"), float64(ioCounter.ReadTime)*100/1000, "", tags)
	sender.Rate(fmt.Sprintf(diskMetric, "write_time_pct"), float64(ioCounter.WriteTime)*100/1000, "", tags)
}

func (c *Check) excludePartition(partition gopsutil_disk.PartitionStat) bool {
	device := partition.Device
	if device == "" || device == "none" {
		device = ""
		if !c.cfg.allPartitions {
			return true
		}
	}
	// Hack for NFS secure mounts
	// Secure mounts might look like this: '/mypath (deleted)', we should
	// ignore all the bits not part of the mountpoint name. Take also into
	// account a space might be in the mountpoint.
	mountpoint := strings.Split(partition.Mountpoint, " ")[0]
	exclude := c.excludeDevice(device) || c.excludeFileSystem(partition.Fstype) || c.excludeMountPoint(mountpoint) || !c.includeDevice(device) || !c.includeFileSystem(partition.Fstype) || !c.includeMountPoint(mountpoint)
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
	return sliceMatchesExpression(c.cfg.excludedFilesystems, fileSystem)
}

func (c *Check) includeFileSystem(fileSystem string) bool {
	if len(c.cfg.includedFilesystems) == 0 {
		return true
	}
	return sliceMatchesExpression(c.cfg.includedFilesystems, fileSystem)
}

func (c *Check) excludeMountPoint(mountPoint string) bool {
	if len(c.cfg.excludedMountpoints) == 0 {
		return false
	}
	return sliceMatchesExpression(c.cfg.excludedMountpoints, mountPoint)
}

func (c *Check) includeMountPoint(mountPoint string) bool {
	if len(c.cfg.includedMountpoints) == 0 {
		return true
	}
	return sliceMatchesExpression(c.cfg.includedMountpoints, mountPoint)
}

func (c *Check) getDeviceTags(device string) []string {
	tags := []string{}
	log.Debugf("Getting device tags for device '%s'", device)
	for re, deviceTags := range c.cfg.deviceTagRe {
		if re.MatchString(device) {
			tags = append(tags, deviceTags...)
		}
	}
	log.Debugf("getDeviceTags: %s", tags)
	return tags
}

func (c *Check) fetchAllDeviceLabels() error {
	log.Debugf("Fetching all device labels")
	if c.cfg.useLsblk {
		return c.fetchAllDeviceLabelsFromLsblk()
	} else if c.cfg.blkidCacheFile != "" {
		return c.fetchAllDeviceLabelsFromBlkidCache()
	}
	return c.fetchAllDeviceLabelsFromBlkid()
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase:    core.NewCheckBase(CheckName),
		deviceLabels: make(map[string]string),
	}
}

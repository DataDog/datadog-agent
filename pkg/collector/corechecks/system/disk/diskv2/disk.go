// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diskv2 provides Disk Check.
package diskv2

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/shirou/gopsutil/v4/common"
	gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
	"github.com/spf13/afero"
	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName   = "disk"
	diskMetric  = "system.disk.%s"
	inodeMetric = "system.fs.inodes.%s"
)

// diskInstanceConfig represents an instance configuration.
type diskInitConfig struct {
	DeviceGlobalExclude       []string `yaml:"device_global_exclude"`
	DeviceGlobalBlacklist     []string `yaml:"device_global_blacklist"`
	FileSystemGlobalExclude   []string `yaml:"file_system_global_exclude"`
	FileSystemGlobalBlacklist []string `yaml:"file_system_global_blacklist"`
	MountPointGlobalExclude   []string `yaml:"mount_point_global_exclude"`
	MountPointGlobalBlacklist []string `yaml:"mount_point_global_blacklist"`
}

// mount represents a network mount configuration.
type mount struct {
	Host       string `yaml:"host"`
	Share      string `yaml:"share"`
	User       string `yaml:"user"`
	Password   string `yaml:"password"`
	Type       string `yaml:"type"`
	MountPoint string `yaml:"mountpoint"`
}

// diskInstanceConfig represents an instance configuration.
type diskInstanceConfig struct {
	UseMount             bool              `yaml:"use_mount"`
	IncludeAllDevices    bool              `yaml:"include_all_devices"`
	AllPartitions        bool              `yaml:"all_partitions"`
	MinDiskSize          uint64            `yaml:"min_disk_size"`
	TagByFilesystem      bool              `yaml:"tag_by_filesystem"`
	TagByLabel           bool              `yaml:"tag_by_label"`
	UseLsblk             bool              `yaml:"use_lsblk"`
	BlkidCacheFile       string            `yaml:"blkid_cache_file"`
	ServiceCheckRw       bool              `yaml:"service_check_rw"`
	CreateMounts         []mount           `yaml:"create_mounts"`
	DeviceInclude        []string          `yaml:"device_include"`
	DeviceWhitelist      []string          `yaml:"device_whitelist"`
	DeviceExclude        []string          `yaml:"device_exclude"`
	DeviceBlacklist      []string          `yaml:"device_blacklist"`
	ExcludedDisks        []string          `yaml:"excluded_disks"`
	ExcludedDiskRe       string            `yaml:"excluded_disk_re"`
	FileSystemInclude    []string          `yaml:"file_system_include"`
	FileSystemWhitelist  []string          `yaml:"file_system_whitelist"`
	FileSystemExclude    []string          `yaml:"file_system_exclude"`
	FileSystemBlacklist  []string          `yaml:"file_system_blacklist"`
	ExcludedFileSystems  []string          `yaml:"excluded_filesystems"`
	MountPointInclude    []string          `yaml:"mount_point_include"`
	MountPointWhitelist  []string          `yaml:"mount_point_whitelist"`
	MountPointExclude    []string          `yaml:"mount_point_exclude"`
	MountPointBlacklist  []string          `yaml:"mount_point_blacklist"`
	ExcludedMountPointRe string            `yaml:"excluded_mountpoint_re"`
	DeviceTagRe          map[string]string `yaml:"device_tag_re"`
	LowercaseDeviceTag   bool              `yaml:"lowercase_device_tag"`
	Timeout              uint16            `yaml:"timeout"`
	ProcMountInfoPath    string            `yaml:"proc_mountinfo_path"`
	ResolveRootDevice    bool              `yaml:"resolve_root_device"`
}

func sliceMatchesExpression(slice []regexp.Regexp, expression string) bool {
	for _, regexp := range slice {
		if regexp.MatchString(expression) {
			return true
		}
	}
	return false
}

func compileRegExp(expr string, ignoreCase bool) (*regexp.Regexp, error) {
	if ignoreCase {
		expr = fmt.Sprintf("(?i)%s", expr)
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		log.Warnf("`%s` is not a valid regular expression and will be ignored", expr)
	}
	return re, err
}

// StatT type
type StatT struct {
	Major uint32
	Minor uint32
}

type statFunc func(string) (StatT, error)

// Check represents the Disk check that will be periodically executed via the Run() function
type Check struct {
	core.CheckBase
	clock                     clock.Clock
	diskPartitionsWithContext func(context.Context, bool) ([]gopsutil_disk.PartitionStat, error)
	diskUsage                 func(string) (*gopsutil_disk.UsageStat, error)
	diskIOCounters            func(...string) (map[string]gopsutil_disk.IOCountersStat, error)
	fs                        afero.Fs
	statFn                    statFunc

	initConfig          diskInitConfig
	instanceConfig      diskInstanceConfig
	includedDevices     []regexp.Regexp
	excludedDevices     []regexp.Regexp
	includedFilesystems []regexp.Regexp
	excludedFilesystems []regexp.Regexp
	includedMountpoints []regexp.Regexp
	excludedMountpoints []regexp.Regexp
	deviceTagRe         map[*regexp.Regexp][]string
	deviceLabels        map[string]string
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	if c.instanceConfig.TagByLabel {
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

// Configure parses the check configuration and init the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) error {
	if flavor.GetFlavor() == flavor.DefaultAgent && !pkgconfigsetup.Datadog().GetBool("disk_check.use_core_loader") && !pkgconfigsetup.Datadog().GetBool("use_diskv2_check") {
		// if use_diskv2_check, then do not skip the core check
		return fmt.Errorf("%w: disk core check is disabled", check.ErrSkipCheckInstance)
	}

	err := c.CommonConfigure(senderManager, initConfig, data, source)
	if err != nil {
		return err
	}
	return c.configureDiskCheck(data, initConfig)
}

func (c *Check) configureDiskCheck(data integration.Data, initConfig integration.Data) error {
	err := c.checkDeprecatedConfig(data, initConfig)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal([]byte(initConfig), &c.initConfig)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal([]byte(data), &c.instanceConfig)
	if err != nil {
		return err
	}
	err = c.configureExcludeDevice()
	if err != nil {
		return err
	}
	err = c.configureIncludeDevice()
	if err != nil {
		return err
	}
	err = c.configureExcludeFileSystem()
	if err != nil {
		return err
	}
	err = c.configureIncludeFileSystem()
	if err != nil {
		return err
	}
	err = c.configureExcludeMountPoint()
	if err != nil {
		return err
	}
	err = c.configureIncludeMountPoint()
	if err != nil {
		return err
	}
	for reString, tags := range c.instanceConfig.DeviceTagRe {
		if re, err := compileRegExp(reString, defaultIgnoreCase()); err == nil {
			splitTags := strings.Split(tags, ",")
			for i, tag := range splitTags {
				splitTags[i] = strings.TrimSpace(tag)
			}
			c.deviceTagRe[re] = splitTags
		} else {
			return err
		}
	}
	if c.instanceConfig.UseLsblk && c.instanceConfig.BlkidCacheFile != "" {
		return errors.New("only one of 'use_lsblk' and 'blkid_cache_file' can be set at the same time")
	}
	c.configureCreateMounts()
	return nil
}

func (c *Check) checkDeprecatedConfig(data integration.Data, initConfig integration.Data) error {
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
	return nil
}

func processRegExpSlices(slices [][]string, ignoreCase bool) ([]regexp.Regexp, error) {
	regExpList := []regexp.Regexp{}
	for _, slice := range slices {
		for _, val := range slice {
			if re, err := compileRegExp(val, ignoreCase); err == nil {
				regExpList = append(regExpList, *re)
			} else {
				return regExpList, err
			}
		}
	}
	return regExpList, nil
}

func processRegExpSlicesWholeWord(slices [][]string, ignoreCase bool) ([]regexp.Regexp, error) {
	regExpList := []regexp.Regexp{}
	for _, slice := range slices {
		for _, val := range slice {
			expr := fmt.Sprintf("^%s$", val)
			if re, err := compileRegExp(expr, ignoreCase); err == nil {
				regExpList = append(regExpList, *re)
			} else {
				return regExpList, err
			}
		}
	}
	return regExpList, nil
}

func (c *Check) configureExcludeDevice() error {
	c.excludedDevices = []regexp.Regexp{}
	if regExpList, err := processRegExpSlices([][]string{c.initConfig.DeviceGlobalExclude, c.initConfig.DeviceGlobalBlacklist, c.instanceConfig.DeviceExclude, c.instanceConfig.DeviceBlacklist}, defaultIgnoreCase()); err == nil {
		c.excludedDevices = append(c.excludedDevices, regExpList...)
	} else {
		return err
	}
	if regExpList, err := processRegExpSlicesWholeWord([][]string{c.instanceConfig.ExcludedDisks}, true); err == nil {
		c.excludedDevices = append(c.excludedDevices, regExpList...)
	} else {
		return err
	}
	if c.instanceConfig.ExcludedDiskRe != "" {
		if re, err := compileRegExp(c.instanceConfig.ExcludedDiskRe, defaultIgnoreCase()); err == nil {
			c.excludedDevices = append(c.excludedDevices, *re)
		} else {
			return err
		}
	}
	return nil
}

func (c *Check) configureIncludeDevice() error {
	c.includedDevices = []regexp.Regexp{}
	if regExpList, err := processRegExpSlices([][]string{c.instanceConfig.DeviceInclude, c.instanceConfig.DeviceWhitelist}, defaultIgnoreCase()); err == nil {
		c.includedDevices = append(c.includedDevices, regExpList...)
	} else {
		return err
	}
	return nil
}

func (c *Check) configureExcludeFileSystem() error {
	c.excludedFilesystems = []regexp.Regexp{}
	if regExpList, err := processRegExpSlices([][]string{c.initConfig.FileSystemGlobalExclude, c.initConfig.FileSystemGlobalBlacklist}, true); err == nil {
		c.excludedFilesystems = append(c.excludedFilesystems, regExpList...)
	} else {
		return err
	}
	if len(c.excludedFilesystems) == 0 {
		// Use default values if neither key was found
		for _, val := range []string{"iso9660$", "tracefs$"} {
			if re, err := compileRegExp(val, true); err == nil {
				c.excludedFilesystems = append(c.excludedFilesystems, *re)
			} else {
				return err
			}
		}
	}
	if regExpList, err := processRegExpSlices([][]string{c.instanceConfig.FileSystemExclude, c.instanceConfig.FileSystemBlacklist}, true); err == nil {
		c.excludedFilesystems = append(c.excludedFilesystems, regExpList...)
	} else {
		return err
	}
	if regExpList, err := processRegExpSlicesWholeWord([][]string{c.instanceConfig.ExcludedFileSystems}, true); err == nil {
		c.excludedFilesystems = append(c.excludedFilesystems, regExpList...)
	} else {
		return err
	}
	return nil
}

func (c *Check) configureIncludeFileSystem() error {
	c.includedFilesystems = []regexp.Regexp{}
	if regExpList, err := processRegExpSlices([][]string{c.instanceConfig.FileSystemInclude, c.instanceConfig.FileSystemWhitelist}, true); err == nil {
		c.includedFilesystems = append(c.includedFilesystems, regExpList...)
	} else {
		return err
	}
	return nil
}

func (c *Check) configureExcludeMountPoint() error {
	c.excludedMountpoints = []regexp.Regexp{}
	if regExpList, err := processRegExpSlices([][]string{c.initConfig.MountPointGlobalExclude, c.initConfig.MountPointGlobalBlacklist}, true); err == nil {
		c.excludedMountpoints = append(c.excludedMountpoints, regExpList...)
	} else {
		return err
	}
	if len(c.excludedMountpoints) == 0 {
		// https://github.com/DataDog/datadog-agent/issues/1961
		// https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2018-1049
		for _, val := range []string{"(/host)?/proc/sys/fs/binfmt_misc$"} {
			if re, err := compileRegExp(val, defaultIgnoreCase()); err == nil {
				c.excludedMountpoints = append(c.excludedMountpoints, *re)
			} else {
				return err
			}
		}
	}
	if regExpList, err := processRegExpSlices([][]string{c.instanceConfig.MountPointExclude, c.instanceConfig.MountPointBlacklist}, true); err == nil {
		c.excludedMountpoints = append(c.excludedMountpoints, regExpList...)
	} else {
		return err
	}
	if c.instanceConfig.ExcludedMountPointRe != "" {
		if re, err := compileRegExp(c.instanceConfig.ExcludedMountPointRe, defaultIgnoreCase()); err == nil {
			c.excludedMountpoints = append(c.excludedMountpoints, *re)
		} else {
			return err
		}
	}
	return nil
}

func (c *Check) configureIncludeMountPoint() error {
	c.includedMountpoints = []regexp.Regexp{}
	if regExpList, err := processRegExpSlices([][]string{c.instanceConfig.MountPointInclude, c.instanceConfig.MountPointWhitelist}, true); err == nil {
		c.includedMountpoints = append(c.includedMountpoints, regExpList...)
	} else {
		return err
	}
	return nil
}

func (c *Check) collectPartitionMetrics(sender sender.Sender) error {
	ctx := context.Background()
	if c.instanceConfig.ProcMountInfoPath != "" {
		ctx = context.WithValue(ctx, common.EnvKey, common.EnvMap{common.HostProcMountinfo: c.instanceConfig.ProcMountInfoPath})
	}
	partitions, err := c.diskPartitionsWithContext(ctx, c.instanceConfig.IncludeAllDevices)
	if err != nil {
		log.Warnf("Unable to get disk partitions: %s", err)
		return err
	}
	rootDevices := make(map[string]string)
	if runtime.GOOS == "linux" && !c.instanceConfig.ResolveRootDevice {
		rootDevices, err = c.loadRootDevices()
		if err != nil {
			log.Warnf("Error reading raw devices: %s", err)
			rootDevices = map[string]string{}
		}
	}
	log.Debugf("rootDevices '%s'", rootDevices)
	for _, partition := range partitions {
		if rootDev, ok := rootDevices[partition.Device]; ok {
			log.Debugf("Found [device: %s] in rootDevices as [rawDev: %s]", partition.Device, rootDev)
			partition.Device = rootDev
		}
		log.Debugf("Checking partition: [device: %s] [mountpoint: %s] [fstype: %s] [opts: %s]", partition.Device, partition.Mountpoint, partition.Fstype, partition.Opts)
		if c.excludePartition(partition) {
			log.Debugf("Excluding partition: [device: %s] [mountpoint: %s] [fstype: %s]", partition.Device, partition.Mountpoint, partition.Fstype)
			continue
		}
		if usage := c.getPartitionUsage(partition); usage != nil {
			tags := c.getPartitionTags(partition)
			c.sendPartitionMetrics(sender, usage, tags)

			if c.instanceConfig.ServiceCheckRw {
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
	}
	return nil
}

func (c *Check) collectDiskMetrics(sender sender.Sender) error {
	iomap, err := c.diskIOCounters()
	if err != nil {
		log.Warnf("Unable to get disk iocounters: %s", err)
		return err
	}
	for deviceName, ioCounters := range iomap {
		log.Debugf("Checking iocounters: [device: %s] [ioCounters: %s]", deviceName, ioCounters)
		tags := c.getDeviceNameTags(deviceName)
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
	sender.Gauge(fmt.Sprintf(diskMetric, "utilized"), usage.UsedPercent, "", tags)
	// FIXME(8.x): use percent, a lot more logical than in_use
	sender.Gauge(fmt.Sprintf(diskMetric, "in_use"), usage.UsedPercent/100, "", tags)

	c.sendInodesMetrics(sender, usage, tags)
}

func (c *Check) sendDiskMetrics(sender sender.Sender, ioCounter gopsutil_disk.IOCountersStat, tags []string) {
	sender.MonotonicCount(fmt.Sprintf(diskMetric, "read_time"), float64(ioCounter.ReadTime), "", tags)
	sender.MonotonicCount(fmt.Sprintf(diskMetric, "write_time"), float64(ioCounter.WriteTime), "", tags)
	// FIXME(8.x): These older metrics are kept here for backwards compatibility, but they are wrong: the value is not a percentage
	// See: https://github.com/DataDog/integrations-core/pull/7323#issuecomment-756427024
	sender.Rate(fmt.Sprintf(diskMetric, "read_time_pct"), float64(ioCounter.ReadTime)*100/1000, "", tags)
	sender.Rate(fmt.Sprintf(diskMetric, "write_time_pct"), float64(ioCounter.WriteTime)*100/1000, "", tags)
}

func (c *Check) getDiskUsageWithTimeout(mountpoint string) (*gopsutil_disk.UsageStat, error) {
	type usageResult struct {
		usage *gopsutil_disk.UsageStat
		err   error
	}
	resultCh := make(chan usageResult, 1)
	timeout := time.Duration(c.instanceConfig.Timeout) * time.Second
	timeoutCh := c.clock.After(timeout)
	// Start the disk usage call in a separate goroutine.
	go func() {
		// UsageWithContext in gopsutil ignores the context for now (PR opened: https://github.com/shirou/gopsutil/pull/1837)
		usage, err := c.diskUsage(mountpoint)
		// Use select to avoid writing to resultCh if timeout already occurred.
		select {
		case resultCh <- usageResult{usage, err}:
		case <-timeoutCh:
		}
	}()
	// Use select to wait for either the disk usage result or a timeout.
	select {
	case result := <-resultCh:
		return result.usage, result.err
	case <-timeoutCh:
		return nil, fmt.Errorf("disk usage call timed out after %s", timeout)
	}
}

func (c *Check) getPartitionUsage(partition gopsutil_disk.PartitionStat) *gopsutil_disk.UsageStat {
	usage, err := c.getDiskUsageWithTimeout(partition.Mountpoint)
	if err != nil {
		log.Warnf("Unable to get disk metrics for %s: %s. You can exclude this mountpoint in the settings if it is invalid.", partition.Mountpoint, err)
		return nil
	}
	log.Debugf("usage %s", usage)
	// Exclude disks with total disk size 0
	if usage.Total == 0 {
		log.Debugf("Excluding partition: [device: %s] [mountpoint: %s] [fstype: %s] with total disk size %d bytes", partition.Device, partition.Mountpoint, partition.Fstype, usage.Total)
		return nil
	}
	// Exclude disks with total disk size smaller than 'min_disk_size' (which is configured in MiB)
	minDiskSizeInBytes := c.instanceConfig.MinDiskSize * 1024 * 1024
	if usage.Total < minDiskSizeInBytes {
		log.Infof("Excluding partition: [device: %s] [mountpoint: %s] [fstype: %s] with total disk size %d bytes", partition.Device, partition.Mountpoint, partition.Fstype, usage.Total)
		return nil
	}
	log.Debugf("Passed partition: [device: %s] [mountpoint: %s] [fstype: %s]", partition.Device, partition.Mountpoint, partition.Fstype)
	return usage
}

func (c *Check) getPartitionTags(partition gopsutil_disk.PartitionStat) []string {
	tags := []string{}
	if c.instanceConfig.TagByFilesystem {
		tags = append(tags, partition.Fstype, fmt.Sprintf("filesystem:%s", partition.Fstype))
	}
	var deviceName string
	if c.instanceConfig.UseMount {
		deviceName = partition.Mountpoint
	} else {
		deviceName = partition.Device
	}
	if c.instanceConfig.LowercaseDeviceTag {
		tags = append(tags, fmt.Sprintf("device:%s", strings.ToLower(deviceName)))
	} else {
		tags = append(tags, fmt.Sprintf("device:%s", deviceName))
	}
	tags = append(tags, fmt.Sprintf("device_name:%s", baseDeviceName(partition.Device)))
	tags = append(tags, c.getDeviceTags(deviceName)...)
	label, ok := c.deviceLabels[partition.Device]
	if ok {
		tags = append(tags, fmt.Sprintf("label:%s", label), fmt.Sprintf("device_label:%s", label))
	}
	return tags
}

func (c *Check) getDeviceNameTags(deviceName string) []string {
	tags := []string{}
	if c.instanceConfig.LowercaseDeviceTag {
		tags = append(tags, fmt.Sprintf("device:%s", strings.ToLower(deviceName)))
	} else {
		tags = append(tags, fmt.Sprintf("device:%s", deviceName))
	}
	tags = append(tags, fmt.Sprintf("device_name:%s", baseDeviceName(deviceName)))
	tags = append(tags, c.getDeviceTags(deviceName)...)
	label, ok := c.deviceLabels[deviceName]
	if ok {
		tags = append(tags, fmt.Sprintf("label:%s", label), fmt.Sprintf("device_label:%s", label))
	}
	return tags
}

func (c *Check) excludePartition(partition gopsutil_disk.PartitionStat) bool {
	if c.excludePartitionInPlatform(partition) {
		return true
	}
	device := partition.Device
	if device == "" || device == "none" {
		device = ""
		if !c.instanceConfig.AllPartitions {
			return true
		}
	}
	// Hack for NFS secure mounts
	// Secure mounts might look like this: '/mypath (deleted)', we should
	// ignore all the bits not part of the mountpoint name. Take also into
	// account a space might be in the mountpoint.
	mountPoint := partition.Mountpoint
	index := strings.LastIndex(mountPoint, " ")
	// If a space is found, update mountPoint to be everything before the last space.
	if index != -1 {
		mountPoint = mountPoint[:index]
	}
	excludePartition := c.excludeDevice(device) || c.excludeFileSystem(partition.Fstype) || c.excludeMountPoint(mountPoint)
	if excludePartition {
		return true
	}
	includePartition := c.includeDevice(device) && c.includeFileSystem(partition.Fstype) && c.includeMountPoint(mountPoint)
	return !includePartition
}

func (c *Check) excludeDevice(device string) bool {
	if device == "" || len(c.excludedDevices) == 0 {
		return false
	}
	return sliceMatchesExpression(c.excludedDevices, device)
}

func (c *Check) includeDevice(device string) bool {
	if device == "" || len(c.includedDevices) == 0 {
		return true
	}
	return sliceMatchesExpression(c.includedDevices, device)
}

func (c *Check) excludeFileSystem(fileSystem string) bool {
	if len(c.excludedFilesystems) == 0 {
		return false
	}
	return sliceMatchesExpression(c.excludedFilesystems, fileSystem)
}

func (c *Check) includeFileSystem(fileSystem string) bool {
	if len(c.includedFilesystems) == 0 {
		return true
	}
	return sliceMatchesExpression(c.includedFilesystems, fileSystem)
}

func (c *Check) excludeMountPoint(mountPoint string) bool {
	if len(c.excludedMountpoints) == 0 {
		return false
	}
	return sliceMatchesExpression(c.excludedMountpoints, mountPoint)
}

func (c *Check) includeMountPoint(mountPoint string) bool {
	if len(c.includedMountpoints) == 0 {
		return true
	}
	return sliceMatchesExpression(c.includedMountpoints, mountPoint)
}

func (c *Check) getDeviceTags(device string) []string {
	tags := []string{}
	log.Debugf("Getting device tags for device '%s'", device)
	for re, deviceTags := range c.deviceTagRe {
		if re.MatchString(device) {
			tags = append(tags, deviceTags...)
		}
	}
	log.Debugf("getDeviceTags: %s", tags)
	return tags
}

func (c *Check) fetchAllDeviceLabels() error {
	log.Debugf("Fetching all device labels")
	if c.instanceConfig.UseLsblk {
		return c.fetchAllDeviceLabelsFromLsblk()
	} else if c.instanceConfig.BlkidCacheFile != "" {
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
		CheckBase:                 core.NewCheckBase(CheckName),
		clock:                     clock.New(),
		diskPartitionsWithContext: gopsutil_disk.PartitionsWithContext,
		diskUsage:                 gopsutil_disk.Usage,
		diskIOCounters:            gopsutil_disk.IOCounters,
		fs:                        afero.NewOsFs(),
		statFn:                    defaultStatFn,
		initConfig: diskInitConfig{
			DeviceGlobalExclude:       []string{},
			DeviceGlobalBlacklist:     []string{},
			FileSystemGlobalExclude:   []string{},
			FileSystemGlobalBlacklist: []string{},
			MountPointGlobalExclude:   []string{},
			MountPointGlobalBlacklist: []string{},
		},
		instanceConfig: diskInstanceConfig{
			UseMount:             false,
			IncludeAllDevices:    true,
			AllPartitions:        false,
			MinDiskSize:          0,
			TagByFilesystem:      false,
			TagByLabel:           true,
			UseLsblk:             false,
			BlkidCacheFile:       "",
			ServiceCheckRw:       false,
			CreateMounts:         []mount{},
			DeviceInclude:        []string{},
			DeviceWhitelist:      []string{},
			DeviceExclude:        []string{},
			DeviceBlacklist:      []string{},
			ExcludedDisks:        []string{},
			ExcludedDiskRe:       "",
			FileSystemInclude:    []string{},
			FileSystemWhitelist:  []string{},
			FileSystemExclude:    []string{},
			FileSystemBlacklist:  []string{},
			ExcludedFileSystems:  []string{},
			MountPointInclude:    []string{},
			MountPointWhitelist:  []string{},
			MountPointExclude:    []string{},
			MountPointBlacklist:  []string{},
			ExcludedMountPointRe: "",
			DeviceTagRe:          make(map[string]string),
			LowercaseDeviceTag:   false,
			Timeout:              5,
			// Match psutil exactly setting default value (https://github.com/giampaolo/psutil/blob/3d21a43a47ab6f3c4a08d235d2a9a55d4adae9b1/psutil/_pslinux.py#L1277)
			ProcMountInfoPath: "/proc/self/mounts",
			// Match psutil reporting '/dev/root' from /proc/self/mounts by default
			ResolveRootDevice: false,
		},
		includedDevices:     []regexp.Regexp{},
		excludedDevices:     []regexp.Regexp{},
		includedFilesystems: []regexp.Regexp{},
		excludedFilesystems: []regexp.Regexp{},
		includedMountpoints: []regexp.Regexp{},
		excludedMountpoints: []regexp.Regexp{},
		deviceTagRe:         make(map[*regexp.Regexp][]string),
		deviceLabels:        make(map[string]string),
	}
}

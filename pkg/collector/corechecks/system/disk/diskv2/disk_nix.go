// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package diskv2

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
)

func defaultIgnoreCase() bool {
	return false
}

func baseDeviceName(device string) string {
	return filepath.Base(device)
}

func (c *Check) configureCreateMounts() {
}

func (c *Check) excludePartitionInPlatform(_ gopsutil_disk.PartitionStat) bool {
	return false
}

// LsblkCommand specifies the command used to retrieve block device information.
var LsblkCommand = func() (string, error) {
	cmd := exec.Command("lsblk", "--noheadings", "--raw", "--output=NAME,LABEL")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
var labelRegex = regexp.MustCompile(`(?i)LABEL="([^"]+)"`)

func (c *Check) fetchAllDeviceLabelsFromLsblk() error {
	log.Debugf("Fetching all device labels from lsblk")
	rawOutput, err := LsblkCommand()
	if err != nil {
		return err
	}
	log.Debugf("lsblk output: %s", rawOutput)
	lines := strings.Split(strings.TrimSpace(rawOutput), "\n")
	c.deviceLabels = make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		log.Debugf("processing line: '%s'", line)
		if line == "" {
			log.Debugf("skipping empty line")
			continue
		}
		// Typically line looks like:
		// sda1  MY_LABEL
		device, label, ok := strings.Cut(line, " ")
		if !ok {
			log.Debugf("skipping malformed line: '%s'", line)
			continue
		}
		if len(label) == 0 {
			log.Debugf("skipping empty label: '%s'", line)
			continue
		}
		device = "/dev/" + device
		c.deviceLabels[device] = label
	}
	return nil
}

// Device represents a device entry in an XML structure.
type device struct {
	XMLName xml.Name `xml:"device"`
	Label   string   `xml:"LABEL,attr"`
	Text    string   `xml:",chardata"`
}

// BlkidCacheCommand specifies the command used to query block device UUIDs and labels.
var BlkidCacheCommand = func(blkidCacheFile string) (string, error) {
	file, err := os.Open(blkidCacheFile)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Check) fetchAllDeviceLabelsFromBlkidCache() error {
	log.Debugf("Fetching all device labels from blkid cache")
	rawOutput, err := BlkidCacheCommand(c.instanceConfig.BlkidCacheFile)
	if err != nil {
		return err
	}
	log.Debugf("blkid cache output: %s", rawOutput)
	lines := strings.Split(strings.TrimSpace(rawOutput), "\n")
	c.deviceLabels = make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		log.Debugf("processing line: '%s'", line)
		if line == "" {
			log.Debugf("skipping empty line")
			continue
		}
		var device device
		err := xml.Unmarshal([]byte(line), &device)
		if err != nil {
			log.Debugf("Failed to parse line %s because of %v - skipping the line (some labels might be missing)\n", line, err)
			continue
		}
		if device.Label != "" && device.Text != "" {
			c.deviceLabels[device.Text] = device.Label
		}
	}
	return nil
}

// BlkidCommand specifies the command used to retrieve block device attributes.
var BlkidCommand = func() (string, error) {
	cmd := exec.Command("blkid")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (c *Check) fetchAllDeviceLabelsFromBlkid() error {
	log.Debugf("Fetching all device labels from blkid")
	rawOutput, err := BlkidCommand()
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(rawOutput), "\n")
	c.deviceLabels = make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		log.Debugf("processing line: '%s'", line)
		if line == "" {
			log.Debugf("skipping empty line")
			continue
		}
		// Typically line looks like:
		// /dev/sda1: UUID="..." TYPE="ext4" LABEL="root"
		device, details, ok := strings.Cut(line, ":")
		if !ok {
			log.Debugf("skipping malformed line: '%s'", line)
			continue
		}
		device = strings.TrimSpace(device)   // e.g. "/dev/sda1"
		details = strings.TrimSpace(details) // e.g. `UUID="..." TYPE="ext4" LABEL="root"`
		match := labelRegex.FindStringSubmatch(details)
		if len(match) == 2 {
			// match[1] is everything captured by ([^"]+)
			c.deviceLabels[device] = match[1]
		}
	}
	return nil
}

func (c *Check) sendInodesMetrics(sender sender.Sender, usage *gopsutil_disk.UsageStat, tags []string) {
	if usage.InodesTotal != 0 {
		// Inodes metrics
		sender.Gauge(fmt.Sprintf(inodeMetric, "total"), float64(usage.InodesTotal), "", tags)
		sender.Gauge(fmt.Sprintf(inodeMetric, "used"), float64(usage.InodesUsed), "", tags)
		sender.Gauge(fmt.Sprintf(inodeMetric, "free"), float64(usage.InodesFree), "", tags)
		sender.Gauge(fmt.Sprintf(inodeMetric, "utilized"), usage.InodesUsedPercent, "", tags)
		// FIXME(8.x): use percent, a lot more logical than in_use
		sender.Gauge(fmt.Sprintf(inodeMetric, "in_use"), usage.InodesUsedPercent/100, "", tags)
	}
}

func (c *Check) getProcMountInfoPath() string {
	hpmPath := c.instanceConfig.ProcMountInfoPath
	if hpmPath == "" {
		hpmPath = os.Getenv("HOST_PROC_MOUNTINFO")
	}
	return hpmPath
}

func readAllLines(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func readMountFile(root string) (lines []string, useMounts bool, filename string, err error) {
	filename = path.Join(root, "mountinfo")
	lines, err = readAllLines(filename)
	if err != nil {
		// if kernel does not support 1/mountinfo, fallback to 1/mounts (<2.6.26)
		useMounts = true
		filename = path.Join(root, "mounts")
		lines, err = readAllLines(filename)
		if err != nil {
			return
		}
		return
	}
	return
}

func (c *Check) loadRawDevices() (map[string]string, error) {
	// Determine base "/proc" root (HOST_PROC override)
	hostProc := os.Getenv("HOST_PROC")
	if hostProc == "" {
		hostProc = "/proc"
	}
	// Default to /proc/1; but if user set a HOST_PROC_MOUNTINFO path, use its dirname
	root := filepath.Join(hostProc, "1")
	hpmPath := c.getProcMountInfoPath()
	if hpmPath != "" {
		root = filepath.Dir(hpmPath)
	}
	// Try reading mount data
	lines, useMounts, filename, err := readMountFile(root)
	if err != nil && hpmPath == "" {
		// fallback to /proc/self
		root = filepath.Join(hostProc, "self")
		lines, useMounts, filename, err = readMountFile(root)
	}
	if err != nil {
		return nil, fmt.Errorf("reading mount file: %w", err)
	}
	// Build the map
	rawDevices := make(map[string]string)
	log.Debugf("mountfile [%s] lines: '%s'", filename, lines)
	if !useMounts {
		// Determine base "/sys" root (HOST_SYS override)
		hostSys := os.Getenv("HOST_SYS")
		if hostSys == "" {
			hostSys = "/sys"
		}
		for _, line := range lines {
			// a line of 1/mountinfo has the following structure:
			// 36  35  98:0 /mnt1 /mnt2 rw,noatime master:1 - ext3 /dev/root rw,errors=continue
			// (1) (2) (3)   (4)   (5)      (6)      (7)   (8) (9)   (10)         (11)

			// split the mountinfo line by the separator hyphen
			parts := strings.Split(line, " - ")
			if len(parts) != 2 {
				return nil, fmt.Errorf("found invalid mountinfo line in file %s: %s ", filename, line)
			}
			fieldsFirstPart := strings.Fields(parts[0])
			blockDeviceID := fieldsFirstPart[2]
			fieldsSecondPart := strings.Fields(parts[1])
			device := fieldsSecondPart[1]
			// /dev/root is not the real device name
			// so we get the real device name from its major/minor number
			if device == "/dev/root" {
				log.Debugf("/dev/root line: '%s'", line)
				devpath, err := os.Readlink(path.Join(hostSys, "/dev/block", blockDeviceID))
				if err == nil {
					deviceResolved := strings.Replace(device, "root", filepath.Base(devpath), 1)
					log.Debugf("device_resolved: '%s'", deviceResolved)
					rawDevices[deviceResolved] = device
				}
			}
		}
	}
	return rawDevices, nil
}

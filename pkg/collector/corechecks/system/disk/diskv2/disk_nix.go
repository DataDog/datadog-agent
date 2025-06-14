// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package diskv2

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
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

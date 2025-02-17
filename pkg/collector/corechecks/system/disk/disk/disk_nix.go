// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package disk

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LsblkCommand specifies the command used to retrieve block device information.
var LsblkCommand = func() (string, error) {
	cmd := exec.Command("lsblk", "--noheadings", "--raw", "--output=NAME,LABEL")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (c *Check) fetchAllDeviceLabelsFromLsblk() error {
	log.Debugf("Fetching all device labels from lsblk")
	rawOutput, err := LsblkCommand()
	if err != nil {
		return err
	}
	log.Debugf("lsblk output: %s", rawOutput)
	lines := strings.Split(strings.TrimSpace(rawOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		log.Debugf("processing line: '%s'", line)
		if line == "" {
			continue
		}
		// Typically line looks like:
		// sda1  MY_LABEL
		fields := strings.Fields(line)
		if len(fields) < 2 {
			// skip malformed lines
			continue
		}
		device := "/dev/" + fields[0]
		label := fields[1]
		c.deviceLabels[device] = label
	}
	return nil
}

// Device represents a device entry in an XML structure.
type Device struct {
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
	rawOutput, err := BlkidCacheCommand(c.cfg.blkidCacheFile)
	if err != nil {
		return err
	}
	log.Debugf("blkid cache output: %s", rawOutput)
	c.deviceLabels = make(map[string]string)
	lines := strings.Split(strings.TrimSpace(rawOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		log.Debugf("processing line: '%s'", line)
		if line == "" {
			continue
		}
		var device Device
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
	c.deviceLabels = make(map[string]string)
	labelRegex := regexp.MustCompile(`LABEL="([^"]+)"`)
	lines := strings.Split(strings.TrimSpace(rawOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		log.Debugf("processing line: '%s'", line)
		if line == "" {
			continue
		}
		// Typically line looks like:
		// /dev/sda1: UUID="..." TYPE="ext4" LABEL="root"
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			// skip malformed lines
			continue
		}
		device := strings.TrimSpace(parts[0])  // e.g. "/dev/sda1"
		details := strings.TrimSpace(parts[1]) // e.g. `UUID="..." TYPE="ext4" LABEL="root"`
		match := labelRegex.FindStringSubmatch(details)
		if len(match) == 2 {
			// match[1] is everything captured by ([^"]+)
			c.deviceLabels[device] = match[1]
		} else {
			// No label found for this device
			c.deviceLabels[device] = ""
		}

	}
	return nil
}

// NetAddConnection specifies the command used to add a new network connection.
var NetAddConnection = func(mountType, localName, remoteName, _password, _username string) error {
	args := []string{}
	args = append(args, "-t")
	if mountType == "SMB" {
		args = append(args, "smbfs")
	} else {
		args = append(args, mountType)
	}
	args = append(args, remoteName)
	args = append(args, localName)
	cmd := exec.Command("mount", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return err
}

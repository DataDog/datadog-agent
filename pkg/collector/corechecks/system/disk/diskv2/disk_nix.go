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
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
	"github.com/spf13/afero"
	"golang.org/x/sys/unix"
)

var defaultStatFn statFunc = func(path string) (StatT, error) {
	var st unix.Stat_t
	err := unix.Stat(path, &st)
	return StatT{Major: unix.Major(uint64(st.Dev)),
		Minor: unix.Minor(uint64(st.Dev))}, err
}

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

func getProcfsPath() string {
	// Determine base "/proc" root (HOST_PROC override)
	hostProc := os.Getenv("HOST_PROC")
	if hostProc == "" {
		hostProc = "/proc"
	}
	return hostProc
}

func getSysfsPath() string {
	// Determine base "/sys" root (HOST_SYS override)
	hostSys := os.Getenv("HOST_SYS")
	if hostSys == "" {
		hostSys = "/sys"
	}
	return hostSys
}

func (c *Check) getProcMountInfoPath() string {
	hpmPath := c.instanceConfig.ProcMountInfoPath
	if hpmPath == "" {
		hpmPath = os.Getenv("HOST_PROC_MOUNTINFO")
	}
	return hpmPath
}

func readAllLines(fs afero.Fs, filename string) ([]string, error) {
	f, err := fs.Open(filename)
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

func readMountFile(fs afero.Fs, root string) (lines []string, useMounts bool, filename string, err error) {
	filename = path.Join(root, "mountinfo")
	lines, err = readAllLines(fs, filename)
	if err != nil {
		useMounts = true
		filename = path.Join(root, "mounts")
		lines, err = readAllLines(fs, filename)
		if err != nil {
			return
		}
		return
	}
	return
}

type rootFsDeviceFinder struct {
	Fs    afero.Fs
	major uint32
	minor uint32
}

func newRootFsDeviceFinder(fs afero.Fs, statFn statFunc) (*rootFsDeviceFinder, error) {
	var st StatT
	st, err := statFn("/")
	if err != nil {
		return nil, err
	}
	log.Debugf("Major[%v], Minor[%v]", unix.Major(uint64(st.Major)), unix.Minor(uint64(st.Minor)))
	return &rootFsDeviceFinder{
		Fs:    fs,
		major: st.Major,
		minor: st.Minor,
	}, nil
}

func (r *rootFsDeviceFinder) askProcPartitions() (string, error) {
	f, err := r.Fs.Open(filepath.Join(getProcfsPath(), "partitions"))
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// skip header lines
	scanner.Scan()
	scanner.Scan()
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		maj, err1 := strconv.ParseUint(fields[0], 10, 32)
		min, err2 := strconv.ParseUint(fields[1], 10, 32)
		if err1 != nil || err2 != nil {
			continue
		}
		if uint32(maj) == r.major && uint32(min) == r.minor {
			name := fields[3]
			if name != "" {
				return "/dev/" + name, nil
			}
		}
	}
	return "", nil
}

func (r *rootFsDeviceFinder) askSysDevBlock() (string, error) {
	path := fmt.Sprintf("/sys/dev/block/%d:%d/uevent", r.major, r.minor)
	f, err := r.Fs.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "DEVNAME=") {
			name := strings.TrimPrefix(line, "DEVNAME=")
			if name != "" {
				return "/dev/" + name, nil
			}
		}
	}
	return "", nil
}

func (r *rootFsDeviceFinder) askSysClassBlock() (string, error) {
	needle := fmt.Sprintf("%d:%d", r.major, r.minor)
	matches, err := filepath.Glob("/sys/class/block/*/dev")
	if err != nil {
		return "", err
	}

	for _, devFile := range matches {
		data, err := os.ReadFile(devFile)
		if err != nil {
			// race conditions are fine
			continue
		}
		if strings.TrimSpace(string(data)) == needle {
			// dirname of ".../block/<name>/dev" is the device name
			name := filepath.Base(filepath.Dir(devFile))
			if name != "" {
				return "/dev/" + name, nil
			}
		}
	}
	return "", nil
}

func (r *rootFsDeviceFinder) Find() (string, error) {
	// Try /proc/partitions
	if path, err := r.askProcPartitions(); err == nil && path != "" {
		if _, err := r.Fs.Stat(path); err == nil {
			return path, nil
		}
	}

	// Try /sys/dev/block
	if path, err := r.askSysDevBlock(); err == nil && path != "" {
		if _, err := r.Fs.Stat(path); err == nil {
			return path, nil
		}
	}

	// Try /sys/class/block
	if path, err := r.askSysClassBlock(); err == nil && path != "" {
		if _, err := r.Fs.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("could not determine rootfs device")
}

// ReadlinkFs method
func ReadlinkFs(fs afero.Fs, name string) (string, error) {
	if lf, ok := fs.(afero.LinkReader); ok {
		if target, err := lf.ReadlinkIfPossible(name); err == nil {
			return target, nil
		}
	}
	return os.Readlink(name)
}

func (c *Check) loadRootDevices() (map[string]string, error) {
	// Determine base "/proc" root (HOST_PROC override)
	hostProc := getProcfsPath()
	// Default to /proc/1; but if user set a HOST_PROC_MOUNTINFO path, use its dirname
	root := filepath.Join(hostProc, "1")
	hpmPath := c.getProcMountInfoPath()
	if hpmPath != "" {
		root = filepath.Dir(hpmPath)
	}
	// Try reading mount data
	lines, useMounts, filename, err := readMountFile(c.fs, root)
	if err != nil && hpmPath == "" {
		// fallback to /proc/self
		root = filepath.Join(hostProc, "self")
		lines, useMounts, filename, err = readMountFile(c.fs, root)
	}
	if err != nil {
		return nil, fmt.Errorf("reading mount file: %w", err)
	}
	// Build the map
	rootDevices := make(map[string]string)
	if !useMounts {
		finder, err := newRootFsDeviceFinder(c.fs, c.statFn)
		var rootFsDevice string
		if err == nil {
			rootFsDevice, err = finder.Find()
			if err != nil {
				log.Debugf("error finding root device: %v", err)
			}
		} else {
			log.Debugf("error stat’ing /: %v", err)
		}
		hostSys := getSysfsPath()
		for _, line := range lines {
			log.Debugf("parsing line: '%s'", line)
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
			if device == "/dev/root" || device == "rootfs" {
				log.Debugf("/dev/root line: '%s'", line)
				linkPath := filepath.Join(hostSys, "dev", "block", blockDeviceID)
				devPath, err := ReadlinkFs(c.fs, linkPath)
				if err == nil {
					base := filepath.Base(devPath)
					deviceResolved := strings.Replace(device, "root", base, 1)
					log.Debugf("device_resolved: '%s'", deviceResolved)
					if rootFsDevice != "" {
						device = rootFsDevice
					}
					rootDevices[deviceResolved] = device
				}
			}
		}
	}
	return rootDevices, nil
}

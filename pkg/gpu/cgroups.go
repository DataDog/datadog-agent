// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigureDeviceCgroups configures the cgroups for a process to allow access to the NVIDIA character devices
func ConfigureDeviceCgroups(pid uint32, rootfs string) error {
	cgroupReader, err := cgroups.NewReader(cgroups.WithHostPrefix(rootfs))
	if err != nil {
		return fmt.Errorf("failed to get cgroups reader: %w", err)
	}

	cgroup, err := getCgroupForProcess(cgroupReader, pid)
	if err != nil {
		return fmt.Errorf("failed to get cgroup for pid %d: %w", pid, err)
	}

	// Configure systemd device allow first, so that in case of a reload we get the correct permissions
	// The containerID for systemd is the last part of the cgroup path
	if err := configureSystemDAllow(cgroup.Identifier(), rootfs); err != nil {
		return fmt.Errorf("failed to configure systemd device allow for cgroup %s: %w", cgroup.Identifier(), err)
	}

	cgroupPath := getFullCgroupPath(cgroup)

	// Now configure the cgroup device allow, depending on the cgroup version
	if cgroupReader.CgroupVersion() == 1 {
		err = configureCgroupV1DeviceAllow(cgroupPath, rootfs)
	} else {
		err = configureCgroupV2DeviceAllow(cgroupPath, rootfs)
	}

	if err != nil {
		return fmt.Errorf("failed to configure cgroup device allow for cgroup path %s: %w", cgroupPath, err)
	}

	return nil
}

const (
	systemdDeviceAllowFile     = "50-DeviceAllow.conf"
	systemdTransientConfigPath = "run/systemd/transient"
	cgroupv1DeviceAllowFile    = "devices.allow"
	cgroupv1DeviceAllowDir     = "sys/fs/cgroup/devices"
	nvidiaSystemdDeviceAllow   = "DeviceAllow=char-nvidia rwm\n" // Allow access to the NVIDIA character devices
	nvidiaCgroupv1Allow        = "c 195:* rwm\n"                 // 195 is the major number for the NVIDIA character devices
)

func getCgroupForProcess(cgroupReader *cgroups.Reader, pid uint32) (cgroups.Cgroup, error) {
	for _, cgroup := range cgroupReader.ListCgroups() {
		pids, err := cgroup.GetPIDs(5 * time.Second)
		if err != nil {
			log.Debugf("failed to get pids for cgroup %s: %v", cgroup.Identifier(), err)
			// Ignore the error and continue to the next cgroup
			continue
		}
		if slices.Contains(pids, int(pid)) {
			return cgroup, nil
		}
	}

	return nil, fmt.Errorf("failed to get cgroup for pid %d", pid)
}

func getFullCgroupPath(cgroup cgroups.Cgroup) string {
	path := cgroup.Identifier()
	parent, err := cgroup.GetParent()
	for err == nil { // GetParent returns nil if the cgroup is the root cgroup
		path = filepath.Join(parent.Identifier(), path)

		parent, err = parent.GetParent()
	}

	return path
}

// insertAfterSection finds a section header in the lines and inserts the new line after it
func insertAfterSection(lines []string, sectionHeader, newLine string) ([]string, error) {
	// Find the section header line
	sectionIndex := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == sectionHeader {
			sectionIndex = i
			break
		}
	}

	if sectionIndex == -1 {
		return nil, fmt.Errorf("failed to find section header %s", sectionHeader)
	}

	// Insert the new line after the section header
	newLines := make([]string, len(lines)+1)
	copy(newLines, lines[:sectionIndex+1])
	newLines[sectionIndex+1] = newLine
	copy(newLines[sectionIndex+2:], lines[sectionIndex+1:])

	return newLines, nil
}

func configureSystemDAllow(containerID, rootfs string) error {
	// The SystemD device configuration might be either in a 50-DeviceAllow.conf file
	// in a service configuration directory, or in a service file directly. Default to the .conf
	// file and fall back to the service file if it doesn't exist.
	configFilePath, err := buildSafePath(rootfs, systemdTransientConfigPath, containerID+".d", systemdDeviceAllowFile)
	if err != nil {
		return fmt.Errorf("failed to build path for systemd device allow: %w", err)
	}

	if _, err := os.Stat(configFilePath); errors.Is(err, os.ErrNotExist) {
		configFilePath, err := buildSafePath(rootfs, systemdTransientConfigPath, containerID, ".service")
		if err != nil {
			return fmt.Errorf("failed to build path for systemd service file: %w", err)
		}

		if _, err := os.Stat(configFilePath); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to find either a SystemD configuration directory or configuration file for container %s", containerID)
		}
	}

	// Read the entire file
	content, err := os.ReadFile(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configFilePath, err)
	}

	lines := strings.Split(string(content), "\n")

	// Insert the nvidiaDeviceAllow line after [Service]
	newLines, err := insertAfterSection(lines, "[Service]", nvidiaSystemdDeviceAllow)
	if err != nil {
		return fmt.Errorf("failed to insert device allow line: %w", err)
	}

	// Write the modified content back to the file
	newContent := strings.Join(newLines, "\n")
	err = os.WriteFile(configFilePath, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write modified config to %s: %w", configFilePath, err)
	}

	return nil
}

func configureCgroupV1DeviceAllow(rootfs, cgroupPath string) error {
	deviceAllowPath, err := buildSafePath(rootfs, cgroupv1DeviceAllowDir, cgroupPath, cgroupv1DeviceAllowFile)
	if err != nil {
		return fmt.Errorf("failed to build path for cgroupv1 device allow: %w", err)
	}

	log.Debugf("configuring cgroupv1 device allow for cgroup path %s: %s", cgroupPath, deviceAllowPath)

	deviceAllowFile, err := os.OpenFile(deviceAllowPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", deviceAllowPath, err)
	}
	defer deviceAllowFile.Close()

	_, err = deviceAllowFile.WriteString(nvidiaCgroupv1Allow)
	if err != nil {
		return fmt.Errorf("failed to write to %s: %w", deviceAllowPath, err)
	}

	return nil
}

// configureCgroupV2DeviceAllow configures device permissions for cgroupv2 using BPF programs
func configureCgroupV2DeviceAllow(rootfs, cgroupPath string) error {
	// Create a BPF program that allows access to NVIDIA devices (major number 195)
	// The program receives device access parameters and should return 1 to allow, 0 to deny
	prog, err := ebpf.NewProgram(&ebpf.ProgramSpec{
		Type: ebpf.CGroupDevice,
		Instructions: asm.Instructions{
			// R1 contains the device type (1=block, 2=char)
			// R2 contains the major number
			// R3 contains the minor number
			// R4 contains the access type (read=1, write=2, mknod=4)

			// Check if this is a character device (type 2)
			asm.LoadImm(asm.R0, 2, asm.DWord),
			asm.JNE.Reg(asm.R1, asm.R0, "deny"),

			// Check if this is an NVIDIA device (major number 195)
			asm.LoadImm(asm.R0, 195, asm.DWord),
			asm.JNE.Reg(asm.R2, asm.R0, "deny"),

			// Allow access to NVIDIA character devices
			asm.LoadImm(asm.R0, 1, asm.DWord),
			asm.Return(),

			// Deny all other devices
			asm.LoadImm(asm.R0, 0, asm.DWord).WithSymbol("deny"),
			asm.Return(),
		},
		License: "GPL",
	})
	if err != nil {
		return fmt.Errorf("failed to create BPF program: %w", err)
	}
	defer prog.Close()

	cgroupHostPath, err := buildSafePath(rootfs, "sys/fs/cgroup", cgroupPath)
	if err != nil {
		return fmt.Errorf("failed to build host path for cgroup %s: %w", cgroupPath, err)
	}

	// Attach the program to the cgroup
	log.Debugf("attaching BPF program to cgroup path %s", cgroupHostPath)
	l, err := link.AttachCgroup(link.CgroupOptions{
		Path:    cgroupHostPath,
		Attach:  ebpf.AttachCGroupDevice,
		Program: prog,
	})
	if err != nil {
		return fmt.Errorf("failed to attach BPF program to cgroup %s: %w", cgroupPath, err)
	}
	defer l.Close()

	log.Debugf("successfully attached BPF device allow program to cgroup %s", cgroupPath)
	return nil
}

// buildSafePath builds a safe path from the rootfs and basedir, and appends the
// parts to it. It assumes that rootfs and basedir are already validated paths,
// and check that the parts being added to the path do not cause the final path
// to escape the rootfs/basedir.
func buildSafePath(rootfs string, basedir string, parts ...string) (string, error) {
	rootfs = strings.TrimSuffix(rootfs, "/")   // Remove trailing slashes from rootfs
	basedir = strings.TrimPrefix(basedir, "/") // Remove leading slashes from basedir

	// that way we can now join the paths using Sprintf to build the base directory
	root := fmt.Sprintf("%s/%s", rootfs, basedir)

	// Join the parts to the base directory and create a full path. Note that this will also remove any ".." from the path
	fullPath := filepath.Join(append([]string{root}, parts...)...)

	// Check that the resulting path is a child of root and that we haven't escaped the rootfs/basedir
	if !strings.HasPrefix(fullPath, root) {
		return "", fmt.Errorf("invalid path %s, should be a child of %s", fullPath, root)
	}

	return fullPath, nil
}
